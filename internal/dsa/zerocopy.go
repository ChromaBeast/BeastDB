package dsa

import (
	"unsafe"
)

// StringToBytes converts a string to a byte slice without heap allocation.
//
// SAFETY: The returned byte slice shares the underlying memory of the string.
// Callers must NEVER modify the contents of the returned byte slice, as strings
// are immutable in Go and modifying this slice causes undefined behavior.
func StringToBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// BytesToString converts a byte slice to a string without heap allocation.
//
// SAFETY: The returned string shares the underlying memory of the byte slice.
// Callers must guarantee that the byte slice is not modified while the string
// is in use.
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// CloneBytes allocates and returns an independent copy of the byte slice.
// Use this when zero-copy safety cannot be guaranteed across goroutines.
func CloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	clone := make([]byte, len(b))
	copy(clone, b)
	return clone
}
