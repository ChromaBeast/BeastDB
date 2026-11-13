package web

import (
	"time"

	"github.com/ChromaBeast/beastdb/internal/metrics"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
)

const (
	// historyInterval is how often the server samples its own gauges.
	historyInterval = 10 * time.Second
	// historyCapacity keeps 24 hours at historyInterval.
	historyCapacity = int(24 * time.Hour / historyInterval)
)

// startSampler records a metrics.Point every interval until the returned stop
// function is called. The request rate comes from the shared request counter.
func startSampler(engine EngineBackend, repl handler.ReplicationSource, hist *metrics.History, interval time.Duration) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		last := metrics.Default.Total()
		lastAt := time.Now()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				m := engine.StorageMetrics()
				total := metrics.Default.Total()
				rate := 0.0
				if dt := now.Sub(lastAt).Seconds(); dt > 0 && total >= last {
					rate = float64(total-last) / dt
				}
				last, lastAt = total, now

				var lag uint64
				if repl != nil {
					for _, rep := range repl.Replicas() {
						if rep.Connected && m.CurrentLSN > rep.AckedLSN && m.CurrentLSN-rep.AckedLSN > lag {
							lag = m.CurrentLSN - rep.AckedLSN
						}
					}
				}
				wal := uint64(0)
				if m.WALSizeBytes > 0 {
					wal = uint64(m.WALSizeBytes)
				}
				hist.Add(metrics.Point{
					Time: now.Unix(), LSN: m.CurrentLSN, WALBytes: wal,
					DirtyPages: m.DirtyPages, CachedPages: m.CachedPages,
					ReqPerSec: rate, ReplicaLag: lag,
				})
			}
		}
	}()
	return func() { close(done) }
}
