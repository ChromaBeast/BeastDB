package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDirDestinationCopiesAndVerifies(t *testing.T) {
	e := newEngine(t, 100)
	m := NewManager(e, filepath.Join(t.TempDir(), "b"))
	off := filepath.Join(t.TempDir(), "offsite")
	m.SetDestination(DirDestination{Path: off})

	info, err := m.Create(KindManual, "root")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filepath.Join(off, info.ID)); err != nil {
		t.Fatalf("the off-host copy must verify: %v", err)
	}
	if _, err := os.Stat(filepath.Join(off, info.ID+".partial")); err == nil {
		t.Fatal("staging directory left behind")
	}
	list, _ := m.List()
	if len(list) != 1 || list[0].Offsite == nil || !list[0].Offsite.OK {
		t.Fatalf("offsite status: %+v", list)
	}
	// Putting the same bundle again is harmless.
	if r, err := m.CopyOffsite(info.ID); err != nil || !r.OK {
		t.Fatalf("repeat copy: %v %+v", err, r)
	}
}

func TestDirDestinationRejectsCorruptSourceAndKeepsNoPartial(t *testing.T) {
	e := newEngine(t, 50)
	root := filepath.Join(t.TempDir(), "b")
	info, err := Create(e, root, KindManual, "x")
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, info.ID)
	f, _ := os.OpenFile(filepath.Join(src, "data.bin"), os.O_WRONLY, 0)
	_, _ = f.WriteAt([]byte{0xFF, 0xEE}, 5000)
	f.Close()
	off := filepath.Join(t.TempDir(), "off")
	if err := (DirDestination{Path: off}).Put(t.Context(), info.ID, src); err == nil {
		t.Fatal("a corrupt bundle must not be accepted off-host")
	}
	if ents, _ := os.ReadDir(off); len(ents) != 0 {
		t.Fatalf("nothing may remain after a failed copy: %v", ents)
	}
}

func writeHook(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell hook test")
	}
	p := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExecDestinationRunsHookWithCleanEnvironment(t *testing.T) {
	t.Setenv("BEASTDB_ADMIN_PASSWORD", "hunter2-secret")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret")
	e := newEngine(t, 60)
	out := filepath.Join(t.TempDir(), "hook.out")
	hook := writeHook(t, `{ echo "args=$1 $2"; echo "id=$BEASTDB_BACKUP_ID"; env; } > `+out+"\ncp \"$1/data.bin\" \"$1/../hook-copy.bin\"\n")
	m := NewManager(e, filepath.Join(t.TempDir(), "b"))
	m.SetDestination(ExecDestination{Program: hook})
	info, err := m.Create(KindManual, "root")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("hook did not run: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "id="+info.ID) || !strings.Contains(got, filepath.Join(m.Dir(), info.ID)+" "+info.ID) {
		t.Fatalf("hook got wrong arguments:\n%s", got)
	}
	if strings.Contains(got, "hunter2-secret") || strings.Contains(got, "aws-secret") {
		t.Fatal("the server's environment leaked into the hook")
	}
	st, _ := m.List()
	if st[0].Offsite == nil || !st[0].Offsite.OK {
		t.Fatalf("%+v", st[0].Offsite)
	}
}

func TestExecDestinationFailureIsRecordedAndOutputCapped(t *testing.T) {
	e := newEngine(t, 20)
	hook := writeHook(t, "head -c 100000 /dev/zero | tr '\\0' 'A'\nexit 3\n")
	m := NewManager(e, filepath.Join(t.TempDir(), "b"))
	m.SetDestination(ExecDestination{Program: hook})
	var seen Result
	m.SetCopyObserver(func(_ string, r Result) { seen = r })
	info, err := m.Create(KindManual, "root")
	if err != nil {
		t.Fatalf("a failed off-host copy must not fail the backup: %v", err)
	}
	st, _ := m.List()
	if st[0].ID != info.ID || st[0].Offsite == nil || st[0].Offsite.OK {
		t.Fatalf("failure not recorded: %+v", st[0].Offsite)
	}
	if len(st[0].Offsite.Error) > maxHookOutput+200 || seen.OK || seen.Error == "" {
		t.Fatalf("error text unbounded or observer not told: len=%d seen=%+v", len(st[0].Offsite.Error), seen)
	}
}

func TestOffsitePausedAndNoDestination(t *testing.T) {
	e := newEngine(t, 20)
	m := NewManager(e, filepath.Join(t.TempDir(), "b"))
	if _, err := m.CopyOffsite("bk-20260101-000000"); err != ErrNoDestination {
		t.Fatalf("got %v", err)
	}
	if m.DestinationInfo() != nil {
		t.Fatal("no destination configured")
	}
	off := filepath.Join(t.TempDir(), "off")
	m.SetDestination(DirDestination{Path: off})
	m.SetOffsite(false)
	if _, err := m.Create(KindManual, "root"); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(off); len(ents) != 0 {
		t.Fatal("copy made while paused")
	}
	if d := m.DestinationInfo(); d == nil || d.Enabled || d.Kind != "dir" {
		t.Fatalf("%+v", d)
	}
}
