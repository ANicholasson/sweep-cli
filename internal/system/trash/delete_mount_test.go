//go:build linux

package trash

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDeleteDoesNotDescendIntoAnotherDevice(t *testing.T) {
	if os.Getenv("SWEEP_DELETE_MOUNT") != "1" {
		cmd := exec.Command("unshare", "--user", "--map-root-user", "--mount",
			os.Args[0], "-test.run=^TestDeleteDoesNotDescendIntoAnotherDevice$", "-test.count=1")
		cmd.Env = append(os.Environ(), "SWEEP_DELETE_MOUNT=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("mount namespace: %v\n%s", err, out)
		}
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
