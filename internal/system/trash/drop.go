// Package trash is the only package that removes anything from disk.
// Move puts a Candidate in the operating system's trash. Drop removes a
// file sweep itself created, such as a temporary Plan.
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

// Place renames oldpath to newpath. A Plan is written to a temporary file
// and then placed at its path. A test moves a directory the same way.
// This package is the only one that renames a path.
func Place(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
