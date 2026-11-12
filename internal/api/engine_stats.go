package api

import (
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// StorageMetrics exposes physical and buffer-pool health indicators.
type StorageMetrics struct {
	TotalPages     uint64 `json:"totalPages"`
	DiskSizeBytes  uint64 `json:"diskSizeBytes"`
	PoolSize       int    `json:"poolSize"`
	CachedPages    int    `json:"cachedPages"`
	PinnedPages    int    `json:"pinnedPages"`
	DirtyPages     int    `json:"dirtyPages"`
	WALSizeBytes   int64  `json:"walSizeBytes"`
	CurrentLSN     uint64 `json:"currentLSN"`
	ActiveDataPage uint64 `json:"activeDataPage"`

	BufferHits        uint64 `json:"bufferHits"`
	BufferMisses      uint64 `json:"bufferMisses"`
	LastCheckpointLSN uint64 `json:"lastCheckpointLSN"`
	// LastCheckpointUnix is 0 until a checkpoint completes in this process.
	LastCheckpointUnix int64 `json:"lastCheckpointUnix"`
	ReadOnly           bool  `json:"readOnly"`
}

// StorageMetrics collects live runtime storage indicators.
func (e *Engine) StorageMetrics() StorageMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	numPages := e.disk.NumPages()
	poolSize, cached, pinned, dirty := e.bpm.Stats()

	hits, misses := e.bpm.HitMiss()

	return StorageMetrics{
		BufferHits:         hits,
		BufferMisses:       misses,
		LastCheckpointLSN:  e.lastCkptLSN.Load(),
		LastCheckpointUnix: e.lastCkptUnix.Load(),
		ReadOnly:           e.readOnly,
		TotalPages:         numPages,
		DiskSizeBytes:      numPages * storage.PageSize,
		PoolSize:           poolSize,
		CachedPages:        cached,
		PinnedPages:        pinned,
		DirtyPages:         dirty,
		WALSizeBytes:       e.wal.FileSize(),
		CurrentLSN:         e.wal.CurrentLSN(),
		ActiveDataPage:     e.activeDataPage,
	}
}
