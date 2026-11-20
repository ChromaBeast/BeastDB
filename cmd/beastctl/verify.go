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
	var (
		meta  storage.MetaData
		pages string
	)
	if pc != nil {
		var n uint64
		if meta, n, err = storage.AuthenticateFile(dataPath, pc); err != nil {
			return fmt.Errorf("page authentication failed: %w", err)
		}
		pages = fmt.Sprintf("%d (all pages authenticated)", n)
	} else {
		page0 := make([]byte, storage.PageSize)
		if _, err := f.ReadAt(page0, 0); err != nil {
			return fmt.Errorf("cannot read Page 0: %w", err)
		}
		if meta, pages, err = verifyDataFile(f, page0, size); err != nil {
			return err
		}
	}

	if meta.RootPageID != manifest.RootPageID {
		return fmt.Errorf("root page ID mismatch: manifest=%d, page0=%d", manifest.RootPageID, meta.RootPageID)
	}

	fmt.Printf("Verification PASSED:\n  Directory:  %s\n  Checksum:   %s (valid)\n  Format:     version %d\n  Pages:      %s\n  RootPageID: %d\n  Checkpoint: LSN %d\n", backupDir, actualChecksum, meta.Version, pages, meta.RootPageID, meta.LastCheckpointLSN)
	return nil
}

// verifyDataFile validates page 0 and, for a format version 2 file, every
// page's checksum. A version 1 file has no page checksums; it is accepted
// (the server migrates it on first open) and reported as unchecked.
func verifyDataFile(f *os.File, page0 []byte, size int64) (storage.MetaData, string, error) {
	version, ok := storage.MetaVersionOf(page0)
	if !ok {
		return storage.MetaData{}, "", fmt.Errorf("Page 0 metadata validation failed: %w", storage.ErrInvalidMetaMagic)
	}
	if version == 1 {
		meta, err := storage.DecodeMetaV1(page0)
		if err != nil {
			return meta, "", fmt.Errorf("Page 0 metadata validation failed: %w", err)
		}
		return meta, "not checked (format version 1 has no page checksums)", nil
	}
	meta, err := storage.DecodeMeta(page0)
	if err != nil {
		return meta, "", fmt.Errorf("Page 0 metadata validation failed: %w", err)
	}
	page := make([]byte, storage.PageSize)
	n := uint64(size / storage.PageSize)
	for id := uint64(0); id < n; id++ {
		if _, err := f.ReadAt(page, int64(id)*storage.PageSize); err != nil {
			return meta, "", fmt.Errorf("read page %d: %w", id, err)
		}
		if err := storage.VerifyPageImage(id, page); err != nil {
			return meta, "", fmt.Errorf("page checksum validation failed: %w", err)
		}
	}
	return meta, fmt.Sprintf("%d (all checksums valid)", n), nil
}
