//go:build windows

package host

import (
	"fmt"
	"syscall"
	"unsafe"
)

// diskUsage returns disk usage statistics for the given path.
// Uses GetDiskFreeSpaceEx via syscall on Windows.
func diskUsage(path string) (map[string]any, error) {
	if len(path) == 0 {
		path = "C:\\"
	}
	// Ensure path ends with backslash for Windows API
	if path[len(path)-1] != '\\' && path[len(path)-1] != '/' {
		path = path + "\\"
	}
	// Convert forward slashes to backslashes
	pathUTF16 := make([]uint16, 0, len(path)+1)
	for _, r := range path {
		if r == '/' {
			pathUTF16 = append(pathUTF16, '\\')
		} else {
			pathUTF16 = append(pathUTF16, uint16(r))
		}
	}
	pathUTF16 = append(pathUTF16, 0)

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytes, totalBytes, totalFreeBytes uint64
	r1, _, err := proc.Call(
		uintptr(unsafe.Pointer(&pathUTF16[0])),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("GetDiskFreeSpaceExW failed: %w", err)
	}

	used := totalBytes - totalFreeBytes

	return map[string]any{
		"path":          path,
		"total_bytes":   totalBytes,
		"used_bytes":    used,
		"free_bytes":    totalFreeBytes,
		"avail_bytes":   freeBytes,
		"usage_percent": fmt.Sprintf("%.1f", float64(used)/float64(totalBytes)*100),
		"inodes": map[string]any{
			"total":         0,
			"used":          0,
			"free":          0,
			"usage_percent": "0.0",
			"note":          "inode tracking not available on Windows",
		},
	}, nil
}
