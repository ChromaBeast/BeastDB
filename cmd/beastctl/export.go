package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

// DumpRecord represents an exported BeastDB entry.
type DumpRecord struct {
	Key       uint64          `json:"key"`
	KeyHex    string          `json:"key_hex"`
	Partition uint8           `json:"partition"`
	Value     json.RawMessage `json:"value"`
}

func runExport(addr, outPath, partitionStr, format string) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	startKey := uint64(0)
	endKey := uint64(math.MaxUint64)

	if partitionStr != "" {
		p, err := parsePartition(partitionStr)
		if err != nil {
			return err
		}
		startKey = uint64(p) << 56
		endKey = (uint64(p) << 56) | 0x00ffffffffffffff
		fmt.Printf("📦 Scoping export to partition 0x%02X (%d)...\n", p, p)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.Service().Scan(ctx, &beastv1.ScanRequest{
		StartKey: startKey,
		EndKey:   endKey,
	})
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	var out io.Writer
	var closeOut func() error
	if outPath == "" || outPath == "-" {
		out = os.Stdout
		closeOut = func() error { return nil }
	} else {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		out = f
		closeOut = f.Close
	}
	defer closeOut()

	if _, err := io.WriteString(out, "[\n"); err != nil {
		return err
	}

	count := 0
	t0 := time.Now()

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading stream: %w", err)
		}

		if count > 0 {
			if _, err := io.WriteString(out, ",\n"); err != nil {
				return err
			}
		}

		val := resp.Value
		var raw json.RawMessage
		if json.Valid(val) {
			raw = json.RawMessage(val)
		} else {
			quoted, _ := json.Marshal(string(val))
			raw = json.RawMessage(quoted)
		}

		rec := DumpRecord{
			Key:       resp.Key,
			KeyHex:    fmt.Sprintf("0x%016X", resp.Key),
			Partition: uint8(resp.Key >> 56),
			Value:     raw,
		}

		itemData, err := json.MarshalIndent(rec, "  ", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}
		if _, err := out.Write(append([]byte("  "), itemData...)); err != nil {
			return err
		}
		count++
	}

	if _, err := io.WriteString(out, "\n]\n"); err != nil {
		return err
	}

	if outPath != "" && outPath != "-" {
		fmt.Printf("✅ Streamed %d records to %s (in %v)\n", count, outPath, time.Since(t0).Round(time.Millisecond))
	}

	return nil
}

func parsePartition(s string) (uint8, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		v, err := strconv.ParseUint(s[2:], 16, 8)
		if err != nil {
			return 0, fmt.Errorf("invalid hex partition %q: %w", s, err)
		}
		return uint8(v), nil
	}
	v, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid partition %q: %w", s, err)
	}
	return uint8(v), nil
}
