package handler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/metrics"
)

var sampleRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{([a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*"(,|))*\})? (-?[0-9][0-9eE.+-]*|\+Inf|-Inf|NaN)$`)

// validateExposition checks Prometheus text format basics and histogram
// consistency (cumulative buckets, +Inf bucket == _count).
func validateExposition(t *testing.T, body string) {
	t.Helper()
	typed := map[string]string{}
	infBucket := map[string]string{}
	count := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if line == "" {
			t.Fatalf("blank line in exposition")
		}
		if strings.HasPrefix(line, "# TYPE ") {
			f := strings.Fields(line)
			if len(f) != 4 {
				t.Fatalf("bad TYPE line %q", line)
			}
			if _, dup := typed[f[2]]; dup {
				t.Fatalf("duplicate TYPE for %s", f[2])
			}
			typed[f[2]] = f[3]
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if !sampleRe.MatchString(line) {
			t.Fatalf("invalid sample line %q", line)
		}
		if strings.Contains(line, `le="+Inf"`) {
			i := strings.LastIndex(line, " ")
			key := strings.Replace(line[:i], `,le="+Inf"`, "", 1)
			infBucket[strings.Replace(key, "_bucket", "", 1)] = line[i+1:]
		} else if strings.Contains(line, "_count") {
			i := strings.LastIndex(line, " ")
			count[strings.Replace(line[:i], "_count", "", 1)] = line[i+1:]
		}
	}
	for k, v := range infBucket {
		if count[k] != v {
			t.Fatalf("histogram %s: +Inf bucket %s != count %s", k, v, count[k])
		}
	}
	if len(infBucket) == 0 {
		t.Fatal("no histogram series found")
	}
	// Bucket monotonicity.
	var prevKey string
	var prev float64
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "_bucket{") {
			continue
		}
		i := strings.LastIndex(line, " ")
		v, _ := strconv.ParseFloat(line[i+1:], 64)
		j := strings.Index(line, ",le=")
		key := line[:j]
		if key == prevKey && v < prev {
			t.Fatalf("non-cumulative buckets at %q", line)
		}
		prevKey, prev = key, v
	}
}

func TestMetricsAuthAndFormat(t *testing.T) {
	metrics.Default.Observe("grpc", "/beastdb.v1.KV/Get", "OK", 3*time.Millisecond)
	metrics.Default.Observe("grpc", "/beastdb.v1.KV/Get", "NotFound", 30*time.Second)
	metrics.Default.Observe("http", "/healthz", "200", time.Millisecond)

	h := MetricsHandler(&mockEngineReader{lsn: 7}, MetricsConfig{
		Role: "follower", Version: `v"1`, DataDir: t.TempDir(), Tokens: []string{"s3cret", ""},
	})

	// No credentials, and an empty-bearer attempt, must be rejected.
	for _, hdr := range []string{"", "Bearer ", "Bearer wrong", "Basic s3cret"} {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("auth header %q: got %d want 401", hdr, rec.Code)
		}
	}
	// Loopback peers get no special treatment.
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("loopback anonymous: got %d want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("content type %q", ct)
	}
	body := rec.Body.String()
	validateExposition(t, body)
	for _, want := range []string{
		`beastdb_build_info{version="v\"1",role="follower"} 1`,
		"beastdb_wal_size_bytes ", "beastdb_wal_current_lsn ", "beastdb_buffer_pool_dirty_pages ",
		"beastdb_buffer_pool_pinned_pages ", "beastdb_replica_applied_lsn 7",
		`beastdb_requests_total{protocol="grpc",method="/beastdb.v1.KV/Get",code="NotFound"} 1`,
		`beastdb_request_duration_seconds_bucket{protocol="grpc",method="/beastdb.v1.KV/Get",le="+Inf"} 2`,
		"beastdb_disk_free_bytes ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in output", want)
		}
	}

	if rec := httptest.NewRecorder(); func() int {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/metrics", nil))
		return rec.Code
	}() != http.StatusMethodNotAllowed {
		t.Fatal("POST should be 405")
	}
}

func TestMetricsNoTokensRejectsBearer(t *testing.T) {
	h := MetricsHandler(&mockEngineReader{}, MetricsConfig{Role: "leader", Version: "x"})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", rec.Code)
	}
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/teapot", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := HTTPMetrics(mux)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/teapot", nil))
	var b strings.Builder
	metrics.Default.WriteRequests(metrics.NewWriter(&b))
	if !strings.Contains(b.String(), `beastdb_requests_total{protocol="http",method="/teapot",code="418"}`) {
		t.Fatalf("http request not recorded:\n%s", b.String())
	}
}
