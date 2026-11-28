//go:build linux

package trash

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDeleteDoesNotDescendIntoAnotherDevice(t *testing.T) {
	if os.Getenv("SWEEP_DELETE_MOUNT") != "1" {
		runInMountNamespace(t)
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

// runInMountNamespace runs this test where a bind mount is permitted. An
// unprivileged user namespace is tried first. A runner that refuses the
// uid map runs the same test through sudo, which can make a mount namespace.
// Either way the test fails when Delete enters the other device.
func runInMountNamespace(t *testing.T) {
	t.Helper()
	bin, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	testArgs := []string{bin, "-test.run=^TestDeleteDoesNotDescendIntoAnotherDevice$", "-test.count=1"}
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
