package main

import (
	"context"
	"io"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

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
