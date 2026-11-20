package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

// runRejoin asks a running, fenced BeastDB node (its Studio/web address) to
// rejoin as a follower of the current leader. The node checks that its log is a
// prefix of the leader's and refuses, saying what it holds that the leader
// lacks, if not. The call is audited on the node as replication.rejoin.
//
// The admin password is read from the environment (BEASTDB_ADMIN_PASSWORD) or
// a file, never from an argument, so it does not land in shell history.
func runRejoin(web, user, passwordFile, leader string) error {
	if web == "" || leader == "" {
		return errors.New("--web (the fenced node's web address) and --leader (the current leader's gRPC address) are required")
	}
	pw := os.Getenv("BEASTDB_ADMIN_PASSWORD")
	if passwordFile != "" {
		b, err := os.ReadFile(passwordFile)
		if err != nil {
			return fmt.Errorf("read --password-file: %w", err)
		}
		pw = strings.TrimSpace(string(b))
	}
	if pw == "" {
		return errors.New("admin password required: set BEASTDB_ADMIN_PASSWORD or pass --password-file")
	}
	base, err := url.Parse(strings.TrimRight(web, "/"))
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return fmt.Errorf("--web must be a URL like http://127.0.0.1:8080, got %q", web)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		// The login answers with a redirect; the cookie is all that is needed.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	login, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	resp, err := client.Post(base.String()+"/login", "application/json", bytes.NewReader(login))
	if err != nil {
		return fmt.Errorf("log in to %s: %w", base, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("log in to %s: %s (an admin account is required)", base, resp.Status)
	}

	body, _ := json.Marshal(map[string]string{"leaderAddr": leader})
	resp, err = client.Post(base.String()+"/api/replication/rejoin", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("rejoin: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK     bool   `json:"ok"`
		Code   string `json:"code"`
		Error  string `json:"error"`
		Lost   uint64 `json:"unreplicatedRecords"`
		Result struct {
			Epoch      uint64 `json:"epoch"`
			AppliedLSN uint64 `json:"appliedLSN"`
			LeaderLSN  uint64 `json:"leaderLSN"`
		} `json:"result"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("rejoin: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if !out.OK {
		if out.Code == "rejoin_refused" {
			return fmt.Errorf("%s\nThe node stays fenced and its data is untouched. Nothing was changed", out.Error)
		}
		return fmt.Errorf("rejoin failed (%s): %s", out.Code, out.Error)
	}
	fmt.Printf("Rejoined as a follower of %s at epoch %d (applied LSN %d, leader LSN %d). It will now catch up.\n",
		leader, out.Result.Epoch, out.Result.AppliedLSN, out.Result.LeaderLSN)
	return nil
}
