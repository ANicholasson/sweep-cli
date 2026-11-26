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
