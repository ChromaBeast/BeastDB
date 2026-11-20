package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Destination receives a finished, verified bundle so a copy exists off the
// database host. Put must be safe to call again for the same id.
type Destination interface {
	// Kind names the implementation ("dir" or "exec"); it is shown in Studio.
	Kind() string
	// Where is a non-secret description of the target.
	Where() string
	// Put copies the bundle in bundleDir under the given backup id.
	Put(ctx context.Context, id, bundleDir string) error
}

// bundleFiles are the files of a bundle; keyring.json exists only when encrypted.
var bundleFiles = []string{"data.bin", KeyringName, "manifest.json"}

// DirDestination copies bundles into a local or mounted directory (an NFS
// share, a second disk). Each bundle is staged as <id>.partial, checked
// against its manifest and renamed into place, so a reader never sees a
// half-copied bundle under its final name.
type DirDestination struct{ Path string }

func (d DirDestination) Kind() string  { return "dir" }
func (d DirDestination) Where() string { return d.Path }

func (d DirDestination) Put(ctx context.Context, id, bundleDir string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	if err := os.MkdirAll(d.Path, 0o750); err != nil {
		return err
	}
	final := filepath.Join(d.Path, id)
	if _, err := os.Stat(filepath.Join(final, "manifest.json")); err == nil {
		return nil // already copied
	}
	stage := final + ".partial"
	_ = os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0o750); err != nil {
		return err
	}
	fail := func(err error) error { _ = os.RemoveAll(stage); return err }
	for _, name := range bundleFiles { // manifest.json last: it marks a complete bundle
		if err := copyBundleFile(ctx, filepath.Join(bundleDir, name), filepath.Join(stage, name)); err != nil {
			if errors.Is(err, os.ErrNotExist) && name == KeyringName {
				continue
			}
			return fail(err)
		}
	}
	// Verify the copy itself (checksum and size; page authentication needs the key).
	if _, err := Verify(stage); err != nil {
		return fail(fmt.Errorf("copy failed verification: %w", err))
	}
	if err := os.RemoveAll(final); err != nil {
		return fail(err)
	}
	if err := os.Rename(stage, final); err != nil {
		return fail(err)
	}
	return nil
}

func copyBundleFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, ctxReader{ctx, in}); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ExecDestination runs an operator-supplied program to ship the bundle, so
// BeastDB itself never holds cloud credentials: the program (a wrapper around
// aws, rclone, scp, ...) owns them. It is run directly, not through a shell,
// as `program <bundle-dir> <backup-id>`, with a cleaned environment (PATH and
// HOME plus BEASTDB_BACKUP_DIR and BEASTDB_BACKUP_ID) so the server's own
// settings do not leak into it. A non-zero exit means the copy failed.
type ExecDestination struct {
	Program string
	Timeout time.Duration // 0 means 30 minutes
}

func (e ExecDestination) Kind() string { return "exec" }

// Where is the program path only; arguments and environment are never shown.
func (e ExecDestination) Where() string { return e.Program }

// maxHookOutput bounds how much of the program's output is kept for errors.
const maxHookOutput = 2048

func (e ExecDestination) Put(ctx context.Context, id, bundleDir string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.Program, bundleDir, id)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
		"BEASTDB_BACKUP_DIR=" + bundleDir, "BEASTDB_BACKUP_ID=" + id,
	}
	var out limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if msg != "" {
			return fmt.Errorf("copy hook failed: %v: %s", err, msg)
		}
		return fmt.Errorf("copy hook failed: %v", err)
	}
	return nil
}

// limitedBuffer keeps the first maxHookOutput bytes and discards the rest.
type limitedBuffer struct{ b bytes.Buffer }

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := maxHookOutput - l.b.Len(); room > 0 {
		l.b.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string { return l.b.String() }
