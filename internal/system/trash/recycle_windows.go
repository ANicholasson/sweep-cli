//go:build windows

package trash

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// A Recycle Bin entry is a pair in <volume>\$Recycle.Bin\<SID>.
// $I holds the original path. $R is the renamed Candidate. The $I file is
// written first. The Candidate is renamed onto the same volume, never
// copied, so a failure leaves it where it was.

const (
	recycleVersion = 2
	shcneCreate    = 0x00000002
	shcneDelete    = 0x00000004
	shcneUpdateDir = 0x00001000
	shcnfPathW     = 0x0005
	fileAttrHidden = 2
)

var (
	getVolumePathName = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumePathNameW")
	setFileAttributes = syscall.NewLazyDLL("kernel32.dll").NewProc("SetFileAttributesW")
	shChangeNotify    = syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify")
)

func recycleMove(root, path string) error {
	abs, err := absPath(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	bin, err := recycleBin(abs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return err
	}
	suffix, iPath, err := claimInfo(bin, info.Name(), abs, info)
	if err != nil {
		return err
	}
	dest := filepath.Join(bin, "$R"+suffix)
	if err := renameBetween(abs, root, false, dest, "", true); err != nil {
		_ = os.Remove(iPath)
		return err
	}
	hide(iPath)
	hide(dest)
	notify(shcneDelete, abs)
	notify(shcneUpdateDir, bin)
	return nil
}

func recycleRestore(root, path string) error {
	abs, err := absPath(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err == nil {
		return fmt.Errorf("%s is still there, so it was not restored", abs)
	} else if !os.IsNotExist(err) {
		return err
	}
	bin, err := recycleBin(abs)
	if err != nil {
		return err
	}
	suffix, err := findRecycled(bin, abs)
	if err != nil {
		return err
	}
	src := filepath.Join(bin, "$R"+suffix)
	if err := renameBetween(src, "", true, abs, root, false); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(bin, "$I"+suffix)); err != nil {
		return err
	}
	notify(shcneCreate, abs)
	notify(shcneUpdateDir, bin)
	return nil
}

func absPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func recycleBin(path string) (string, error) {
	vol, err := volumeOf(path)
	if err != nil {
		return "", err
	}
	sid, err := currentSID()
	if err != nil {
		return "", err
	}
	return filepath.Join(vol, "$Recycle.Bin", sid), nil
}

func volumeOf(path string) (string, error) {
	vol, err := volumePath(path)
	if err != nil {
		vol, err = volumePath(filepath.Dir(path))
	}
	if err != nil {
		return "", err
	}
	vol = stripExtended(vol)
	if vol == "" {
		return "", fmt.Errorf("%s has no volume", path)
	}
	if !strings.HasSuffix(vol, `\`) {
		vol += `\`
	}
	return vol, nil
}

func volumePath(path string) (string, error) {
	p, err := syscall.UTF16PtrFromString(long(path))
	if err != nil {
		return "", err
	}
	buf := make([]uint16, 32768)
	r, _, callErr := getVolumePathName.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buf), nil
}

func stripExtended(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`):
		return p[len(`\\?\`):]
	default:
		return p
	}
}

func currentSID() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer func() { _ = token.Close() }()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	sid, err := user.User.Sid.String()
	runtime.KeepAlive(user)
	return sid, err
}

// claimInfo writes the $I file and returns the shared suffix of the pair.
// The Candidate is still in place when this returns.
func claimInfo(bin, base, abs string, info os.FileInfo) (string, string, error) {
	ext := filepath.Ext(base)
	if len(ext) > 16 {
		ext = ""
	}
	for range 100 {
		id, err := randomID()
		if err != nil {
			return "", "", err
		}
		suffix := id + ext
		iPath := filepath.Join(bin, "$I"+suffix)
		rPath := filepath.Join(bin, "$R"+suffix)
		if exists(iPath) || exists(rPath) {
			continue
		}
		if err := writeInfo(iPath, abs, fileSize(info)); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", "", err
		}
		return suffix, iPath, nil
	}
	return "", "", errors.New("the trash has no free name")
}

func fileSize(info os.FileInfo) uint64 {
	if info.IsDir() || info.Size() < 0 {
		return 0
	}
	return uint64(info.Size())
}

func randomID() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = alphabet[int(c)%len(alphabet)]
	}
	return string(out), nil
}

// writeInfo writes a version-2 $I file. The path length counts UTF-16
// code units and includes the trailing NUL, which is the Windows 10 layout.
func writeInfo(path, original string, size uint64) error {
	encoded := utf16.Encode([]rune(original + "\x00"))
	buf := make([]byte, 28+len(encoded)*2)
	binary.LittleEndian.PutUint64(buf[0:], recycleVersion)
	binary.LittleEndian.PutUint64(buf[8:], size)
	binary.LittleEndian.PutUint64(buf[16:], nowFiletime())
	binary.LittleEndian.PutUint32(buf[24:], uint32(len(encoded)))
	for i, u := range encoded {
		binary.LittleEndian.PutUint16(buf[28+i*2:], u)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(buf)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(path)
		if werr != nil {
			return werr
		}
		return cerr
	}
	return nil
}

func nowFiletime() uint64 {
	// 100-nanosecond intervals since 1601-01-01 UTC. A Duration cannot
	// span from that epoch to now, so the count is taken from the Unix epoch.
	const windowsToUnix = 116444736000000000
	return uint64(time.Now().UnixNano()/100) + windowsToUnix
}

func findRecycled(bin, abs string) (string, error) {
	entries, err := os.ReadDir(bin)
	if err != nil {
		return "", err
	}
	var bestWhen uint64
	var best string
	var found bool
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "$I") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(bin, e.Name()))
		if err != nil {
			return "", err
		}
		got, when, ok := parseRecycleInfo(data)
		if !ok || got != abs {
			continue
		}
		if !found || when >= bestWhen {
			found = true
			bestWhen = when
			best = strings.TrimPrefix(e.Name(), "$I")
		}
	}
	if !found {
		return "", fmt.Errorf("%s is not in the trash", abs)
	}
	return best, nil
}

func parseRecycleInfo(data []byte) (path string, when uint64, ok bool) {
	if len(data) < 28 {
		return "", 0, false
	}
	if binary.LittleEndian.Uint64(data[0:8]) != recycleVersion {
		return "", 0, false
	}
	when = binary.LittleEndian.Uint64(data[16:24])
	n := binary.LittleEndian.Uint32(data[24:28])
	if n == 0 || int(n) > (len(data)-28)/2 {
		return "", 0, false
	}
	chars := make([]uint16, n)
	for i := range chars {
		chars[i] = binary.LittleEndian.Uint16(data[28+i*2:])
	}
	path = strings.TrimRight(string(utf16.Decode(chars)), "\x00")
	return path, when, path != ""
}

func hide(path string) {
	p, err := syscall.UTF16PtrFromString(long(path))
	if err != nil {
		return
	}
	_, _, _ = setFileAttributes.Call(uintptr(unsafe.Pointer(p)), fileAttrHidden)
}

func notify(event uintptr, path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	_, _, _ = shChangeNotify.Call(event, shcnfPathW, uintptr(unsafe.Pointer(p)), 0)
}
