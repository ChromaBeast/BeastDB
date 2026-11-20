//go:build !unix && !windows

package metrics

// DiskFreeBytes is unsupported on this platform.
func DiskFreeBytes(string) (uint64, bool) { return 0, false }
