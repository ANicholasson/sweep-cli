package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	// Walk the target one part at a time. Cleaning ".." before the next
	// Link is followed would land on a sibling of that Link.
	return walk(filepath.Dir(path), target, hops+1)
}

func walk(dir, target string, hops int) (string, error) {
	if hops > 255 {
		return "", fmt.Errorf("%s has too many Links", target)
	}
	cur := dir
	rest := target
	if filepath.IsAbs(target) {
		cur, rest = volumeRoot(target), target[len(filepath.VolumeName(target)):]
	}
	for _, part := range components(rest) {
		switch part {
		case "", ".":
			continue
		case "..":
			cur = parent(cur)
			continue
		}
		next := cur
		if next != "" && !strings.HasSuffix(next, "/") && !strings.HasSuffix(next, `\`) {
			next += string(filepath.Separator)
		}
		next += part
		info, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		followed, ok, err := linkTarget(info, next)
		if err != nil {
			return "", err
		}
		if !ok {
			cur = next
			continue
		}
		cur, err = walk(cur, followed, hops+1)
		if err != nil {
			return "", err
		}
	}
	return cur, nil
}

func components(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || (runtime.GOOS == "windows" && r == '\\')
	})
}

func volumeRoot(path string) string {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return string(filepath.Separator)
	}
	if strings.HasPrefix(vol, `\\`) || strings.HasPrefix(vol, `//`) {
		return vol
	}
	return vol + `\`
}

func parent(cur string) string {
	root := volumeRoot(cur)
	if len(cur) <= len(root) {
		return root
	}
	next := filepath.Dir(cur)
	if len(next) < len(root) {
		return root
	}
	return next
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
