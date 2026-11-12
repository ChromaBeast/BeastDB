package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
