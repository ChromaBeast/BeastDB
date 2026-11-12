package main

import (
	"encoding/binary"
	"time"
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
