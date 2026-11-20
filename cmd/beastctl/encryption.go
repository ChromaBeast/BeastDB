package main

import (
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
	keys, err := crypt.OpenKeyringFile(filepath.Join(dir, "keyring.json"), master)
	if err != nil {
		return nil, err
	}
	return storage.NewPageCipher(keys.PageAEAD(), keys.ID), nil
}
