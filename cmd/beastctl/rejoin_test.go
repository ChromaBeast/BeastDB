package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rejoinServer(t *testing.T, answer string, status int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["username"] != "admin" || in["password"] != "pw" {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "s", Value: "1"})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("/api/replication/rejoin", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("s"); err != nil || c.Value != "1" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunRejoin(t *testing.T) {
	t.Setenv("BEASTDB_ADMIN_PASSWORD", "pw")
	ok := rejoinServer(t, `{"ok":true,"result":{"epoch":2,"appliedLSN":5,"leaderLSN":6}}`, 200)
	if err := runRejoin(ok.URL, "admin", "", "10.0.0.1:50051"); err != nil {
		t.Fatalf("success path: %v", err)
	}

	refused := rejoinServer(t, `{"ok":false,"code":"rejoin_refused","error":"holds 2 record(s) the leader never had","unreplicatedRecords":2}`, 409)
	err := runRejoin(refused.URL, "admin", "", "10.0.0.1:50051")
	if err == nil || !strings.Contains(err.Error(), "never had") || !strings.Contains(err.Error(), "stays fenced") {
		t.Fatalf("refusal must reach the operator, got %v", err)
	}

	if err := runRejoin(ok.URL, "admin", "", ""); err == nil {
		t.Fatal("missing --leader must fail")
	}
	t.Setenv("BEASTDB_ADMIN_PASSWORD", "wrong")
	if err := runRejoin(ok.URL, "admin", "", "x:1"); err == nil || !strings.Contains(err.Error(), "log in") {
		t.Fatalf("bad login: %v", err)
	}
}
