package wal

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestEpochMarksPlainAndEncryptedAcrossRotation(t *testing.T) {
	for name, c := range map[string]*Cipher{"plain": nil, "encrypted": testWALCipher(t)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "w.wal")
			w, err := OpenWALCipher(path, true, c)
			if err != nil {
				t.Fatal(err)
			}
			mustAppend := func(op byte, k, v []byte) uint64 {
				lsn, err := w.Write(op, k, v)
				if err != nil {
					t.Fatal(err)
				}
				return lsn
			}
			mustAppend(OpPut, make([]byte, 8), []byte("legacy")) // before any marker
			m1 := mustAppend(OpEpoch, nil, EncodeEpoch(1))
			mustAppend(OpPut, make([]byte, 8), []byte("x"))
			if err := w.Checkpoint(); err != nil { // marker moves to the .bak generation
				t.Fatal(err)
			}
			m2 := mustAppend(OpEpoch, nil, EncodeEpoch(2))
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}

			got, err := EpochMarks(c, path+".bak", path)
			if err != nil {
				t.Fatal(err)
			}
			want := []EpochMark{{LSN: m1, Epoch: 1}, {LSN: m2, Epoch: 2}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("marks = %+v, want %+v", got, want)
			}
		})
	}
}

// A log written before markers existed has none and reads as before.
func TestEpochMarksAbsentInLegacyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.wal")
	writeLog(t, path, nil, 5)
	got, err := EpochMarks(nil, path+".bak", path)
	if err != nil || len(got) != 0 {
		t.Fatalf("marks = %+v, err = %v; want none", got, err)
	}
}

func TestMalformedEpochMarkerIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.wal")
	w, _ := OpenWAL(path, true)
	if _, err := w.Write(OpEpoch, nil, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	if _, err := EpochMarks(nil, path); err != ErrBadEpochMarker {
		t.Fatalf("err = %v, want ErrBadEpochMarker", err)
	}
}
