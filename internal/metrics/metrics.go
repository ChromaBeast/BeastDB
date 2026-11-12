// Package metrics is a tiny dependency-free Prometheus text-format (0.0.4)
// exposition layer: request counters, latency histograms and gauge helpers.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultBuckets are latency histogram upper bounds in seconds.
var DefaultBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type hist struct {
	counts []uint64 // per-bucket (non-cumulative), len = len(bounds)+1 (+Inf)
	sum    float64
	n      uint64
}

type reqKey struct{ proto, method, code string }

// RequestMetrics records request counts (by protocol, method, status code)
// and latency histograms (by protocol, method).
type RequestMetrics struct {
	mu    sync.Mutex
	count map[reqKey]uint64
	hists map[[2]string]*hist
}

// NewRequestMetrics creates an empty collector.
func NewRequestMetrics() *RequestMetrics {
	return &RequestMetrics{count: map[reqKey]uint64{}, hists: map[[2]string]*hist{}}
}

// Observe records one finished request. code is a short status string
// (gRPC code name or HTTP status number).
func (r *RequestMetrics) Observe(proto, method, code string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count[reqKey{proto, method, code}]++
	hk := [2]string{proto, method}
	h := r.hists[hk]
	if h == nil {
		h = &hist{counts: make([]uint64, len(DefaultBuckets)+1)}
		r.hists[hk] = h
	}
	s := d.Seconds()
	i := sort.SearchFloat64s(DefaultBuckets, s) // first bound >= s
	h.counts[i]++
	h.sum += s
	h.n++
}

// Default is the process-wide request collector shared by the gRPC
// interceptors and the HTTP middleware.
var Default = NewRequestMetrics()

// InFlight tracks currently executing gRPC requests.
var InFlight atomic.Int64

// Writer emits Prometheus text exposition.
type Writer struct {
	w    io.Writer
	seen map[string]bool
}

// NewWriter wraps w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w, seen: map[string]bool{}} }

func (w *Writer) header(name, help, typ string) {
	if w.seen[name] {
		return
	}
	w.seen[name] = true
	fmt.Fprintf(w.w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

// Escape escapes a label value.
func Escape(v string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(v)
}

func labels(kv []string) string {
	if len(kv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, kv[i]+`="`+Escape(kv[i+1])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func fmtFloat(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// Gauge writes a gauge sample. kv are alternating label names/values.
func (w *Writer) Gauge(name, help string, v float64, kv ...string) {
	w.header(name, help, "gauge")
	fmt.Fprintf(w.w, "%s%s %s\n", name, labels(kv), fmtFloat(v))
}

// Counter writes a counter sample.
func (w *Writer) Counter(name, help string, v float64, kv ...string) {
	w.header(name, help, "counter")
	fmt.Fprintf(w.w, "%s%s %s\n", name, labels(kv), fmtFloat(v))
}

// WriteRequests emits the request counter and latency histogram families.
func (r *RequestMetrics) WriteRequests(w *Writer) {
	r.mu.Lock()
	keys := make([]reqKey, 0, len(r.count))
	counts := make(map[reqKey]uint64, len(r.count))
	for k, v := range r.count {
		keys = append(keys, k)
		counts[k] = v
	}
	hk := make([][2]string, 0, len(r.hists))
	hs := make(map[[2]string]hist, len(r.hists))
	for k, h := range r.hists {
		hk = append(hk, k)
		hs[k] = hist{counts: append([]uint64(nil), h.counts...), sum: h.sum, n: h.n}
	}
	r.mu.Unlock()

	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.proto != b.proto {
			return a.proto < b.proto
		}
		if a.method != b.method {
			return a.method < b.method
		}
		return a.code < b.code
	})
	for _, k := range keys {
		w.Counter("beastdb_requests_total", "Requests handled, by protocol, method and status code.",
			float64(counts[k]), "protocol", k.proto, "method", k.method, "code", k.code)
	}
	sort.Slice(hk, func(i, j int) bool {
		if hk[i][0] != hk[j][0] {
			return hk[i][0] < hk[j][0]
		}
		return hk[i][1] < hk[j][1]
	})
	const name = "beastdb_request_duration_seconds"
	for _, k := range hk {
		h := hs[k]
		w.header(name, "Request latency in seconds.", "histogram")
		var cum uint64
		for i, b := range DefaultBuckets {
			cum += h.counts[i]
			fmt.Fprintf(w.w, "%s_bucket%s %d\n", name, labels([]string{"protocol", k[0], "method", k[1], "le", fmtFloat(b)}), cum)
		}
		cum += h.counts[len(DefaultBuckets)]
		fmt.Fprintf(w.w, "%s_bucket%s %d\n", name, labels([]string{"protocol", k[0], "method", k[1], "le", "+Inf"}), cum)
		fmt.Fprintf(w.w, "%s_sum%s %s\n", name, labels([]string{"protocol", k[0], "method", k[1]}), fmtFloat(h.sum))
		fmt.Fprintf(w.w, "%s_count%s %d\n", name, labels([]string{"protocol", k[0], "method", k[1]}), h.n)
	}
}
