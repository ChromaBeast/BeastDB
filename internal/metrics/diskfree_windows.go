//go:build windows

package metrics

import "golang.org/x/sys/windows"

// DiskFreeBytes reports the space available to the caller on path's volume.
func DiskFreeBytes(path string) (uint64, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return 0, false
	}
	return avail, true
}
