package main

import (
	"math"
	"math/bits"
	"time"
)

// Histogram is a small HDR-style log-linear histogram of nanosecond values.
// Each power-of-two range is split into 128 linear sub-buckets, bounding the
// relative quantile error to under 1%. It is not safe for concurrent use;
// give each worker its own and Merge afterwards.
type Histogram struct {
	counts [histBuckets]uint64
	total  uint64
	sum    float64
	min    uint64
	max    uint64
}

const (
	histSubBits = 7
	histSub     = 1 << histSubBits
	histBuckets = (64-histSubBits)*histSub + 2*histSub
)

// NewHistogram returns an empty histogram.
func NewHistogram() *Histogram { return &Histogram{min: math.MaxUint64} }

func bucketIndex(v uint64) int {
	shift := bits.Len64(v) - histSubBits - 1
	if shift < 0 {
		shift = 0
	}
	return shift*histSub + int(v>>uint(shift))
}

// bucketBounds returns the inclusive value range covered by bucket idx.
func bucketBounds(idx int) (lo, hi uint64) {
	if idx < 2*histSub {
		return uint64(idx), uint64(idx)
	}
	shift := uint(idx/histSub - 1)
	m := uint64(idx%histSub + histSub)
	lo = m << shift
	return lo, lo + (uint64(1) << shift) - 1
}

// Record adds one observation.
func (h *Histogram) Record(d time.Duration) {
	if d < 0 {
		d = 0
	}
	v := uint64(d)
	h.counts[bucketIndex(v)]++
	h.total++
	h.sum += float64(v)
	if v < h.min {
		h.min = v
	}
	if v > h.max {
		h.max = v
	}
}

// Count returns the number of observations.
func (h *Histogram) Count() uint64 { return h.total }

// Max returns the largest observation (exact).
func (h *Histogram) Max() time.Duration { return time.Duration(h.max) }

// Min returns the smallest observation (exact), or 0 when empty.
func (h *Histogram) Min() time.Duration {
	if h.total == 0 {
		return 0
	}
	return time.Duration(h.min)
}

// Mean returns the arithmetic mean.
func (h *Histogram) Mean() time.Duration {
	if h.total == 0 {
		return 0
	}
	return time.Duration(h.sum / float64(h.total))
}

// Quantile returns the value at quantile q in [0,1] (bucket upper bound,
// clamped to the exact max).
func (h *Histogram) Quantile(q float64) time.Duration {
	if h.total == 0 {
		return 0
	}
	if q >= 1 {
		return h.Max()
	}
	rank := uint64(math.Ceil(q * float64(h.total)))
	if rank < 1 {
		rank = 1
	}
	var seen uint64
	for i, c := range h.counts {
		if c == 0 {
			continue
		}
		seen += c
		if seen >= rank {
			_, hi := bucketBounds(i)
			if hi > h.max {
				hi = h.max
			}
			return time.Duration(hi)
		}
	}
	return h.Max()
}

// Merge adds all observations of o into h.
func (h *Histogram) Merge(o *Histogram) {
	for i, c := range o.counts {
		h.counts[i] += c
	}
	h.total += o.total
	h.sum += o.sum
	if o.total > 0 && o.min < h.min {
		h.min = o.min
	}
	if o.max > h.max {
		h.max = o.max
	}
}
