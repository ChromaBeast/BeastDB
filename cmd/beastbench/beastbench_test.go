package main

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

func TestHistogramQuantiles(t *testing.T) {
	h := NewHistogram()
	var vals []int
	for i := 1; i <= 100000; i++ {
		d := time.Duration(i) * time.Microsecond
		h.Record(d)
		vals = append(vals, int(d))
	}
	sort.Ints(vals)
	for _, q := range []float64{.5, .95, .99} {
		want := float64(vals[int(q*float64(len(vals)))-1])
		got := float64(h.Quantile(q))
		if got < want || got > want*1.02 {
			t.Errorf("q=%v got %v want ~%v", q, got, want)
		}
	}
	if h.Max() != 100000*time.Microsecond || h.Min() != time.Microsecond {
		t.Errorf("min/max wrong: %v %v", h.Min(), h.Max())
	}
	if h.Count() != 100000 {
		t.Errorf("count %d", h.Count())
	}
}

func TestHistogramBucketsRoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 127, 128, 255, 256, 257, 1000, 1 << 20, 1<<40 + 12345, 1<<63 + 5} {
		lo, hi := bucketBounds(bucketIndex(v))
		if v < lo || v > hi {
			t.Errorf("v=%d not in [%d,%d]", v, lo, hi)
		}
	}
}

func TestHistogramMergeAndEmpty(t *testing.T) {
	a, b := NewHistogram(), NewHistogram()
	if a.Quantile(.5) != 0 || a.Mean() != 0 || a.Min() != 0 {
		t.Error("empty histogram should report zeros")
	}
	a.Record(time.Millisecond)
	b.Record(3 * time.Millisecond)
	a.Merge(b)
	if a.Count() != 2 || a.Max() != 3*time.Millisecond || a.Min() != time.Millisecond {
		t.Errorf("merge wrong: %d %v %v", a.Count(), a.Min(), a.Max())
	}
}

func TestZipfianSkewAndRange(t *testing.T) {
	const n = 1000
	z := NewZipfian(n, DefaultZipfTheta)
	r := rand.New(rand.NewSource(1))
	counts := make([]int, n)
	for i := 0; i < 200000; i++ {
		v := z.Next(r)
		if v >= n {
			t.Fatalf("out of range: %d", v)
		}
		counts[v]++
	}
	if counts[0] < counts[10] || counts[10] < counts[500] {
		t.Errorf("not monotonically skewed: %d %d %d", counts[0], counts[10], counts[500])
	}
	if counts[0] < 200000/20 {
		t.Errorf("item 0 too cold: %d", counts[0])
	}
}

func TestUniformAndDeterminism(t *testing.T) {
	u := Uniform{N: 10}
	r := rand.New(rand.NewSource(7))
	seen := map[uint64]bool{}
	for i := 0; i < 1000; i++ {
		seen[u.Next(r)] = true
	}
	if len(seen) != 10 {
		t.Errorf("uniform missed buckets: %d", len(seen))
	}
	z := NewZipfian(100, .9)
	a, b := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	for i := 0; i < 100; i++ {
		if z.Next(a) != z.Next(b) {
			t.Fatal("same seed must give same sequence")
		}
	}
}

func TestLatestFavoursNewest(t *testing.T) {
	f := &atomic.Uint64{}
	f.Store(1000)
	l := Latest{Zipf: NewZipfian(1000, DefaultZipfTheta), Frontier: f}
	r := rand.New(rand.NewSource(2))
	newest := 0
	for i := 0; i < 10000; i++ {
		v := l.Next(r)
		if v >= 1000 {
			t.Fatalf("out of range %d", v)
		}
		if v >= 990 {
			newest++
		}
	}
	if newest < 2000 {
		t.Errorf("latest not skewed to newest: %d", newest)
	}
}

func TestWorkloadMixes(t *testing.T) {
	for name, w := range workloads {
		var sum float64
		for _, p := range w.Mix {
			sum += p
		}
		if sum < .999 || sum > 1.001 {
			t.Errorf("workload %s mix sums to %v", name, sum)
		}
	}
	w := workloads["A"]
	if w.pick(0.1) != OpRead || w.pick(0.9) != OpUpdate {
		t.Error("A pick wrong")
	}
	if _, err := lookupWorkload("Z"); err == nil {
		t.Error("expected error for unknown workload")
	}
}

func TestSmokeAllWorkloads(t *testing.T) {
	for _, wl := range []string{"A", "B", "C", "D", "E", "F"} {
		wl := wl
		t.Run(wl, func(t *testing.T) {
			var out, errb bytes.Buffer
			err := realMain([]string{"--self-contained", "--json", "--workload", wl, "--records", "200",
				"--ops", "300", "--warmup", "20", "--threads", "4", "--value-size", "64", "--seed", "5", "--max-scan-len", "10"}, &out, &errb)
			if err != nil {
				t.Fatalf("%v (%s)", err, errb.String())
			}
			var rep Report
			if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
				t.Fatal(err)
			}
			if len(rep.Phases) != 2 || rep.Phases[1].Ops != 300 {
				t.Fatalf("unexpected phases: %+v", rep.Phases)
			}
			for _, p := range rep.Phases {
				for _, o := range p.PerOp {
					if o.Errors != 0 {
						t.Errorf("%s/%s: %d errors", p.Phase, o.Op, o.Errors)
					}
					if o.P50Us > o.P99Us || o.P99Us > o.MaxUs {
						t.Errorf("%s/%s percentiles not monotonic: %+v", p.Phase, o.Op, o)
					}
				}
			}
			if rep.Phases[0].Ops != 200 {
				t.Errorf("load ops %d", rep.Phases[0].Ops)
			}
		})
	}
}

func TestTextOutputAndBadFlags(t *testing.T) {
	var out, errb bytes.Buffer
	if err := realMain([]string{"--self-contained", "--records", "50", "--ops", "50", "--threads", "2"}, &out, &errb); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("not a competitive benchmark")) || !bytes.Contains(out.Bytes(), []byte("p99(us)")) {
		t.Errorf("text output missing pieces:\n%s", out.String())
	}
	if err := realMain([]string{"--workload", "Q"}, &out, &errb); err == nil {
		t.Error("expected error")
	}
	if err := realMain([]string{"--value-size", "1"}, &out, &errb); err == nil {
		t.Error("expected error")
	}
}
