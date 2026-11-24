package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Resolve returns path as an absolute path, with a Link at its end followed
// to a path that is not a Link. A chain is followed here and not again. An
// ancestor that is a Link stays in the spelling, so a Root that is not
// itself a Link is unchanged. filepath.EvalSymlinks leaves a Windows
// junction alone, so this follows one itself.
func Resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolve(abs, 0)
}

func resolve(path string, hops int) (string, error) {
	if hops > 255 {
		return "", fmt.Errorf("%s has too many Links", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	target, ok, err := linkTarget(info, path)
	if err != nil {
		return "", err
	}
	if !ok {
		return filepath.Clean(path), nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	next, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	return resolve(next, hops+1)
}

// linkTarget reports the path a symlink or a Windows junction stores.
// ModeIrregular is also a socket or a placeholder, and Readlink fails on
// those, so they are not Links.
func linkTarget(info os.FileInfo, path string) (string, bool, error) {
	mode := info.Mode()
	if mode&os.ModeSymlink == 0 && mode&fs.ModeIrregular == 0 {
		return "", false, nil
	}
	target, err := os.Readlink(path)
	if err != nil {
		if mode&os.ModeSymlink != 0 {
			return "", false, err
		}
		return "", false, nil
	}
	return target, true, nil
}
