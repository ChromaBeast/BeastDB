package crypt

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// A key source is named by a spec string, used everywhere a master key is
// configured (-encryption-key-file, beastctl --key-file):
//
//	/path/to/key        a key file (same as file:/path/to/key)
//	file:PATH           a key file; the prefix forces a path that looks like another spec
//	env:NAME            the environment variable NAME
//	exec:COMMAND ARGS   a command whose standard output is the key
//	exec:["cmd","arg"]  the same, argv as a JSON array (for arguments with spaces)
//
// In every case the key is 32 raw bytes or 64 hex characters (surrounding
// whitespace ignored). exec runs the program directly, never through a shell,
// with a timeout and a bounded read of stdout; its stderr is discarded so a
// careless script cannot leak the key into BeastDB's logs. This is the
// interface for a KMS: a small wrapper around the provider's CLI or SDK prints
// the unwrapped master key, and BeastDB links no cloud SDK.

const (
	// ExecTimeout bounds one run of an exec key source.
	ExecTimeout = 30 * time.Second
	// maxKeyOutput bounds what is read from a command or file.
	maxKeyOutput = 4096
)

var (
	// ErrKeyEnvUnset means an env: source names a variable that is empty or unset.
	ErrKeyEnvUnset = errors.New("crypt: key environment variable is not set")
	// ErrKeyCommand means an exec: source failed to run or exited non-zero.
	ErrKeyCommand = errors.New("crypt: key command failed")
)

// SourceKind names the kind of key source a spec selects: "file", "env" or
// "exec". It never includes the file name, variable name or command.
func SourceKind(spec string) string {
	switch {
	case strings.HasPrefix(spec, "env:"):
		return "env"
	case strings.HasPrefix(spec, "exec:"):
		return "exec"
	default:
		return "file"
	}
}

// Zero overwrites b. Go gives no guarantee about copies the runtime or the
// standard library made, but this clears the slices BeastDB owns.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// ReadMasterKey loads the master key named by spec (see the package comment on
// key sources). The caller should Zero the result when done with it.
func ReadMasterKey(spec string) ([]byte, error) {
	switch SourceKind(spec) {
	case "env":
		return readKeyEnv(strings.TrimPrefix(spec, "env:"))
	case "exec":
		return readKeyExec(strings.TrimPrefix(spec, "exec:"))
	}
	return readKeyFile(strings.TrimPrefix(spec, "file:"))
}

// parseKey accepts 32 raw bytes or 64 hex characters. It zeroes b.
func parseKey(b []byte) ([]byte, error) {
	defer Zero(b)
	if len(b) == KeySize {
		return append([]byte(nil), b...), nil
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 2*KeySize {
		k := make([]byte, KeySize)
		if _, err := hex.Decode(k, trimmed); err == nil {
			return k, nil
		}
		Zero(k)
	}
	return nil, ErrBadKeyFile
}

func readKeyEnv(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("crypt: env: key source needs a variable name")
	}
	v := os.Getenv(name)
	if v == "" {
		return nil, fmt.Errorf("%w: %s", ErrKeyEnvUnset, name)
	}
	// The process environment keeps its own copy of the string, which cannot
	// be cleared from Go; the runbook says to prefer a file or exec source.
	return parseKey([]byte(v))
}

func readKeyExec(cmdline string) ([]byte, error) {
	argv, err := parseCommand(cmdline)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	runErr := cmd.Run()
	defer Zero(out.b)
	if runErr != nil {
		// Neither stdout nor stderr is included: either could hold key material.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %s timed out after %s", ErrKeyCommand, argv[0], ExecTimeout)
		}
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			return nil, fmt.Errorf("%w: %s exited with status %d", ErrKeyCommand, argv[0], ee.ExitCode())
		}
		return nil, fmt.Errorf("%w: %s: %v", ErrKeyCommand, argv[0], errors.Unwrap(runErr))
	}
	if out.overflow {
		return nil, fmt.Errorf("%w: %s wrote more than %d bytes", ErrKeyCommand, argv[0], maxKeyOutput)
	}
	return parseKey(append([]byte(nil), out.b...))
}

// parseCommand splits an exec: command line into argv: a JSON array of
// strings, or whitespace-separated words (no quoting, no shell).
func parseCommand(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	var argv []string
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal([]byte(s), &argv); err != nil {
			return nil, fmt.Errorf("crypt: exec: key source: argv is not a JSON array of strings: %v", err)
		}
	} else {
		argv = strings.Fields(s)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("crypt: exec: key source needs a command")
	}
	return argv, nil
}

type limitedBuffer struct {
	b        []byte
	overflow bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := maxKeyOutput - len(l.b); len(p) > room {
		l.overflow = true
		p = p[:room]
	}
	l.b = append(l.b, p...)
	return len(p), nil // swallow the rest so the child never blocks on a full pipe
}

// SpecFromParts builds one key spec from the three ways a server can be
// configured (a file or full spec, the NAME of an environment variable, a
// command line). At most one may be set; none returns "" (no encryption).
func SpecFromParts(file, envName, command string) (string, error) {
	n := 0
	for _, s := range []string{file, envName, command} {
		if s != "" {
			n++
		}
	}
	switch {
	case n == 0:
		return "", nil
	case n > 1:
		return "", errors.New("crypt: set only one of the key file, key environment variable and key command options")
	case envName != "":
		return "env:" + envName, nil
	case command != "":
		return "exec:" + command, nil
	}
	return file, nil
}
