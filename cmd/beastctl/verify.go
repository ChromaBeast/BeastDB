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

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// runVerify checks a bundle without a key (checksum and, for plaintext, page 0).
func runVerify(backupDir string) error { return runVerifyKey(backupDir, "") }

func runVerifyKey(backupDir, keyFile string) error {
	if backupDir == "" {
		return errors.New("backup directory is required (--backup)")
	}

	manifestPath := filepath.Join(backupDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("missing or unreadable manifest.json: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("invalid manifest JSON: %w", err)
	}

	dataPath := filepath.Join(backupDir, "data.bin")
	f, err := os.Open(dataPath)
	if err != nil {
		return fmt.Errorf("missing or unreadable data.bin: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return fmt.Errorf("failed reading data.bin for checksum: %w", err)
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	if actualChecksum != manifest.DataChecksumSHA256 {
		return fmt.Errorf("checksum mismatch: expected %s, computed %s", manifest.DataChecksumSHA256, actualChecksum)
	}

	if size != manifest.FileSize {
		return fmt.Errorf("file size mismatch: manifest specifies %d bytes, found %d bytes", manifest.FileSize, size)
	}

	pc, err := bundleCipher(backupDir, manifest.Encrypted, keyFile)
	if err != nil {
		return err
	}
	if manifest.Encrypted && pc == nil {
		fmt.Printf("Verification PASSED (checksum and size only):\n  Directory: %s\n  Checksum:  %s (valid)\n  The bundle is encrypted; pass --key-file to authenticate every page.\n", backupDir, actualChecksum)
		return nil
	}
	var meta storage.MetaData
	if pc != nil {
		if meta, _, err = storage.AuthenticateFile(dataPath, pc); err != nil {
			return fmt.Errorf("page authentication failed: %w", err)
		}
	} else {
		page0 := make([]byte, storage.PageSize)
		if _, err := f.ReadAt(page0, 0); err != nil {
			return fmt.Errorf("cannot read Page 0: %w", err)
		}
		if meta, err = storage.DecodeMeta(page0); err != nil {
			return fmt.Errorf("Page 0 metadata validation failed: %w", err)
		}
	}

	if meta.RootPageID != manifest.RootPageID {
		return fmt.Errorf("root page ID mismatch: manifest=%d, page0=%d", manifest.RootPageID, meta.RootPageID)
	}

	fmt.Printf("Verification PASSED:\n  Directory:  %s\n  Checksum:   %s (valid)\n  RootPageID: %d\n  Checkpoint: LSN %d\n", backupDir, actualChecksum, meta.RootPageID, meta.LastCheckpointLSN)
	return nil
}
