package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
)

// TestBackupRestoreCleanHostDrill executes an end-to-end backup, checksum verification,
// and clean-host restore drill, measuring RTO and verifying RPO = 0.
func TestBackupRestoreCleanHostDrill(t *testing.T) {
	tempDir := t.TempDir()

	srcDB := filepath.Join(tempDir, "source.bin")
	srcWAL := filepath.Join(tempDir, "source.wal")
	backupDir := filepath.Join(tempDir, "backup_bundle")
	restoreDir := filepath.Join(tempDir, "clean_host_dest")

	// 1. Initialize and populate source database
	engine, err := api.NewEngine(srcDB, srcWAL, 20)
	if err != nil {
		t.Fatalf("failed to create source engine: %v", err)
	}

	const recordCount = 100
	for i := uint64(1); i <= recordCount; i++ {
		val := []byte(fmt.Sprintf("user-payload-id-%d", i))
		if err := engine.Put(i, val); err != nil {
			t.Fatalf("failed putting record %d: %v", i, err)
		}
	}
	_ = engine.Close()

	// 2. Execute physical backup
	if err := runBackup(srcDB, srcWAL, backupDir); err != nil {
		t.Fatalf("runBackup failed: %v", err)
	}

	// 3. Verify backup bundle integrity
	if err := runVerify(backupDir); err != nil {
		t.Fatalf("runVerify failed: %v", err)
	}

	// 4. Test safety: verify tamper detection
	tamperPath := filepath.Join(backupDir, "data.bin")
	originalBytes, err := os.ReadFile(tamperPath)
	if err != nil {
		t.Fatalf("failed to read data.bin: %v", err)
	}

	// Flip a byte and verify runVerify detects checksum divergence
	corruptBytes := make([]byte, len(originalBytes))
	copy(corruptBytes, originalBytes)
	corruptBytes[len(corruptBytes)-1] ^= 0xFF
	_ = os.WriteFile(tamperPath, corruptBytes, 0644)
	if err := runVerify(backupDir); err == nil {
		t.Fatalf("expected runVerify to detect corrupted data.bin, but passed")
	}

	// Restore pristine bytes
	_ = os.WriteFile(tamperPath, originalBytes, 0644)
	if err := runVerify(backupDir); err != nil {
		t.Fatalf("pristine verify failed: %v", err)
	}

	// 5. Execute clean-host restore and measure RTO
	startRestore := time.Now()
	res, err := runRestore(backupDir, restoreDir, false)
	rto := time.Since(startRestore)
	if err != nil {
		t.Fatalf("runRestore failed: %v", err)
	}

	t.Logf("Restore Drill Metrics: RTO = %v, Restored Records = %d", rto, res.RecordCount)

	if res.RecordCount != recordCount {
		t.Fatalf("RPO violation: expected %d records, got %d", recordCount, res.RecordCount)
	}

	// 6. Test safety: prevent silent overwrite without --force
	if _, err := runRestore(backupDir, restoreDir, false); err == nil {
		t.Fatalf("expected restore to reject overwriting existing database without force, but it succeeded")
	}

	// 7. Verify all restored records are byte-for-byte identical
	restoredEngine, err := api.NewEngine(res.DestDB, res.DestWAL, 20)
	if err != nil {
		t.Fatalf("failed to open restored database: %v", err)
	}
	defer restoredEngine.Close()

	for i := uint64(1); i <= recordCount; i++ {
		val, found, err := restoredEngine.Get(i)
		if err != nil || !found {
			t.Fatalf("restored db missing record %d: found=%v, err=%v", i, found, err)
		}
		expected := []byte(fmt.Sprintf("user-payload-id-%d", i))
		if !bytes.Equal(val, expected) {
			t.Fatalf("data mismatch on key %d: expected %s, got %s", i, expected, val)
		}
	}
}
