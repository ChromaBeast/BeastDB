package main

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// runKeygen writes a new random master key file (mode 0600).
func runKeygen(out string) error {
	if out == "" {
		return errors.New("--out is required")
	}
	if err := crypt.GenerateKeyFile(out); err != nil {
		return err
	}
	fmt.Printf("Wrote master key to %s (mode 0600).\nStore a copy away from the data volume: without this key and %s the data cannot be read.\n",
		out, filepath.Base(crypt.KeyringPath("beast.bin")))
	return nil
}

// runMigrate implements encrypt (dst key only), decrypt (src key only) and
// rekey (both). The server must be stopped.
func runMigrate(verb, db, wal, srcKey, dstKey string, deleteOriginal bool) error {
	if db == "" || wal == "" {
		return errors.New("--db and --wal are required")
	}
	res, err := api.MigrateEncryption(db, wal, api.MigrateOptions{SrcKeyFile: srcKey, DstKeyFile: dstKey, DeleteOriginal: deleteOriginal})
	if err != nil {
		return err
	}
	fmt.Printf("%s complete: %d records copied to a new database.\n", verb, res.Records)
	fmt.Println("The write-ahead log restarted at LSN 0: re-seed any replicas.")
	if len(res.Kept) > 0 {
		fmt.Println("The originals were kept and are still readable by anyone with the disk:")
		for _, p := range res.Kept {
			fmt.Println("  ", p)
		}
		if verb != "decrypt" {
			fmt.Println("Move them to safe storage or delete them once the new database is checked (rerun with --delete-original to remove them at migration time; removal cannot guarantee the bytes are unrecoverable).")
		}
	}
	return nil
}

// bundleCipher builds a page cipher for an encrypted bundle from its keyring
// and the master key file. It returns nil, nil for a plaintext bundle.
func bundleCipher(dir string, manifestEncrypted bool, keyFile string) (*storage.PageCipher, error) {
	if !manifestEncrypted {
		if keyFile != "" {
			return nil, errors.New("bundle is not encrypted but --key-file was given")
		}
		return nil, nil
	}
	if keyFile == "" {
		return nil, nil
	}
	master, err := crypt.ReadMasterKey(keyFile)
	if err != nil {
		return nil, err
	}
	defer crypt.Zero(master)
	keys, err := crypt.OpenKeyringFile(filepath.Join(dir, "keyring.json"), master)
	if err != nil {
		return nil, err
	}
	return storage.NewPageCipher(keys.PageAEAD(), keys.ID), nil
}

// runRewrap changes the master key by re-wrapping the data key in the keyring.
// No data page or WAL byte is touched. The old key is proven against the data
// file first; the keyring is then replaced atomically (temp file, fsync,
// rename), so the old key works until the swap and the new key after it, and a
// crash leaves one or the other. Safe with the server stopped; a running server
// keeps its in-memory keys but must be restarted with the new key source.
func runRewrap(db, oldSpec, newSpec string) error {
	if db == "" {
		return errors.New("--db is required")
	}
	exists, enc, err := storage.FileEncrypted(db)
	if err != nil {
		return err
	}
	if !exists || !enc {
		return errors.New("database is not encrypted")
	}
	oldKey, err := crypt.ReadMasterKey(oldSpec)
	if err != nil {
		return err
	}
	defer crypt.Zero(oldKey)
	newKey, err := crypt.ReadMasterKey(newSpec)
	if err != nil {
		return err
	}
	defer crypt.Zero(newKey)
	if bytes.Equal(oldKey, newKey) {
		return errors.New("--new-key-file holds the same key as --key-file; nothing to rotate")
	}
	keys, err := crypt.OpenKeyring(db, oldKey)
	if err != nil {
		return err
	}
	if _, n, err := storage.AuthenticateFile(db, storage.NewPageCipher(keys.PageAEAD(), keys.ID)); err != nil {
		return fmt.Errorf("data file does not authenticate under the current key (keyring left unchanged): %w", err)
	} else if n == 0 {
		return errors.New("data file is empty")
	}
	if err := crypt.Rewrap(db, oldKey, newKey); err != nil {
		return err
	}
	fmt.Printf("Rekey complete: %s now opens with the new master key only. No data was rewritten; the data key (key ID %d) is unchanged.\n", filepath.Base(crypt.KeyringPath(db)), keys.ID)
	fmt.Println("Update the server's key source and restart it. Existing backups keep the master key they were made under (their keyring.json is a copy of the old one).")
	fmt.Println("Re-wrapping does not replace the data key: a copy of the old keyring plus the old master key still opens the data. Use rekey without --rewrap-only to generate a new data key.")
	return nil
}
