package logbuf

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
)

func TestRingBoundedAndSeqMonotonic(t *testing.T) {
	b := New(5)
	for i := 0; i < 12; i++ {
		fmt.Fprintf(b, "line %d\n", i)
	}
	r := b.Snapshot(Query{})
	if len(r.Entries) != 5 || r.Latest != 12 {
		t.Fatalf("got %d entries latest %d", len(r.Entries), r.Latest)
	}
	for i, e := range r.Entries {
		if e.Seq != uint64(8+i) || e.Message != fmt.Sprintf("line %d", 7+i) {
			t.Fatalf("entry %d = %+v", i, e)
		}
	}
}

func TestSinceLimitFilters(t *testing.T) {
	b := New(50)
	b.Write([]byte("hello\nFATAL: boom\nWarning: careful\nHELLO again\n"))
	if got := b.Snapshot(Query{Since: 2}).Entries; len(got) != 2 {
		t.Fatalf("since: %d", len(got))
	}
	if got := b.Snapshot(Query{Level: "error"}).Entries; len(got) != 1 || got[0].Message != "FATAL: boom" {
		t.Fatalf("level: %+v", got)
	}
	if got := b.Snapshot(Query{Level: "warn"}).Entries; len(got) != 1 {
		t.Fatalf("warn: %+v", got)
	}
	if got := b.Snapshot(Query{Text: "hello"}).Entries; len(got) != 2 {
		t.Fatalf("text: %+v", got)
	}
	got := b.Snapshot(Query{Limit: 1}).Entries
	if len(got) != 1 || got[0].Seq != 4 {
		t.Fatalf("limit should keep newest: %+v", got)
	}
}

func TestTeePassesThroughAndParsesStdLog(t *testing.T) {
	b := New(10)
	var dst bytes.Buffer
	l := log.New(b.Tee(&dst), "", log.LstdFlags)
	l.Printf("Starting up")
	if !strings.Contains(dst.String(), "Starting up") {
		t.Fatalf("dst missing output: %q", dst.String())
	}
	e := b.Snapshot(Query{}).Entries
	if len(e) != 1 || e[0].Message != "Starting up" || e[0].Level != "info" || e[0].Time.IsZero() {
		t.Fatalf("%+v", e)
	}
}

func TestLevelDetection(t *testing.T) {
	cases := map[string]string{
		"⚠️  SECURITY WARNING: x":                      "warn",
		"[Studio] Warning: failed to parse x":          "warn",
		"Failed to sync admin password: x":             "error",
		"BeastDB gRPC server listening on 127.0.0.1:1": "info",
		"Follower sync stream dropped: x":              "info",
	}
	for msg, want := range cases {
		if got := detectLevel(msg); got != want {
			t.Errorf("%q: got %s want %s", msg, got, want)
		}
	}
}

func TestPartialLinesAndOversize(t *testing.T) {
	b := New(10)
	b.Write([]byte("abc"))
	if len(b.Snapshot(Query{}).Entries) != 0 {
		t.Fatal("partial line stored early")
	}
	b.Write([]byte("def\n"))
	b.Write([]byte(strings.Repeat("x", maxMessageBytes*2) + "\n"))
	e := b.Snapshot(Query{}).Entries
	if e[0].Message != "abcdef" || len(e[1].Message) > maxMessageBytes+4 {
		t.Fatalf("unexpected %q / %d", e[0].Message, len(e[1].Message))
	}
}

func TestSecretsRedacted(t *testing.T) {
	b := New(10)
	b.AddSecret("s3cr3t-token")
	b.Write([]byte("token is s3cr3t-token ok\n"))
	m := b.Snapshot(Query{}).Entries[0].Message
	if strings.Contains(m, "s3cr3t") || !strings.Contains(m, "[redacted]") {
		t.Fatal(m)
	}
}

func TestConcurrent(t *testing.T) {
	b := New(100)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				fmt.Fprintf(b, "msg %d\n", i)
				b.Snapshot(Query{Since: 10})
			}
		}()
	}
	wg.Wait()
	if r := b.Snapshot(Query{Limit: 2000}); len(r.Entries) != 100 || r.Latest != 4000 {
		t.Fatalf("%d %d", len(r.Entries), r.Latest)
	}
}
