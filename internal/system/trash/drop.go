// Package trash removes a file from disk. It is the only package that does.
package trash

import "os"

// Drop removes path. A path that is already gone is not an error.
func Drop(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
