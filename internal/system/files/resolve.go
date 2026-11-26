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
// to a path that is not a Link. A chain is followed here and not again. A
// relative target, including "..", is applied after every Link on the way
// to that Link has been followed. A path that is not itself a Link keeps
// the caller's spelling. filepath.EvalSymlinks leaves a Windows junction
// alone, so this follows a junction itself.
func Resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolve(abs, 0)
}

// ResolveDir returns path as an absolute path with every Link on the way
// followed, including one that is not the last component. path must exist.
func ResolveDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolveDir(abs, 0)
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
	// ".." in the target is the parent of the directory the Link is in,
	// after Links on the way there have been followed. The spelling of
	// that directory still contains those Links, so it is the wrong base.
	base, err := resolveDir(filepath.Dir(path), hops)
	if err != nil {
		return "", err
	}
	return walk(base, target, hops+1)
}

// resolveDir follows every Link in path. path is the directory that holds
// the Link about to be followed.
func resolveDir(path string, hops int) (string, error) {
	vol := volumeRoot(path)
	rest := path
	if v := filepath.VolumeName(path); v != "" {
		rest = path[len(v):]
	}
	return descend(vol, components(rest), hops)
}

func walk(dir, target string, hops int) (string, error) {
	if hops > 255 {
		return "", fmt.Errorf("%s has too many Links", target)
	}
	cur := dir
	rest := target
	if filepath.IsAbs(target) {
		cur = volumeRoot(target)
		if v := filepath.VolumeName(target); v != "" {
			rest = target[len(v):]
		}
	}
	return descend(cur, components(rest), hops)
}

func descend(dir string, parts []string, hops int) (string, error) {
	cur := dir
	for _, part := range parts {
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
		hops++
		cur, err = walk(cur, followed, hops)
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

// volumeRoot is the directory at the top of path's volume. A \\?\ volume
// names the device until a separator follows it, so the filesystem root
// keeps that separator.
func volumeRoot(path string) string {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return string(filepath.Separator)
	}
	if strings.HasSuffix(vol, `/`) || strings.HasSuffix(vol, `\`) {
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
