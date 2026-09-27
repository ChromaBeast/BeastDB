package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

// ImportItem represents a record to be inserted into BeastDB.
type ImportItem struct {
	Key   uint64          `json:"key"`
	Value json.RawMessage `json:"value"`
}

func runImport(addr, filePath string, isSeed bool) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("could not read file %q: %w", filePath, err)
	}

	var items []ImportItem
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("invalid JSON array format in %q: %w", filePath, err)
	}

	action := "Importing"
	if isSeed {
		action = "Seeding"
	}
	fmt.Printf("🌱 %s %d records from %s into %s...\n", action, len(items), filePath, addr)

	t0 := time.Now()
	partitionCounts := make(map[uint8]int)
	ctx := context.Background()

	const batchSize = 100
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		ops := make([]*beastv1.BatchOperation, end-i)
		for j := i; j < end; j++ {
			ops[j-i] = &beastv1.BatchOperation{
				OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT,
				Key:    items[j].Key,
				Value:  []byte(items[j].Value),
			}
			p := uint8(items[j].Key >> 56)
			partitionCounts[p]++
		}

		resp, err := client.Service().BatchWrite(ctx, &beastv1.BatchWriteRequest{Operations: ops})
		if err != nil {
			return fmt.Errorf("failed to batch write at offset %d: %w", i, err)
		}
		if !resp.Success {
			return fmt.Errorf("server rejected batch at offset %d", i)
		}
	}

	duration := time.Since(t0)
	opsPerSec := float64(len(items)) / duration.Seconds()
	if duration.Seconds() == 0 {
		opsPerSec = float64(len(items))
	}

	fmt.Printf("✅ Successfully stored %d records in %v (%.0f writes/sec)\n",
		len(items), duration.Round(time.Millisecond), opsPerSec)

	fmt.Println("\n📊 Partition Distribution:")
	for p, count := range partitionCounts {
		fmt.Printf("  • Partition 0x%02X (%3d): %5d records\n", p, p, count)
	}
	fmt.Println()

	return nil
}
