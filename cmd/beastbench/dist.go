package main

import (
	"math"
	"math/rand"
	"sync/atomic"
)

// KeyChooser picks a record index in [0, n).
type KeyChooser interface {
	Next(r *rand.Rand) uint64
}

// Uniform picks indexes uniformly.
type Uniform struct{ N uint64 }

func (u Uniform) Next(r *rand.Rand) uint64 { return uint64(r.Int63n(int64(u.N))) }

// Zipfian implements the YCSB / Gray et al. generator. Index 0 is the most
// popular item. Parameters are precomputed so Next is O(1) and the value is
// safe for concurrent use (each caller supplies its own *rand.Rand).
type Zipfian struct {
	n                  uint64
	theta, alpha, zeta float64
	zeta2, eta         float64
}

// DefaultZipfTheta is YCSB's default skew constant.
const DefaultZipfTheta = 0.99

func zetaN(n uint64, theta float64) float64 {
	var s float64
	for i := uint64(1); i <= n; i++ {
		s += 1 / math.Pow(float64(i), theta)
	}
	return s
}

// NewZipfian builds a generator over n items.
func NewZipfian(n uint64, theta float64) *Zipfian {
	z := &Zipfian{n: n, theta: theta}
	z.zeta2 = zetaN(2, theta)
	z.zeta = zetaN(n, theta)
	z.alpha = 1 / (1 - theta)
	z.eta = (1 - math.Pow(2/float64(n), 1-theta)) / (1 - z.zeta2/z.zeta)
	return z
}

func (z *Zipfian) Next(r *rand.Rand) uint64 {
	u := r.Float64()
	uz := u * z.zeta
	if uz < 1 {
		return 0
	}
	if uz < 1+math.Pow(0.5, z.theta) {
		return 1 % z.n
	}
	v := uint64(float64(z.n) * math.Pow(z.eta*u-z.eta+1, z.alpha))
	if v >= z.n {
		v = z.n - 1
	}
	return v
}

// Latest favours the most recently inserted records (workload D): it draws a
// zipfian offset back from the current insert frontier.
type Latest struct {
	Zipf     *Zipfian
	Frontier *atomic.Uint64 // number of records currently present
}

func (l Latest) Next(r *rand.Rand) uint64 {
	n := l.Frontier.Load()
	if n == 0 {
		return 0
	}
	off := l.Zipf.Next(r)
	if off >= n {
		off = n - 1
	}
	return n - 1 - off
}
