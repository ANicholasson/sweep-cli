//go:build unix

package files

import (
	"fmt"
	"os"
	"syscall"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Space is lstat: st_blocks is in 512-byte units whatever the file system's
// block size, and the device with the inode is what every hard link to one
// file shares. du reads the same fields.
func (Disk) Space(path string) (scan.Space, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return scan.Space{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return scan.Space{}, fmt.Errorf("%s: the file system gave no block count", path)
	}
	return scan.Space{
		Bytes: int64(st.Blocks) * 512,
		File:  scan.FileID{Volume: uint64(st.Dev), File: uint64(st.Ino)},
	}, nil
}
