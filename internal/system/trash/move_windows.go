//go:build windows

package trash

import (
	"os"
	"path/filepath"
)

// Mover moves a path into the Recycle Bin, or into dir when that is set.
// Move never deletes a path it was asked to move. A path that cannot be
// trashed stays where it was.
type Mover struct {
	dir string
}

// Open is the Recycle Bin. When XDG_DATA_HOME is set, Open uses
// $XDG_DATA_HOME/Trash instead, so a test never writes the machine's
// Recycle Bin. The directory is created when a path is moved, not before.
func Open() (Mover, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return In(filepath.Join(dir, "Trash")), nil
	}
	return Mover{}, nil
}

// In moves paths into dir, the trash directory that holds files and info.
// dir is created on the first Move. A test passes a directory it created,
// so a Clean in a test never uses the machine's Recycle Bin.
func In(dir string) Mover { return Mover{dir: dir} }

// Move puts path in the trash. A Link above root is followed, because that
// is how the caller spelled the Root. A Link beneath root is not followed.
// A rename that fails, including one onto another volume, leaves path where
// it was. Move does not copy and it does not delete path.
func (m Mover) Move(root, path string) error {
	if m.dir == "" {
		return recycleMove(root, path)
	}
	return m.dirMove(root, path)
}

// Restore puts the most recent Move of path back. path must be gone from
// where it was. The file keeps its identity.
func (m Mover) Restore(root, path string) error {
	if m.dir == "" {
		return recycleRestore(root, path)
	}
	return m.dirRestore(root, path)
}
