package net

import (
	"bufio"
	"bytes"
	"testing"
)

func TestParseSimpleStringAndError(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte("+PONG\r\n-ERR missing key\r\n")))

	val, err := ParseResp(reader)
	if err != nil || val.Type != TypeSimpleString || string(val.Str) != "PONG" {
		t.Fatalf("unexpected simple string: %v, %s", err, string(val.Str))
	}

	val, err = ParseResp(reader)
	if err != nil || val.Type != TypeError || string(val.Str) != "ERR missing key" {
		t.Fatalf("unexpected error: %v, %s", err, string(val.Str))
	}
}

func TestParseInteger(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte(":1000\r\n:-42\r\n")))

	val, err := ParseResp(reader)
	if err != nil || val.Type != TypeInteger || val.Num != 1000 {
		t.Fatalf("unexpected integer: %v, %d", err, val.Num)
	}

	val, err = ParseResp(reader)
	if err != nil || val.Type != TypeInteger || val.Num != -42 {
		t.Fatalf("unexpected negative integer: %v, %d", err, val.Num)
	}
}

func TestParseBulkString(t *testing.T) {
	// Standard bulk string followed by Null bulk string
	data := []byte("$5\r\nhello\r\n$-1\r\n")
	reader := bufio.NewReader(bytes.NewReader(data))

	val, err := ParseResp(reader)
	if err != nil || string(val.Str) != "hello" {
		t.Fatalf("unexpected bulk string: %v, %s", err, string(val.Str))
	}

	val, err = ParseResp(reader)
	if err != nil || !val.Null {
		t.Fatalf("expected Null bulk string, got: %v, null=%v", err, val.Null)
	}
}

func TestParseArrayCommand(t *testing.T) {
	// *3\r\n$3\r\nSET\r\n$4\r\nname\r\n$10\r\nHeisenberg\r\n
	cmd := []byte("*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$10\r\nHeisenberg\r\n")
	reader := bufio.NewReader(bytes.NewReader(cmd))

	val, err := ParseResp(reader)
	if err != nil {
		t.Fatalf("failed to parse array: %v", err)
	}

	if val.Type != TypeArray || len(val.Array) != 3 {
		t.Fatalf("expected array of 3, got len=%d", len(val.Array))
	}

	expected := []string{"SET", "name", "Heisenberg"}
	for i, exp := range expected {
		if string(val.Array[i].Str) != exp {
			t.Fatalf("item %d: expected %q, got %q", i, exp, string(val.Array[i].Str))
		}
	}
}

func TestRespWriter(t *testing.T) {
	var buf bytes.Buffer

	_ = WriteOK(&buf)
	_ = WriteNull(&buf)
	_ = WriteBulkString(&buf, []byte("database"))

	expected := "+OK\r\n$-1\r\n$8\r\ndatabase\r\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func BenchmarkParseRespCommand(b *testing.B) {
	cmd := []byte("*3\r\n$3\r\nSET\r\n$4\r\nuser\r\n$10\r\nHeisenberg\r\n")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(bytes.NewReader(cmd))
		_, _ = ParseResp(reader)
	}
}
