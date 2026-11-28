//go:build linux

package trash

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDeleteDoesNotDescendIntoAnotherDevice(t *testing.T) {
	if !inMountNamespace(t) {
		return
	}

	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	candidate := filepath.Join(root, "node_modules")
	mountPoint := filepath.Join(candidate, "mnt")
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join("/dev/shm", "sweep-delete-"+strconv.Itoa(os.Getpid()))
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outside) }()
	secret := filepath.Join(outside, "precious.txt")
	if err := os.WriteFile(secret, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(outside, mountPoint, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Unmount(mountPoint, unix.MNT_DETACH) }()

	err := (Mover{}).Delete(root, candidate)
	got, rerr := os.ReadFile(secret)
	if rerr != nil || string(got) != "precious\n" {
		t.Fatalf("the other device was removed: %q, %v", got, rerr)
	}
	if err == nil {
		t.Fatal("Delete returned after descending onto another device")
	}
}

func TestDeleteDoesNotDescendIntoASameFilesystemBindMount(t *testing.T) {
	if !inMountNamespace(t) {
		return
	}

	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	candidate := filepath.Join(root, "node_modules")
	mountPoint := filepath.Join(candidate, "mnt")
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "elsewhere")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "precious.txt")
	if err := os.WriteFile(secret, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var parent, source unix.Stat_t
	if err := unix.Stat(candidate, &parent); err != nil {
		t.Fatal(err)
	}
	if err := unix.Stat(outside, &source); err != nil {
		t.Fatal(err)
	}
	if parent.Dev != source.Dev {
		t.Fatalf("source device %d, parent device %d", source.Dev, parent.Dev)
	}
	if err := unix.Mount(outside, mountPoint, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Unmount(mountPoint, unix.MNT_DETACH) }()

	err := (Mover{}).Delete(root, candidate)
	got, rerr := os.ReadFile(secret)
	if rerr != nil || string(got) != "precious\n" {
		t.Fatalf("the bind mount was removed: %q, %v", got, rerr)
	}
	if !errors.Is(err, syscall.EXDEV) {
		t.Fatalf("Delete = %v, want a cross-device error", err)
	}
}

func TestDeleteDoesNotDescendIntoABindMountAboveTheCandidate(t *testing.T) {
	if !inMountNamespace(t) {
		return
	}

	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	mountPoint := filepath.Join(root, "app")
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "elsewhere")
	secret := filepath.Join(outside, "node_modules", "pkg", "precious.txt")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var parent, source unix.Stat_t
	if err := unix.Stat(root, &parent); err != nil {
		t.Fatal(err)
	}
	if err := unix.Stat(outside, &source); err != nil {
		t.Fatal(err)
	}
	if parent.Dev != source.Dev {
		t.Fatalf("source device %d, root device %d", source.Dev, parent.Dev)
	}
	if err := unix.Mount(outside, mountPoint, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Unmount(mountPoint, unix.MNT_DETACH) }()

	// The trash open still enters the mount. Delete does not.
	opened, oerr := openBeneath(mountPoint, root)
	if oerr != nil {
		t.Fatal(oerr)
	}
	_ = unix.Close(opened)

	candidate := filepath.Join(mountPoint, "node_modules")
	err := (Mover{}).Delete(root, candidate)
	got, rerr := os.ReadFile(secret)
	if rerr != nil || string(got) != "precious\n" {
		t.Fatalf("the path outside the Root was removed: %q, %v", got, rerr)
	}
	if !errors.Is(err, syscall.EXDEV) {
		t.Fatalf("Delete = %v, want a cross-device error", err)
	}
}

func TestMountIDOpenRefusesASameFilesystemBindMount(t *testing.T) {
	if !inMountNamespace(t) {
		return
	}

	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	candidate := filepath.Join(root, "node_modules")
	mountPoint := filepath.Join(candidate, "mnt")
	plain := filepath.Join(candidate, "plain")
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "elsewhere")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "precious.txt")
	if err := os.WriteFile(secret, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(outside, mountPoint, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Unmount(mountPoint, unix.MNT_DETACH) }()

	parent, err := unix.Open(candidate, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(parent) }()

	// This kernel has openat2, so Delete does not take this path. The path is
	// what runs when openat2 returns ENOSYS or EPERM.
	fd, err := openChildByMountID(parent, "mnt")
	if fd >= 0 {
		_ = unix.Close(fd)
	}
	if !errors.Is(err, syscall.EXDEV) {
		t.Fatalf("open = %v, want a cross-device error", err)
	}
	got, rerr := os.ReadFile(secret)
	if rerr != nil || string(got) != "precious\n" {
		t.Fatalf("the bind mount was removed: %q, %v", got, rerr)
	}
	plainfd, err := openChildByMountID(parent, "plain")
	if err != nil {
		t.Fatal(err)
	}
	_ = unix.Close(plainfd)
}

// inMountNamespace reports whether this process may bind-mount. The parent
// runs the test again where a mount is permitted and does not run the body.
// An unprivileged user namespace is tried first. A runner that refuses the
// uid map uses sudo, which can make a mount namespace. Either way the test
// fails when Delete enters the mount.
func inMountNamespace(t *testing.T) bool {
	t.Helper()
	if os.Getenv("SWEEP_DELETE_MOUNT") == "1" {
		return true
	}
	reexecInMountNamespace(t)
	return false
}

func reexecInMountNamespace(t *testing.T) {
	t.Helper()
	bin, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	testArgs := []string{bin, "-test.run=^" + t.Name() + "$", "-test.count=1"}
	cmd := exec.Command("unshare", append([]string{"--user", "--map-root-user", "--mount"}, testArgs...)...)
	cmd.Env = append(os.Environ(), "SWEEP_DELETE_MOUNT=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	if !bytes.Contains(out, []byte("uid_map")) {
		t.Fatalf("mount namespace: %v\n%s", err, out)
	}
	cmd = exec.Command("sudo", append([]string{"-n", "env", "SWEEP_DELETE_MOUNT=1", "unshare", "--mount"}, testArgs...)...)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mount namespace: %v\n%s", err, out)
	}
}
