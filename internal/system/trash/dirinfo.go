//go:build windows

package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// dirMove puts path in m.dir. The info file is written first. path is
// renamed onto the same file system. A Link above root is followed,
// because that is how the caller spelled the Root. A Link beneath root
// is not followed. A rename that fails, including one onto another file
// system, leaves path where it was. dirMove does not copy and it does not
// delete path.
func (m Mover) dirMove(root, path string) error {
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
	// A Link above the Root is followed. A Link beneath it is not. The
	// trash may sit behind a Link.
	if err := renameBetween(abs, root, false, dest, "", true); err != nil {
		_ = os.Remove(infoPath)
		return err
	}
	return nil
}

// dirRestore puts the most recent Move of path back. path must be gone
// from where it was. The file in the trash keeps its identity.
func (m Mover) dirRestore(root, path string) error {
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
	// The trash may sit behind a Link. The path is put back through the
	// Root's spelling, and a Link beneath the Root is not followed.
	if err := renameBetween(src, "", true, abs, root, false); err != nil {
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

// contains reports whether path is root or lies beneath it. The comparison
// is the cleaned text, which is the spelling the Plan recorded.
func contains(root, path string) bool {
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}
