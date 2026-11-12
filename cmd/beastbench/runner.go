package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config holds everything that determines a run.
type Config struct {
	Workload   string        `json:"workload"`
	Dist       string        `json:"distribution"`
	ZipfTheta  float64       `json:"zipf_theta,omitempty"`
	Records    uint64        `json:"records"`
	Ops        uint64        `json:"ops"`
	WarmupOps  uint64        `json:"warmup_ops"`
	Threads    int           `json:"threads"`
	ValueSize  int           `json:"value_size"`
	MaxScanLen int           `json:"max_scan_len"`
	Seed       int64         `json:"seed"`
	Timeout    time.Duration `json:"-"`
}

// OpStats is the per-operation result.
type OpStats struct {
	Op         string  `json:"op"`
	Count      uint64  `json:"count"`
	Errors     uint64  `json:"errors"`
	Misses     uint64  `json:"misses,omitempty"`    // reads/scans that found nothing
	Conflicts  uint64  `json:"conflicts,omitempty"` // conditional writes rejected (rmw)
	Throughput float64 `json:"ops_per_sec"`
	MeanUs     float64 `json:"mean_us"`
	P50Us      float64 `json:"p50_us"`
	P95Us      float64 `json:"p95_us"`
	P99Us      float64 `json:"p99_us"`
	MaxUs      float64 `json:"max_us"`
}

// PhaseResult summarises one phase.
type PhaseResult struct {
	Phase      string    `json:"phase"`
	Seconds    float64   `json:"seconds"`
	Ops        uint64    `json:"ops"`
	Throughput float64   `json:"ops_per_sec"`
	PerOp      []OpStats `json:"per_op"`
}

func us(d time.Duration) float64 { return float64(d) / float64(time.Microsecond) }

func recordKey(idx uint64) uint64 { return idx + 1 }

// makeValue deterministically fills a value for (idx, version).
func makeValue(buf []byte, idx uint64, version uint64) {
	x := idx*0x9E3779B97F4A7C15 + version + 1
	for i := 0; i < len(buf); i += 8 {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], x)
		copy(buf[i:], tmp[:])
	}
}

type counters struct {
	hist      [numOps]*Histogram
	errs      [numOps]uint64
	misses    [numOps]uint64
	conflicts [numOps]uint64
}

func newCounters() *counters {
	c := &counters{}
	for i := range c.hist {
		c.hist[i] = NewHistogram()
	}
	return c
}

// Load inserts cfg.Records records with the given parallelism.
func Load(ctx context.Context, cl []beastv1.BeastDBServiceClient, cfg Config) (PhaseResult, error) {
	cs := make([]*counters, cfg.Threads)
	var wg sync.WaitGroup
	var firstErr atomic.Value
	start := time.Now()
	per := (cfg.Records + uint64(cfg.Threads) - 1) / uint64(cfg.Threads)
	for t := 0; t < cfg.Threads; t++ {
		cs[t] = newCounters()
		wg.Add(1)
		go func(t int) {
			defer wg.Done()
			c := cs[t]
			client := cl[t%len(cl)]
			buf := make([]byte, cfg.ValueSize)
			lo := uint64(t) * per
			hi := lo + per
			if hi > cfg.Records {
				hi = cfg.Records
			}
			for i := lo; i < hi; i++ {
				if ctx.Err() != nil {
					return
				}
				makeValue(buf, i, 0)
				t0 := time.Now()
				_, err := callPut(ctx, client, recordKey(i), buf, nil, cfg.Timeout)
				c.hist[OpInsert].Record(time.Since(t0))
				if err != nil {
					c.errs[OpInsert]++
					firstErr.CompareAndSwap(nil, err)
					return
				}
			}
		}(t)
	}
	wg.Wait()
	el := time.Since(start)
	res := summarize("load", el, cs)
	if e, ok := firstErr.Load().(error); ok {
		return res, fmt.Errorf("load failed: %w", e)
	}
	return res, ctx.Err()
}

// callPut returns conflict=true when a conditional write was rejected (ABORTED).
func callPut(ctx context.Context, c beastv1.BeastDBServiceClient, key uint64, val []byte, cond *beastv1.Condition, to time.Duration) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	_, err := c.Put(cctx, &beastv1.PutRequest{Key: key, Value: val, Condition: cond})
	if err != nil {
		if status.Code(err) == codes.Aborted {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// Run executes warmup (unrecorded) then the measured run phase.
func Run(ctx context.Context, cl []beastv1.BeastDBServiceClient, cfg Config) (PhaseResult, error) {
	w, err := lookupWorkload(cfg.Workload)
	if err != nil {
		return PhaseResult{}, err
	}
	frontier := &atomic.Uint64{}
	frontier.Store(cfg.Records)

	var chooser KeyChooser
	switch {
	case w.Latest:
		chooser = Latest{Zipf: NewZipfian(cfg.Records, cfg.ZipfTheta), Frontier: frontier}
	case cfg.Dist == "uniform":
		chooser = Uniform{N: cfg.Records}
	default:
		chooser = NewZipfian(cfg.Records, cfg.ZipfTheta)
	}

	phase := func(total uint64, seedOff int64, record bool) ([]*counters, time.Duration) {
		cs := make([]*counters, cfg.Threads)
		var wg sync.WaitGroup
		start := time.Now()
		for t := 0; t < cfg.Threads; t++ {
			cs[t] = newCounters()
			n := total / uint64(cfg.Threads)
			if uint64(t) < total%uint64(cfg.Threads) {
				n++
			}
			wg.Add(1)
			go func(t int, n uint64) {
				defer wg.Done()
				r := rand.New(rand.NewSource(cfg.Seed + seedOff + int64(t)*7919))
				worker(ctx, cl[t%len(cl)], cfg, w, chooser, frontier, r, n, cs[t], record)
			}(t, n)
		}
		wg.Wait()
		return cs, time.Since(start)
	}

	if cfg.WarmupOps > 0 {
		phase(cfg.WarmupOps, 1_000_003, false)
	}
	cs, el := phase(cfg.Ops, 0, true)
	return summarize("run", el, cs), ctx.Err()
}

func worker(ctx context.Context, cl beastv1.BeastDBServiceClient, cfg Config, w Workload,
	ch KeyChooser, frontier *atomic.Uint64, r *rand.Rand, n uint64, c *counters, record bool) {
	buf := make([]byte, cfg.ValueSize)
	for i := uint64(0); i < n && ctx.Err() == nil; i++ {
		op := w.pick(r.Float64())
		t0 := time.Now()
		var err error
		var miss, conflict bool
		switch op {
		case OpRead:
			idx := ch.Next(r)
			var found bool
			found, err = doGet(ctx, cl, recordKey(idx), cfg.Timeout, nil)
			miss = err == nil && !found
		case OpUpdate:
			idx := ch.Next(r)
			r.Read(buf)
			conflict, err = callPut(ctx, cl, recordKey(idx), buf, nil, cfg.Timeout)
		case OpInsert:
			idx := frontier.Add(1) - 1
			makeValue(buf, idx, 0)
			conflict, err = callPut(ctx, cl, recordKey(idx), buf, nil, cfg.Timeout)
		case OpScan:
			idx := ch.Next(r)
			ln := 1 + r.Intn(cfg.MaxScanLen)
			var got int
			got, err = doScan(ctx, cl, recordKey(idx), uint32(ln), cfg.Timeout)
			miss = err == nil && got == 0
		case OpRMW:
			idx := ch.Next(r)
			var old []byte
			var found bool
			found, err = doGet(ctx, cl, recordKey(idx), cfg.Timeout, &old)
			if err == nil && !found {
				miss = true
				break
			}
			if err == nil {
				nv := append([]byte(nil), old...)
				if len(nv) > 0 {
					nv[r.Intn(len(nv))] ^= byte(1 + r.Intn(255))
				}
				conflict, err = callPut(ctx, cl, recordKey(idx), nv, &beastv1.Condition{
					Kind: beastv1.ConditionKind_CONDITION_KIND_VALUE_EQUALS, Value: old}, cfg.Timeout)
			}
		}
		if ctx.Err() != nil {
			return
		}
		if !record {
			continue
		}
		c.hist[op].Record(time.Since(t0))
		if err != nil {
			c.errs[op]++
		}
		if miss {
			c.misses[op]++
		}
		if conflict {
			c.conflicts[op]++
		}
	}
}

func doGet(ctx context.Context, cl beastv1.BeastDBServiceClient, key uint64, to time.Duration, out *[]byte) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	resp, err := cl.Get(cctx, &beastv1.GetRequest{Key: key})
	if err != nil {
		return false, err
	}
	if out != nil {
		*out = resp.Value
	}
	return resp.Found, nil
}

func doScan(ctx context.Context, cl beastv1.BeastDBServiceClient, start uint64, limit uint32, to time.Duration) (int, error) {
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	st, err := cl.Scan(cctx, &beastv1.ScanRequest{StartKey: start, EndKey: math.MaxUint64, Limit: limit})
	if err != nil {
		return 0, err
	}
	n := 0
	for {
		_, err := st.Recv()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		n++
	}
}

func summarize(name string, el time.Duration, cs []*counters) PhaseResult {
	res := PhaseResult{Phase: name, Seconds: el.Seconds()}
	for op := OpType(0); op < numOps; op++ {
		h := NewHistogram()
		var errs, miss, conf uint64
		for _, c := range cs {
			h.Merge(c.hist[op])
			errs += c.errs[op]
			miss += c.misses[op]
			conf += c.conflicts[op]
		}
		if h.Count() == 0 {
			continue
		}
		res.Ops += h.Count()
		res.PerOp = append(res.PerOp, OpStats{
			Op: op.String(), Count: h.Count(), Errors: errs, Misses: miss, Conflicts: conf,
			Throughput: float64(h.Count()) / el.Seconds(),
			MeanUs:     us(h.Mean()), P50Us: us(h.Quantile(.50)), P95Us: us(h.Quantile(.95)),
			P99Us: us(h.Quantile(.99)), MaxUs: us(h.Max()),
		})
	}
	res.Throughput = float64(res.Ops) / el.Seconds()
	return res
}
