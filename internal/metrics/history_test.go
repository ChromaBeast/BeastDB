package metrics

import (
	"testing"
	"time"
)

func TestHistoryRingWindowAndThinning(t *testing.T) {
	h := NewHistory(5)
	base := time.Unix(1_000_000, 0)
	for i := 0; i < 8; i++ { // 3 oldest are evicted
		h.Add(Point{Time: base.Add(time.Duration(i) * time.Minute).Unix(), LSN: uint64(i)})
	}
	now := base.Add(7 * time.Minute)

	all := h.Window(now, time.Hour, 0)
	if len(all) != 5 || all[0].LSN != 3 || all[4].LSN != 7 {
		t.Fatalf("ring should hold LSN 3..7 oldest first: %+v", all)
	}

	recent := h.Window(now, 2*time.Minute, 0) // minutes 5, 6, 7
	if len(recent) != 3 || recent[0].LSN != 5 {
		t.Fatalf("window filter: %+v", recent)
	}

	thin := h.Window(now, time.Hour, 2)
	if len(thin) > 2 || thin[len(thin)-1].LSN != 7 {
		t.Fatalf("thinning must keep the newest point and cap the count: %+v", thin)
	}
	if thin[0].Time > thin[len(thin)-1].Time {
		t.Fatal("thinned points must stay oldest first")
	}
}

func TestRequestTotal(t *testing.T) {
	r := NewRequestMetrics()
	r.Observe("grpc", "Get", "OK", time.Millisecond)
	r.Observe("http", "/x", "200", time.Millisecond)
	r.Observe("http", "/x", "500", time.Millisecond)
	if got := r.Total(); got != 3 {
		t.Fatalf("total %d, want 3", got)
	}
}
