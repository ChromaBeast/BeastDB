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
