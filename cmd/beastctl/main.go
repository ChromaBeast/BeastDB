package main

import (
	"flag"
	"fmt"
	"os"
)

const Version = "0.1.0"

func printUsage() {
	fmt.Println("BeastDB CLI (beastctl) - High-Performance Database Tooling")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  beastctl <command> [arguments]")
	fmt.Println()
	fmt.Println("Available Commands:")
	fmt.Println("  ping               Check database reachability and measure roundtrip latency")
	fmt.Println("  export             Export database or specific partition records to a JSON file")
	fmt.Println("  import             Bulk load records from an exported JSON file")
	fmt.Println("  seed               Seed database with fixture data and display partition stats")
	fmt.Println("  backup             Create an atomic physical database backup bundle with checksum")
	fmt.Println("  verify             Validate manifest and checksums of a backup bundle")
	fmt.Println("  restore            Restore database bundle to target directory with safe overwrite checks")
	fmt.Println("  keygen             Write a new random master key file for encryption at rest")
	fmt.Println("  encrypt            Convert a stopped plaintext database to encrypted (--key-file)")
	fmt.Println("  decrypt            Convert a stopped encrypted database back to plaintext (--key-file)")
	fmt.Println("  rekey              Re-encrypt a stopped database under a new master key (--key-file, --new-key-file)")
	fmt.Println("  rejoin             Rejoin a fenced node as a follower of the new leader (refused if it holds writes the leader lacks)")
	fmt.Println("  get                Retrieve a record value by its 64-bit key")
	fmt.Println("  put                Store or update a key-value record")
	fmt.Println("  delete             Tombstone/delete a record by its 64-bit key")
	fmt.Println("  version            Show beastctl version")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  beastctl ping --addr 127.0.0.1:50051")
	fmt.Println("  beastctl export --out backup.json --partition 0x01")
	fmt.Println("  beastctl seed --file ./fixtures/seed.json")
	fmt.Println("  beastctl put --key 72057594037927936 --value '{\"username\":\"charlie\"}'")
	fmt.Println("  beastctl get --key 72057594037927936")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "ping":
		fs := flag.NewFlagSet("ping", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		_ = fs.Parse(args)
		handleErr(runPing(*addr))

	case "export":
		fs := flag.NewFlagSet("export", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		out := fs.String("out", "export.json", "Output file path (or '-' for stdout)")
		partition := fs.String("partition", "", "Optional partition filter (e.g. 0x01 or 1)")
		_ = fs.Parse(args)
		handleErr(runExport(*addr, *out, *partition, "json"))

	case "import":
		fs := flag.NewFlagSet("import", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		file := fs.String("file", "", "JSON file to import")
		_ = fs.Parse(args)
		if *file == "" {
			fmt.Println("Error: --file argument is required.")
			os.Exit(1)
		}
		handleErr(runImport(*addr, *file, false))

	case "seed":
		fs := flag.NewFlagSet("seed", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		file := fs.String("file", "", "Fixture JSON file to seed")
		_ = fs.Parse(args)
		if *file == "" {
			fmt.Println("Error: --file argument is required.")
			os.Exit(1)
		}
		handleErr(runImport(*addr, *file, true))

	case "get":
		fs := flag.NewFlagSet("get", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		key := fs.Uint64("key", 0, "64-bit record key")
		_ = fs.Parse(args)
		handleErr(runGet(*addr, *key))

	case "put":
		fs := flag.NewFlagSet("put", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		key := fs.Uint64("key", 0, "64-bit record key")
		val := fs.String("value", "", "Record value (JSON or string)")
		_ = fs.Parse(args)
		if *val == "" {
			fmt.Println("Error: --value argument is required.")
			os.Exit(1)
		}
		handleErr(runPut(*addr, *key, *val))

	case "delete":
		fs := flag.NewFlagSet("delete", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:50051", "BeastDB gRPC address")
		key := fs.Uint64("key", 0, "64-bit record key")
		_ = fs.Parse(args)
		handleErr(runDelete(*addr, *key))

	case "backup":
		fs := flag.NewFlagSet("backup", flag.ExitOnError)
		db := fs.String("db", "beast.bin", "Path to source database file")
		wal := fs.String("wal", "beast.wal", "Path to source WAL file")
		out := fs.String("out", "./backup", "Output directory for backup bundle")
		keyFile := fs.String("key-file", "", "Master key file (required if the database is encrypted)")
		_ = fs.Parse(args)
		handleErr(runBackupKey(*db, *wal, *out, *keyFile))

	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		backupDir := fs.String("backup", "./backup", "Path to backup directory")
		keyFile := fs.String("key-file", "", "Master key file: authenticates every page of an encrypted bundle")
		_ = fs.Parse(args)
		handleErr(runVerifyKey(*backupDir, *keyFile))

	case "restore":
		fs := flag.NewFlagSet("restore", flag.ExitOnError)
		backupDir := fs.String("backup", "./backup", "Path to backup directory")
		dest := fs.String("dest", "./restored", "Destination directory")
		force := fs.Bool("force", false, "Overwrite existing files in destination directory")
		keyFile := fs.String("key-file", "", "Master key file (required for an encrypted bundle)")
		_ = fs.Parse(args)
		_, err := runRestoreKey(*backupDir, *dest, *force, *keyFile)
		handleErr(err)

	case "rejoin":
		fs := flag.NewFlagSet("rejoin", flag.ExitOnError)
		web := fs.String("web", "http://127.0.0.1:8080", "Web (Studio) address of the fenced node")
		user := fs.String("user", "admin", "Admin username")
		pwFile := fs.String("password-file", "", "File holding the admin password (else BEASTDB_ADMIN_PASSWORD)")
		leader := fs.String("leader", "", "gRPC address (host:port) of the current leader")
		_ = fs.Parse(args)
		handleErr(runRejoin(*web, *user, *pwFile, *leader))

	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ExitOnError)
		out := fs.String("out", "", "Path of the key file to create (mode 0600; refuses to overwrite)")
		_ = fs.Parse(args)
		handleErr(runKeygen(*out))

	case "encrypt", "decrypt", "rekey":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		db := fs.String("db", "beast.bin", "Path to the database file (server must be stopped)")
		wal := fs.String("wal", "beast.wal", "Path to the WAL file")
		keyFile := fs.String("key-file", "", "Master key file (encrypt: the new key; decrypt/rekey: the current key)")
		newKey := fs.String("new-key-file", "", "rekey: the new master key file")
		del := fs.Bool("delete-original", false, "Remove the replaced files instead of keeping *.pre*.bak copies")
		_ = fs.Parse(args)
		if *keyFile == "" {
			fmt.Println("Error: --key-file is required.")
			os.Exit(1)
		}
		switch cmd {
		case "encrypt":
			handleErr(runMigrate("Encrypt", *db, *wal, "", *keyFile, *del))
		case "decrypt":
			handleErr(runMigrate("Decrypt", *db, *wal, *keyFile, "", *del))
		default:
			if *newKey == "" {
				fmt.Println("Error: --new-key-file is required for rekey.")
				os.Exit(1)
			}
			handleErr(runMigrate("Rekey", *db, *wal, *keyFile, *newKey, *del))
		}

	case "version":
		fmt.Printf("beastctl v%s\n", Version)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func handleErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
