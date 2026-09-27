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

	for i, item := range items {
		valBytes := []byte(item.Value)
		// If the JSON raw message is a quoted string, unquote it if needed or store as is
		resp, err := client.Service().Put(ctx, &beastv1.PutRequest{
			Key:   item.Key,
			Value: valBytes,
		})
		if err != nil {
			return fmt.Errorf("failed to put key %d (index %d): %w", item.Key, i, err)
		}
		if !resp.Success {
			return fmt.Errorf("server rejected key %d (index %d)", item.Key, i)
		}

		p := uint8(item.Key >> 56)
		partitionCounts[p]++
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
