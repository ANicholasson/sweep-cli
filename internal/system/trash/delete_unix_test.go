//go:build linux || darwin

package trash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteDoesNotFollowALinkBeneathTheRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects")
	app := filepath.Join(root, "stale-app")
	candidate := filepath.Join(app, "node_modules")
	if err := os.MkdirAll(candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "note.txt"), []byte("note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside")
	secret := filepath.Join(outside, "node_modules", "precious.txt")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	held := app + ".held"
	if err := os.Rename(app, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, app); err != nil {
		t.Fatal(err)
	}

	err := Mover{}.Delete(root, candidate)
	if err == nil {
		t.Fatal("Delete followed a Link beneath the Root")
	}
	got, rerr := os.ReadFile(secret)
	if rerr != nil || string(got) != "precious\n" {
		t.Fatalf("the directory outside the Root is %q, %v", got, rerr)
	}
	note, rerr := os.ReadFile(filepath.Join(held, "node_modules", "note.txt"))
	if rerr != nil || string(note) != "note\n" {
		t.Fatalf("the Candidate set aside is %q, %v", note, rerr)
	}
}

func TestDeleteFollowsALinkAboveTheRoot(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real", "projects", "web-app")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(real, "node_modules")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "fx")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "fx", "projects")
	spelled := filepath.Join(root, "web-app", "node_modules")
	if err := (Mover{}).Delete(root, spelled); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("the Candidate is still there: %v", err)
	}
}

func TestDeleteUnlinksALinkInsideTheCandidate(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	candidate := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(outside, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(candidate, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "own.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Mover{}).Delete(root, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		t.Fatalf("the Candidate is still there: %v", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("the Link target is %q, %v", got, err)
	}
}
