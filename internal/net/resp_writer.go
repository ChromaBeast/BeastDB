package net

import (
	"fmt"
	"io"
	"strconv"
)

var (
	crlf     = []byte("\r\n")
	nullBulk = []byte("$-1\r\n")
	okString = []byte("+OK\r\n")
)

// WriteOK writes standard "+OK\r\n" status response.
func WriteOK(w io.Writer) error {
	_, err := w.Write(okString)
	return err
}

// WriteSimpleString writes a "+<string>\r\n" status line.
func WriteSimpleString(w io.Writer, s string) error {
	_, err := fmt.Fprintf(w, "+%s\r\n", s)
	return err
}

// WriteError writes a "-ERR <msg>\r\n" error line.
func WriteError(w io.Writer, msg string) error {
	_, err := fmt.Fprintf(w, "-ERR %s\r\n", msg)
	return err
}

// WriteInteger writes a ":<num>\r\n" integer line.
func WriteInteger(w io.Writer, num int64) error {
	_, err := fmt.Fprintf(w, ":%d\r\n", num)
	return err
}

// WriteBulkString writes a length-prefixed bulk string: "$<len>\r\n<data>\r\n".
func WriteBulkString(w io.Writer, data []byte) error {
	header := "$" + strconv.Itoa(len(data)) + "\r\n"
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := w.Write(crlf)
	return err
}

// WriteNull writes "$-1\r\n" indicating a missing or null value.
func WriteNull(w io.Writer) error {
	_, err := w.Write(nullBulk)
	return err
}

// WriteArrayHeader writes "*<count>\r\n" starting a multi-bulk array.
func WriteArrayHeader(w io.Writer, count int) error {
	_, err := fmt.Fprintf(w, "*%d\r\n", count)
	return err
}
