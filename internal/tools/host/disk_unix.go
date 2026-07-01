//go:build !windows

package host

import (
	"fmt"
	"syscall"
)

// diskUsage returns disk usage statistics for the given path.
// Uses syscall.Statfs on Linux and Darwin.
func diskUsage(path string) (map[string]any, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	avail := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	inodesTotal := stat.Files
	inodesFree := stat.Ffree
	inodesUsed := inodesTotal - inodesFree

	return map[string]any{
		"path":          path,
		"total_bytes":   total,
		"used_bytes":    used,
		"free_bytes":    free,
		"avail_bytes":   avail,
		"usage_percent": fmt.Sprintf("%.1f", float64(used)/float64(total)*100),
		"inodes": map[string]any{
			"total":         inodesTotal,
			"used":          inodesUsed,
			"free":          inodesFree,
			"usage_percent": fmt.Sprintf("%.1f", float64(inodesUsed)/float64(inodesTotal)*100),
		},
	}, nil
}
