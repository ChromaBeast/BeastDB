package main

import (
	"context"
	"fmt"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

func runPing(addr string) error {
	t0 := time.Now()
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Query key 0 to test roundtrip
	_, err = client.Service().Get(ctx, &beastv1.GetRequest{Key: 0})
	latency := time.Since(t0).Round(time.Microsecond)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	fmt.Printf("⚡ BeastDB at %s is healthy (latency: %v)\n", addr, latency)
	return nil
}

func runGet(addr string, key uint64) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.Service().Get(context.Background(), &beastv1.GetRequest{Key: key})
	if err != nil {
		return fmt.Errorf("get error: %w", err)
	}

	if !resp.Found {
		fmt.Printf("❌ Key %d not found.\n", key)
		return nil
	}

	fmt.Printf("🔑 Key:       %d (0x%016X)\n", resp.Key, resp.Key)
	fmt.Printf("📦 Partition: 0x%02X (%d)\n", uint8(resp.Key>>56), uint8(resp.Key>>56))
	fmt.Printf("📄 Value:\n%s\n", string(resp.Value))
	return nil
}

func runPut(addr string, key uint64, value string) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.Service().Put(context.Background(), &beastv1.PutRequest{
		Key:   key,
		Value: []byte(value),
	})
	if err != nil {
		return fmt.Errorf("put error: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("server rejected write for key %d", key)
	}

	fmt.Printf("✅ Stored key %d in partition 0x%02X.\n", key, uint8(key>>56))
	return nil
}

func runDelete(addr string, key uint64) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.Service().Delete(context.Background(), &beastv1.DeleteRequest{Key: key})
	if err != nil {
		return fmt.Errorf("delete error: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete failed for key %d", key)
	}

	fmt.Printf("🗑️ Deleted key %d.\n", key)
	return nil
}
