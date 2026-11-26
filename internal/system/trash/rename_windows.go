//go:build windows

package trash

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	fileListDirectory   = 0x0001
	fileAddFile         = 0x0002
	fileAddSubdirectory = 0x0004
	fileTraverse        = 0x0020
	fileReadAttributes  = 0x0080
	deleteAccess        = 0x00010000
	synchronize         = 0x00100000
	standardRightsRead  = 0x00020000

	fileOpen                = 1
	fileDirectoryFile       = 0x00000001
	fileSynchronousNonalert = 0x00000020
	fileOpenForBackup       = 0x00004000
	fileOpenReparsePoint    = 0x00200000
	objCaseInsensitive      = 0x00000040
)

var (
	ntdll                 = syscall.NewLazyDLL("ntdll.dll")
	ntCreateFile          = ntdll.NewProc("NtCreateFile")
	rtlNtStatusToDosError = ntdll.NewProc("RtlNtStatusToDosError")
	moveFileEx            = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")
)

type ntUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type objectAttributes struct {
	Length             uint32
	RootDirectory      syscall.Handle
	ObjectName         *ntUnicodeString
	Attributes         uint32
	SecurityDescriptor uintptr
	SecurityQoS        uintptr
}

type ioStatusBlock struct {
	Status      uintptr
	Information uintptr
}

// renameBetween renames oldpath to newpath. followOld and followNew say
// whether every Link in that path, above the last component, may be
// followed. When follow is false, root is the Root: a Link above it is
// followed and a Link beneath it is refused. The last component is
// renamed as itself. The rename is refused when the two paths are on
// different volumes, and nothing is copied or deleted.
func renameBetween(oldpath, oldRoot string, followOld bool, newpath, newRoot string, followNew bool) error {
	oldParent, err := openParent(oldpath, oldRoot, followOld, parentAccess)
	if err != nil {
		return err
	}
	src, err := openChild(oldParent, filepath.Base(oldpath))
	if err != nil {
		_ = syscall.CloseHandle(oldParent)
		return err
	}
	_ = syscall.CloseHandle(src)
	_ = syscall.CloseHandle(oldParent)
	newParent, err := openParent(newpath, newRoot, followNew, destAccess)
	if err != nil {
		return err
	}
	_ = syscall.CloseHandle(newParent)
	// The opens above refused a Link. MoveFileEx does not copy, so a
	// different volume fails and the path stays where it was.
	return moveSameVolume(oldpath, newpath)
}

const (
	parentAccess = fileListDirectory | fileTraverse | fileReadAttributes | synchronize | standardRightsRead
	destAccess   = parentAccess | fileAddFile | fileAddSubdirectory
	childAccess  = deleteAccess | fileReadAttributes | synchronize | standardRightsRead
	childOptions = fileOpenReparsePoint | fileOpenForBackup | fileSynchronousNonalert
	dirOptions   = fileDirectoryFile | childOptions
)

func openParent(path, root string, follow bool, access uint32) (syscall.Handle, error) {
	dir := filepath.Clean(filepath.Dir(path))
	if follow {
		return openFollow(dir, access)
	}
	return openBeneath(dir, root, access)
}

func openChild(parent syscall.Handle, name string) (syscall.Handle, error) {
	return openRelative(parent, name, childAccess, childOptions)
}

// openFollow opens path and follows a Link on the way. It is used for the
// trash directory only.
func openFollow(path string, access uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(long(path))
	if err != nil {
		return 0, err
	}
	h, err := syscall.CreateFile(p, access, shareAll, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, err
	}
	dir, err := handleIsDir(h)
	if err != nil || !dir {
		_ = syscall.CloseHandle(h)
		if err != nil {
			return 0, err
		}
		return 0, &os.PathError{Op: "open", Path: path, Err: errors.New("not a directory")}
	}
	return h, nil
}

// openBeneath opens dir. A Link above root is followed, which is how a
// Root that keeps the caller's spelling is reached. A Link at root or
// beneath it is refused.
func openBeneath(dir, root string, access uint32) (syscall.Handle, error) {
	dir = filepath.Clean(dir)
	root = filepath.Clean(root)
	if root == "" || !contains(root, dir) {
		return openNoFollow(dir, access)
	}
	h, err := openFinalNoFollow(root, access)
	if err != nil {
		return 0, err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		_ = syscall.CloseHandle(h)
		return openNoFollow(dir, access)
	}
	if rel == "." {
		return h, nil
	}
	for _, name := range strings.Split(rel, string(filepath.Separator)) {
		next, err := openOne(h, dir, name, access)
		if err != nil {
			return 0, err
		}
		h = next
	}
	return h, nil
}

// openNoFollow opens path by opening each component and refusing a Link.
func openNoFollow(path string, access uint32) (syscall.Handle, error) {
	path = filepath.Clean(path)
	vol := filepath.VolumeName(path)
	if vol == "" {
		return 0, &os.PathError{Op: "open", Path: path, Err: errors.New("not absolute")}
	}
	root := vol
	if !strings.HasSuffix(root, `\`) {
		root += `\`
	}
	h, err := openFinalNoFollow(root, access)
	if err != nil {
		return 0, err
	}
	if path == root || path == strings.TrimRight(root, `\`) {
		return h, nil
	}
	if !strings.HasPrefix(path, root) {
		_ = syscall.CloseHandle(h)
		return 0, &os.PathError{Op: "open", Path: path, Err: errors.New("not absolute")}
	}
	for _, name := range strings.Split(path[len(root):], `\`) {
		next, err := openOne(h, path, name, access)
		if err != nil {
			return 0, err
		}
		h = next
	}
	return h, nil
}

func openOne(parent syscall.Handle, full, name string, access uint32) (syscall.Handle, error) {
	if name == "" || name == "." {
		return parent, nil
	}
	next, err := openRelative(parent, name, access, dirOptions)
	_ = syscall.CloseHandle(parent)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: full, Err: err}
	}
	rep, err := handleIsReparse(next)
	if err != nil || rep {
		_ = syscall.CloseHandle(next)
		if err != nil {
			return 0, err
		}
		return 0, &os.PathError{Op: "open", Path: full, Err: errors.New("it is a Link")}
	}
	return next, nil
}

// openFinalNoFollow opens path and follows a Link above the last component.
// A Link at the end fails the open.
func openFinalNoFollow(path string, access uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(long(path))
	if err != nil {
		return 0, err
	}
	flags := uint32(syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT)
	h, err := syscall.CreateFile(p, access, shareAll, nil, syscall.OPEN_EXISTING, flags, 0)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	rep, err := handleIsReparse(h)
	if err != nil || rep {
		_ = syscall.CloseHandle(h)
		if err != nil {
			return 0, err
		}
		return 0, &os.PathError{Op: "open", Path: path, Err: errors.New("it is a Link")}
	}
	dir, err := handleIsDir(h)
	if err != nil || !dir {
		_ = syscall.CloseHandle(h)
		if err != nil {
			return 0, err
		}
		return 0, &os.PathError{Op: "open", Path: path, Err: errors.New("not a directory")}
	}
	return h, nil
}

const shareAll = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE

func openRelative(parent syscall.Handle, name string, access, options uint32) (syscall.Handle, error) {
	chars, err := syscall.UTF16FromString(name)
	if err != nil {
		return 0, err
	}
	if len(chars) < 2 || len(chars)*2 > 0xffff {
		return 0, &os.PathError{Op: "open", Path: name, Err: errors.New("a component has no usable name")}
	}
	us := ntUnicodeString{
		Length:        uint16((len(chars) - 1) * 2),
		MaximumLength: uint16(len(chars) * 2),
		Buffer:        &chars[0],
	}
	oa := objectAttributes{
		RootDirectory: parent,
		ObjectName:    &us,
		Attributes:    objCaseInsensitive,
	}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var iosb ioStatusBlock
	var h syscall.Handle
	status, _, _ := ntCreateFile.Call(
		uintptr(unsafe.Pointer(&h)),
		uintptr(access),
		uintptr(unsafe.Pointer(&oa)),
		uintptr(unsafe.Pointer(&iosb)),
		0,
		uintptr(syscall.FILE_ATTRIBUTE_NORMAL),
		uintptr(shareAll),
		fileOpen,
		uintptr(options),
		0,
		0,
	)
	runtime.KeepAlive(chars)
	runtime.KeepAlive(&us)
	runtime.KeepAlive(&oa)
	runtime.KeepAlive(&iosb)
	if status != 0 {
		return 0, ntErrno(status)
	}
	return h, nil
}

func ntErrno(status uintptr) error {
	r, _, _ := rtlNtStatusToDosError.Call(status)
	if r == 0 {
		return syscall.Errno(status)
	}
	return syscall.Errno(r)
}

// moveSameVolume renames oldpath to newpath. No copy flag is set, so a
// different volume is an error and oldpath is left in place.
func moveSameVolume(oldpath, newpath string) error {
	from, err := syscall.UTF16PtrFromString(long(oldpath))
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(long(newpath))
	if err != nil {
		return err
	}
	r, _, callErr := moveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), 0)
	if r == 0 {
		return &os.PathError{Op: "move", Path: oldpath, Err: callErr}
	}
	return nil
}

func handleIsReparse(h syscall.Handle) (bool, error) {
	info, err := handleInfo(h)
	if err != nil {
		return false, err
	}
	return info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func handleIsDir(h syscall.Handle) (bool, error) {
	info, err := handleInfo(h)
	if err != nil {
		return false, err
	}
	return info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0, nil
}

func handleInfo(h syscall.Handle) (syscall.ByHandleFileInformation, error) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return info, err
	}
	return info, nil
}

// long gives a path the prefix that lifts the 260-character limit.
// syscall.CreateFile does not add it, and the os package does.
func long(path string) string {
	switch {
	case len(path) < 248 || strings.HasPrefix(path, `\\?\`):
		return path
	case strings.HasPrefix(path, `\\`):
		return `\\?\UNC\` + path[2:]
	default:
		return `\\?\` + path
	}
}
