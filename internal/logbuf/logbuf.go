// Package logbuf keeps a bounded in-memory tail of the process log so the
// Studio can show it. It tees the standard logger: the original writer
// (stderr) still receives every byte unchanged.
package logbuf

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
)

// DefaultCapacity is the number of entries retained.
const DefaultCapacity = 2000

// maxMessageBytes caps one stored message so memory stays bounded
// (capacity * maxMessageBytes worst case).
const maxMessageBytes = 4096

// Entry is one captured log line.
type Entry struct {
	Seq     uint64    `json:"seq"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Buffer is a fixed-capacity ring of log entries. Safe for concurrent use.
type Buffer struct {
	mu      sync.Mutex
	ring    []Entry
	start   int // index of oldest entry
	n       int // entries stored
	nextSeq uint64
	secrets []string
	partial []byte // bytes of an unterminated line
}

// New returns a Buffer holding at most capacity entries.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Buffer{ring: make([]Entry, capacity), nextSeq: 1}
}

// AddSecret registers a value that must never appear in stored entries; any
// occurrence is replaced with "[redacted]". Empty values are ignored.
func (b *Buffer) AddSecret(s string) {
	if s == "" {
		return
	}
	b.mu.Lock()
	b.secrets = append(b.secrets, s)
	b.mu.Unlock()
}

// Tee returns a writer that forwards to dst unchanged and records each
// complete line in the buffer. A dst write error is returned as-is.
func (b *Buffer) Tee(dst io.Writer) io.Writer { return &tee{b: b, dst: dst} }

type tee struct {
	b   *Buffer
	dst io.Writer
}

func (t *tee) Write(p []byte) (int, error) {
	n, err := t.dst.Write(p)
	t.b.Write(p) // never fails
	return n, err
}

// Write implements io.Writer: it splits p into lines and appends each one.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data := p
	if len(b.partial) > 0 {
		data = append(append([]byte(nil), b.partial...), p...)
		b.partial = nil
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		b.appendLocked(string(data[:i]))
		data = data[i+1:]
	}
	if len(data) > 0 {
		if len(data) > maxMessageBytes {
			data = data[:maxMessageBytes]
		}
		b.partial = append([]byte(nil), data...)
	}
	return len(p), nil
}

func (b *Buffer) appendLocked(line string) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}
	for _, s := range b.secrets {
		line = strings.ReplaceAll(line, s, "[redacted]")
	}
	if len(line) > maxMessageBytes {
		line = line[:maxMessageBytes] + "…"
	}
	ts, msg := splitTimestamp(line)
	e := Entry{Seq: b.nextSeq, Time: ts, Level: detectLevel(msg), Message: msg}
	b.nextSeq++
	if b.n < len(b.ring) {
		b.ring[(b.start+b.n)%len(b.ring)] = e
		b.n++
		return
	}
	b.ring[b.start] = e
	b.start = (b.start + 1) % len(b.ring)
}

// splitTimestamp strips the std logger's "2006/01/02 15:04:05 " prefix when
// present and parses it; otherwise it stamps the line with the current time.
func splitTimestamp(line string) (time.Time, string) {
	const layout = "2006/01/02 15:04:05"
	if len(line) > len(layout) && line[len(layout)] == ' ' {
		if t, err := time.ParseInLocation(layout, line[:len(layout)], time.Local); err == nil {
			return t, line[len(layout)+1:]
		}
	}
	return time.Now(), line
}

// detectLevel only trusts explicit markers this server actually emits
// ("FATAL:", "Warning:", "SECURITY WARNING", "Failed ..."); else info.
func detectLevel(msg string) string {
	m := strings.TrimSpace(strings.TrimLeft(msg, "⚠️🔒🔐 ️"))
	up := strings.ToUpper(m)
	switch {
	case strings.HasPrefix(up, "FATAL"), strings.HasPrefix(up, "PANIC"),
		strings.HasPrefix(up, "ERROR"), strings.HasPrefix(up, "FAILED"):
		return "error"
	case strings.HasPrefix(up, "WARN"), strings.HasPrefix(up, "SECURITY WARNING"),
		strings.Contains(up, "] WARNING:"):
		return "warn"
	case strings.HasPrefix(up, "DEBUG"):
		return "debug"
	}
	return "info"
}

// Query selects entries.
type Query struct {
	// Since returns only entries with Seq > Since.
	Since uint64
	// Limit caps the result (newest entries win); <=0 means 500, max 2000.
	Limit int
	// Level, if set ("debug","info","warn","error"), matches that level exactly.
	Level string
	// Text is a case-insensitive substring filter on the message.
	Text string
}

// Result is a query answer. Latest is the highest seq stored (0 if empty), so
// a client can poll with since=Latest even when filters hid every entry.
type Result struct {
	Entries []Entry
	Latest  uint64
}

// Snapshot returns entries oldest-first matching q.
func (b *Buffer) Snapshot(q Query) Result {
	limit := q.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > DefaultCapacity {
		limit = DefaultCapacity
	}
	text := strings.ToLower(q.Text)
	b.mu.Lock()
	defer b.mu.Unlock()
	res := Result{Latest: b.nextSeq - 1}
	var out []Entry
	for i := b.n - 1; i >= 0 && len(out) < limit; i-- { // newest first
		e := b.ring[(b.start+i)%len(b.ring)]
		if e.Seq <= q.Since {
			break
		}
		if q.Level != "" && e.Level != q.Level {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(e.Message), text) {
			continue
		}
		out = append(out, e)
	}
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	res.Entries = out
	return res
}
