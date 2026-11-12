// Command beastbench is a reproducible YCSB-style load generator for BeastDB.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"
)

// Report is the full machine-readable result.
type Report struct {
	Tool       string        `json:"tool"`
	GoVersion  string        `json:"go_version"`
	GOOS       string        `json:"goos"`
	GOARCH     string        `json:"goarch"`
	NumCPU     int           `json:"num_cpu"`
	SelfHosted bool          `json:"self_hosted"`
	Addr       string        `json:"addr"`
	Config     Config        `json:"config"`
	Phases     []PhaseResult `json:"phases"`
}

func main() {
	if err := realMain(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "beastbench:", err)
		os.Exit(1)
	}
}

func realMain(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("beastbench", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfg      Config
		co       ConnOpts
		self     = fs.Bool("self-contained", false, "start an in-process engine+gRPC server on a temp dir (smoke runs only)")
		poolSize = fs.Int("pool-size", 128, "buffer pool frames for --self-contained")
		asJSON   = fs.Bool("json", false, "emit JSON instead of text")
		skipLoad = fs.Bool("skip-load", false, "skip the load phase (dataset already loaded with the same --records/--value-size/--seed)")
		loadOnly = fs.Bool("load-only", false, "only run the load phase")
		timeout  = fs.Duration("op-timeout", 30*time.Second, "per-operation timeout")
	)
	fs.StringVar(&co.Addr, "addr", "127.0.0.1:50051", "BeastDB gRPC address")
	fs.StringVar(&co.Token, "token", strings.TrimSpace(os.Getenv("BEASTDB_API_TOKEN")), "Bearer token (default $BEASTDB_API_TOKEN)")
	fs.StringVar(&co.TLSCert, "tls-cert", os.Getenv("BEASTDB_TLS_CERT"), "client TLS certificate (PEM)")
	fs.StringVar(&co.TLSKey, "tls-key", os.Getenv("BEASTDB_TLS_KEY"), "client TLS key (PEM)")
	fs.StringVar(&co.TLSCA, "tls-ca", os.Getenv("BEASTDB_TLS_CA"), "CA certificate to verify the server (PEM)")
	fs.StringVar(&co.TLSServerName, "tls-server-name", "", "override TLS server name")
	fs.IntVar(&co.Conns, "conns", 1, "gRPC connections (threads are spread round-robin)")
	fs.StringVar(&cfg.Workload, "workload", "A", "workload A-F")
	fs.StringVar(&cfg.Dist, "dist", "zipfian", "key distribution: zipfian or uniform (workload D always uses read-latest)")
	fs.Float64Var(&cfg.ZipfTheta, "zipf-theta", DefaultZipfTheta, "zipfian skew constant in (0,1)")
	fs.Uint64Var(&cfg.Records, "records", 10000, "records loaded before the run")
	fs.Uint64Var(&cfg.Ops, "ops", 50000, "operations in the measured run phase")
	fs.Uint64Var(&cfg.WarmupOps, "warmup", 0, "unmeasured warmup operations before the run phase")
	fs.IntVar(&cfg.Threads, "threads", 8, "concurrent client workers")
	fs.IntVar(&cfg.ValueSize, "value-size", 100, "value size in bytes (8..2097152)")
	fs.IntVar(&cfg.MaxScanLen, "max-scan-len", 100, "workload E scan length is uniform in [1,max-scan-len]")
	fs.Int64Var(&cfg.Seed, "seed", 1, "RNG seed; same seed gives the same operation sequence per thread")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg.Workload = strings.ToUpper(cfg.Workload)
	cfg.Dist = strings.ToLower(cfg.Dist)
	cfg.Timeout = *timeout
	if _, err := lookupWorkload(cfg.Workload); err != nil {
		return err
	}
	switch {
	case cfg.Dist != "zipfian" && cfg.Dist != "uniform":
		return fmt.Errorf("--dist must be zipfian or uniform")
	case cfg.Records < 2, cfg.Threads < 1, cfg.MaxScanLen < 1:
		return fmt.Errorf("need --records >= 2, --threads >= 1, --max-scan-len >= 1")
	case cfg.ValueSize < 8 || cfg.ValueSize > 2<<20:
		return fmt.Errorf("--value-size must be in [8, 2097152]")
	case cfg.ZipfTheta <= 0 || cfg.ZipfTheta >= 1:
		return fmt.Errorf("--zipf-theta must be in (0,1)")
	}
	if uint64(cfg.Threads) > cfg.Records {
		cfg.Threads = int(cfg.Records)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *self {
		addr, stopSrv, err := StartSelfHosted(*poolSize)
		if err != nil {
			return err
		}
		defer stopSrv()
		co.Addr = addr
		co.TLSCert, co.TLSKey, co.TLSCA, co.Token = "", "", "", ""
	}
	cl, closeConns, err := Dial(co)
	if err != nil {
		return err
	}
	defer closeConns()

	rep := Report{Tool: "beastbench", GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		NumCPU: runtime.NumCPU(), SelfHosted: *self, Addr: co.Addr, Config: cfg}

	if !*skipLoad {
		res, err := Load(ctx, cl, cfg)
		rep.Phases = append(rep.Phases, res)
		if err != nil {
			return err
		}
	}
	if !*loadOnly {
		res, err := Run(ctx, cl, cfg)
		rep.Phases = append(rep.Phases, res)
		if err != nil {
			return err
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	printText(stdout, rep)
	return nil
}

func printText(w io.Writer, r Report) {
	c := r.Config
	fmt.Fprintf(w, "beastbench  workload=%s dist=%s records=%d ops=%d warmup=%d threads=%d value=%dB seed=%d\n",
		c.Workload, c.Dist, c.Records, c.Ops, c.WarmupOps, c.Threads, c.ValueSize, c.Seed)
	fmt.Fprintf(w, "%s/%s go=%s cpus=%d self-contained=%v addr=%s\n", r.GOOS, r.GOARCH, r.GoVersion, r.NumCPU, r.SelfHosted, r.Addr)
	if r.SelfHosted {
		fmt.Fprintln(w, "NOTE: self-contained loopback smoke run; not a competitive benchmark.")
	}
	for _, p := range r.Phases {
		fmt.Fprintf(w, "\n[%s] %d ops in %.2fs = %.0f ops/s\n", p.Phase, p.Ops, p.Seconds, p.Throughput)
		fmt.Fprintf(w, "  %-8s %10s %10s %10s %10s %10s %10s %7s %7s %7s\n",
			"op", "count", "ops/s", "p50(us)", "p95(us)", "p99(us)", "max(us)", "errors", "misses", "confl")
		for _, o := range p.PerOp {
			fmt.Fprintf(w, "  %-8s %10d %10.0f %10.1f %10.1f %10.1f %10.1f %7d %7d %7d\n",
				o.Op, o.Count, o.Throughput, o.P50Us, o.P95Us, o.P99Us, o.MaxUs, o.Errors, o.Misses, o.Conflicts)
		}
	}
}
