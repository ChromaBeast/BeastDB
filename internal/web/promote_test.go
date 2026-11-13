package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/replication"
)

type fakeCluster struct {
	status replication.Status
	res    replication.PromoteResult
	err    error
	calls  []replication.PromoteOptions
}

func (f *fakeCluster) Status() replication.Status { return f.status }
func (f *fakeCluster) Promote(_ context.Context, o replication.PromoteOptions) (replication.PromoteResult, error) {
	f.calls = append(f.calls, o)
	return f.res, f.err
}

func postJSON(srv *Server, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func promoteServer(t *testing.T, fc *fakeCluster) (*Server, *audit.Log, []*http.Cookie, []*http.Cookie) {
	t.Helper()
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	srv, _, admin, viewer := opsServerWith(t, Options{Cluster: fc, Audit: log})
	return srv, log, admin, viewer
}

func promoteEntries(t *testing.T, log *audit.Log) []audit.Entry {
	t.Helper()
	es, err := log.List(audit.Query{Action: audit.ActionPromote})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestPromoteEndpointIsAdminOnly(t *testing.T) {
	fc := &fakeCluster{}
	srv, log, admin, viewer := promoteServer(t, fc)
	if rec := postJSON(srv, "/api/replication/promote", "{}", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous: want 403, got %d", rec.Code)
	}
	if rec := postJSON(srv, "/api/replication/promote", "{}", viewer); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: want 403, got %d", rec.Code)
	}
	if rec := do(srv, http.MethodGet, "/api/replication/promote", admin); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: want 405, got %d", rec.Code)
	}
	if len(fc.calls) != 0 || len(promoteEntries(t, log)) != 0 {
		t.Fatal("a rejected request must not reach Promote or the audit log")
	}
}

func TestPromoteEndpointAuditsSuccess(t *testing.T) {
	fc := &fakeCluster{
		status: replication.Status{NodeID: "r1", Role: replication.RoleLeader, Epoch: 2},
		res:    replication.PromoteResult{Epoch: 2, OldLeaderFenced: true, FinalLSN: 40, AppliedLSN: 40},
	}
	srv, log, admin, _ := promoteServer(t, fc)
	rec := postJSON(srv, "/api/replication/promote", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK     bool                      `json:"ok"`
		Result replication.PromoteResult `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.OK || body.Result.Epoch != 2 {
		t.Fatalf("body %s (%v)", rec.Body.String(), err)
	}
	es := promoteEntries(t, log)
	if len(es) != 1 || es[0].Actor != "root" || es[0].Target != "r1" ||
		!strings.Contains(es[0].Detail, "epoch 2") || !strings.Contains(es[0].Detail, "fenced at LSN 40") {
		t.Fatalf("audit entries: %+v", es)
	}
	if fc.calls[0].AcceptDataLoss {
		t.Fatal("AcceptDataLoss must default to false")
	}
}

func TestPromoteEndpointAuditsUnfencedPromotionLoudly(t *testing.T) {
	fc := &fakeCluster{
		status: replication.Status{NodeID: "r1"},
		res:    replication.PromoteResult{Epoch: 2, DataLossPossible: true, LeaderLSNKnown: 90, AppliedLSN: 88},
	}
	srv, log, admin, _ := promoteServer(t, fc)
	if rec := postJSON(srv, "/api/replication/promote", `{"acceptDataLoss":true}`, admin); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if !fc.calls[0].AcceptDataLoss {
		t.Fatal("acceptDataLoss was not passed through")
	}
	es := promoteEntries(t, log)
	if len(es) != 1 || !strings.Contains(es[0].Detail, "WITHOUT fencing") || !strings.Contains(es[0].Detail, "data loss possible") {
		t.Fatalf("audit entries: %+v", es)
	}
}

func TestPromoteEndpointReportsAndAuditsRefusal(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{replication.ErrLeaderUnreachable, "leader_unreachable"},
		{replication.ErrNotFollower, "not_follower"},
		{replication.ErrCatchUpTimeout, "catch_up_timeout"},
		{replication.ErrFenceRefused, "fence_refused"},
		{errors.New("disk full"), "failed"},
	}
	for _, tc := range cases {
		fc := &fakeCluster{status: replication.Status{NodeID: "r1"}, err: tc.err}
		srv, log, admin, _ := promoteServer(t, fc)
		rec := postJSON(srv, "/api/replication/promote", "{}", admin)
		wantStatus := http.StatusConflict
		if tc.code == "failed" {
			wantStatus = http.StatusInternalServerError
		}
		if rec.Code != wantStatus || !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("%v: %d %s", tc.err, rec.Code, rec.Body.String())
		}
		es := promoteEntries(t, log)
		if len(es) != 1 || !strings.HasPrefix(es[0].Detail, "refused:") {
			t.Fatalf("%v: refusal not audited: %+v", tc.err, es)
		}
	}
}

func TestPromoteEndpointWithoutClusterIsNotImplemented(t *testing.T) {
	srv, _, admin, _ := opsServer(t, fakeReplicas{}, false)
	if rec := postJSON(srv, "/api/replication/promote", "{}", admin); rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}

func TestReplicationStatusIncludesClusterAndHidesReplicasOnFollower(t *testing.T) {
	fc := &fakeCluster{status: replication.Status{NodeID: "r1", Role: replication.RoleFollower, Epoch: 3, Promotable: false, PromoteBlockedReason: "Behind the leader by 4 records"}}
	src := fakeReplicas{list: []replication.ReplicaInfo{{ID: "x"}}}
	srv, _, admin, _ := opsServerWith(t, Options{Cluster: fc, Replication: src})
	rec := do(srv, http.MethodGet, "/api/replication", admin)
	var body struct {
		Role    string              `json:"role"`
		Tracked bool                `json:"tracked"`
		Cluster *replication.Status `json:"cluster"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Role != "follower" || body.Tracked || body.Cluster == nil || body.Cluster.Epoch != 3 || body.Cluster.PromoteBlockedReason == "" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
