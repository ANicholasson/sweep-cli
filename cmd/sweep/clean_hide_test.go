//go:build linux

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanSkipsACandidateThatHidesATrackedFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a directory by its mode")
	}
	b := build(t)
	dir, key := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	web := at("projects/web-app/node_modules")
	planPath := filepath.Join(t.TempDir(), "plan.json")
	dry := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, "--pick", web, at(key.Root))
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q", dry.status, dry.stderr)
	}
	// The repository is added after the Plan, then hidden. A Scan would have
	// kept the Candidate had it been able to see the tracked file. A Clean
	// cannot see it either, and that is why the Git rule applies.
	sealed := filepath.Join(web, "sealed")
	if err := os.MkdirAll(sealed, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(sealed, "keep.js")
	if err := os.WriteFile(kept, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sealed, "init", "-q", "-b", "main")
	gitIn(t, sealed, "add", "--", "keep.js")
	gitIn(t, sealed, "commit", "-q", "-m", "Track")
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })

	got := b.run(t, dir, "clean", "--plan", planPath)
	if err := os.Chmod(sealed, 0o755); err != nil {
		t.Fatal(err)
	}
	if got.status != 1 {
		t.Fatalf("exit %d, want 1, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if !strings.Contains(got.stdout, "Git rule") || !strings.Contains(got.stdout, web) {
		t.Errorf("stdout is\n%s\nwant a Skip of %s under the Git rule", got.stdout, web)
	}
	body, err := os.ReadFile(kept)
	if err != nil || string(body) != "keep\n" {
		t.Errorf("the tracked file inside the unreadable directory: %q, %v", body, err)
	}
	if _, err := os.Lstat(web); err != nil {
		t.Errorf("the Candidate was moved: %v", err)
	}
	stale := at("projects/stale-app/node_modules")
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Errorf("the rest of the Plan was not moved: %v", err)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_AUTHOR_NAME=Fixture",
		"GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture",
		"GIT_COMMITTER_EMAIL=fixture@example.invalid",
	}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			env = append(env, v)
		}
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
