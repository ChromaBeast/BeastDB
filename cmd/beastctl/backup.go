package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// BackupManifest describes the metadata and checksum of a physical backup bundle.
type BackupManifest struct {
	Version            string `json:"version"`
	CreatedAt          string `json:"created_at"`
	DataChecksumSHA256 string `json:"data_checksum_sha256"`
	RootPageID         uint64 `json:"root_page_id"`
	ActiveDataPageID   uint64 `json:"active_data_page_id"`
	LastCheckpointLSN  uint64 `json:"last_checkpoint_lsn"`
	FileSize           int64  `json:"file_size"`
	Encrypted          bool   `json:"encrypted,omitempty"`
	KeyID              uint16 `json:"key_id,omitempty"`
}

// runBackup backs up a plaintext database.
func runBackup(dbPath, walPath, outDir string) error {
	return runBackupKey(dbPath, walPath, outDir, "")
}

func runBackupKey(dbPath, walPath, outDir, keyFile string) error {
	if dbPath == "" {
		return errors.New("db path is required (--db)")
	}
	if outDir == "" {
		return errors.New("output backup directory is required (--out)")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	engine, err := api.NewEngineWithOptions(dbPath, walPath, 20, api.Options{KeyFile: keyFile})
	if err != nil {
		return fmt.Errorf("failed to open database for backup: %w", err)
	}

	snapshotPath, err := engine.CreateSnapshot(outDir)
	krPath, pc := engine.KeyringPath(), engine.VerifyCipher()
	_ = engine.Close()
	if err != nil {
		return fmt.Errorf("create snapshot failed: %w", err)
	}

	targetData := filepath.Join(outDir, "data.bin")
	_ = os.Remove(targetData)
	if err := os.Rename(snapshotPath, targetData); err != nil {
		return fmt.Errorf("finalize backup data file failed: %w", err)
	}

	f, err := os.Open(targetData)
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return err
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	var meta storage.MetaData
	if pc != nil {
		var aerr error
		if meta, _, aerr = storage.AuthenticateFile(targetData, pc); aerr != nil {
			return fmt.Errorf("authenticate backup pages: %w", aerr)
		}
		kr, rerr := os.ReadFile(krPath)
		if rerr != nil {
			return fmt.Errorf("copy keyring: %w", rerr)
		}
		if werr := os.WriteFile(filepath.Join(outDir, "keyring.json"), kr, 0o600); werr != nil {
			return werr
		}
	} else {
		page0 := make([]byte, storage.PageSize)
		if _, err := f.ReadAt(page0, 0); err == nil {
			meta, _ = storage.DecodeMeta(page0)
		}
	}

	manifest := BackupManifest{
		Version:            "1.0.0",
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
		DataChecksumSHA256: checksum,
		RootPageID:         meta.RootPageID,
		ActiveDataPageID:   meta.ActiveDataPageID,
		LastCheckpointLSN:  meta.LastCheckpointLSN,
		FileSize:           size,
	}
	if pc != nil {
		manifest.Encrypted, manifest.KeyID = true, pc.KeyID()
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(outDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("failed to write backup manifest: %w", err)
	}

	fmt.Printf("Backup completed successfully:\n  Directory: %s\n  Checksum:  %s\n  Size:      %d bytes\n", outDir, checksum, size)
	return nil
}
