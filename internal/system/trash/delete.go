package trash

import (
	"os"
	"path/filepath"
)

// Delete removes a Link at path, or inside it, as a Link. The target stays.
func (Mover) Delete(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}
	return os.RemoveAll(abs)
}
