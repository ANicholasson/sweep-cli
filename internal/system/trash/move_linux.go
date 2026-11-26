//go:build linux

package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Open is the trash in $XDG_DATA_HOME/Trash, or in
// $HOME/.local/share/Trash when XDG_DATA_HOME is unset. The directory is
// created when a path is moved, not before.
func Open() (Mover, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Mover{}, err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return In(filepath.Join(dir, "Trash")), nil
}

// In moves paths into dir, the trash directory that holds files and info.
// dir is created on the first Move. A test passes a directory it created,
// so a Clean in a test never uses the machine's trash.
func In(dir string) Mover { return Mover{dir: dir} }

// Move puts path in the trash. The info file is written first. path is
// renamed onto the same file system, one component at a time, without
// following a Link. A rename that fails, including one onto another file
// system, leaves path where it was. Move does not copy and it does not
// delete path.
func (m Mover) Move(path string) error {
	if m.dir == "" {
		return ErrUnavailable
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if err := os.MkdirAll(filepath.Join(m.dir, "files"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.dir, "info"), 0o700); err != nil {
		return err
	}
	name, infoPath, err := m.place(abs)
	if err != nil {
		return err
	}
	dest := filepath.Join(m.dir, "files", name)
	if err := renameNoFollow(abs, dest); err != nil {
		_ = os.Remove(infoPath)
		return err
	}
	return nil
}

// Restore puts the most recent Move of path back. path must be gone from
// where it was. The file in the trash keeps its identity.
func (m Mover) Restore(path string) error {
	if m.dir == "" {
		return ErrUnavailable
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	name, infoPath, err := m.find(abs)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err == nil {
		return fmt.Errorf("%s is still there, so it was not restored", abs)
	} else if !os.IsNotExist(err) {
		return err
	}
	src := filepath.Join(m.dir, "files", name)
	if err := renameNoFollow(src, abs); err != nil {
		return err
	}
	if err := os.Remove(infoPath); err != nil {
		return err
	}
	return nil
}

// place writes the info file for abs and returns the name in the trash.
// The info file exists before the path is renamed.
func (m Mover) place(abs string) (string, string, error) {
	base := filepath.Base(abs)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return "", "", fmt.Errorf("%s has no name", abs)
	}
	for n := 1; n < 100000; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s.%d", base, n)
		}
		infoPath := filepath.Join(m.dir, "info", name+".trashinfo")
		filePath := filepath.Join(m.dir, "files", name)
		if exists(infoPath) || exists(filePath) {
			continue
		}
		body := "[Trash Info]\nPath=" + escape(abs) + "\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		_, werr := f.WriteString(body)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(infoPath)
			if werr != nil {
				return "", "", werr
			}
			return "", "", cerr
		}
		return name, infoPath, nil
	}
	return "", "", errors.New("the trash has no free name")
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (m Mover) find(abs string) (name, infoPath string, err error) {
	entries, err := os.ReadDir(filepath.Join(m.dir, "info"))
	if err != nil {
		return "", "", err
	}
	var bestDate, bestName, bestPath string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".trashinfo") {
			continue
		}
		infoPath := filepath.Join(m.dir, "info", e.Name())
		data, err := os.ReadFile(infoPath)
		if err != nil {
			return "", "", err
		}
		got, when, ok := parseInfo(string(data))
		if !ok || got != abs {
			continue
		}
		if when >= bestDate {
			bestDate = when
			bestName = strings.TrimSuffix(e.Name(), ".trashinfo")
			bestPath = infoPath
		}
	}
	if bestName == "" {
		return "", "", fmt.Errorf("%s is not in the trash", abs)
	}
	return bestName, bestPath, nil
}

func parseInfo(text string) (path, when string, ok bool) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) == 0 || lines[0] != "[Trash Info]" {
		return "", "", false
	}
	for _, line := range lines[1:] {
		if rest, found := strings.CutPrefix(line, "Path="); found {
			decoded, err := unescape(rest)
			if err != nil {
				return "", "", false
			}
			path = decoded
		}
		if rest, found := strings.CutPrefix(line, "DeletionDate="); found {
			when = rest
		}
	}
	return path, when, path != ""
}

func escape(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', strings.ContainsRune("-_.~/", rune(c)):
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func unescape(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", errors.New("the trash info has a bad Path")
		}
		n, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			return "", errors.New("the trash info has a bad Path")
		}
		b.WriteByte(byte(n))
		i += 2
	}
	return b.String(), nil
}

// renameNoFollow renames oldpath to newpath. A Link in either path is an
// error, and neither path is followed. The last component is renamed as
// itself, so a Link there is moved and not what it leads to.
func renameNoFollow(oldpath, newpath string) error {
	oldfd, err := openDirNoFollow(filepath.Dir(oldpath))
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(oldfd) }()
	newfd, err := openDirNoFollow(filepath.Dir(newpath))
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(newfd) }()
	if err := syscall.Renameat(oldfd, filepath.Base(oldpath), newfd, filepath.Base(newpath)); err != nil {
		return &os.PathError{Op: "move", Path: oldpath, Err: err}
	}
	return nil
}

// openDirNoFollow opens path by opening each component with O_NOFOLLOW.
// A Link anywhere in path fails the open.
func openDirNoFollow(path string) (int, error) {
	path = filepath.Clean(path)
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if path == "/" {
		return fd, nil
	}
	if !strings.HasPrefix(path, "/") {
		_ = syscall.Close(fd)
		return -1, &os.PathError{Op: "open", Path: path, Err: errors.New("not absolute")}
	}
	for _, name := range strings.Split(path[1:], "/") {
		next, err := syscall.Openat(fd, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
		_ = syscall.Close(fd)
		if err != nil {
			return -1, &os.PathError{Op: "open", Path: path, Err: err}
		}
		fd = next
	}
	return fd, nil
}
