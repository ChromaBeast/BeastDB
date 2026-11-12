//go:build unix

package metrics

import "syscall"

// DiskFreeBytes returns bytes available to unprivileged users on the
// filesystem holding path.
func DiskFreeBytes(path string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}
