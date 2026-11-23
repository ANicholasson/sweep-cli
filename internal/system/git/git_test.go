package git

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestLocalPassesTheGitContract(t *testing.T) {
	sweeptest.GitContract(t, func(t *testing.T, work sweeptest.Work) (safety.Git, string) {
		top := t.TempDir()
		for _, file := range work.Files {
			target := filepath.Join(top, filepath.FromSlash(file))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if work.Repo != "" {
			repo := filepath.Join(top, filepath.FromSlash(work.Repo))
			git(t, repo, "init", "-q", "-b", "main")
			for _, file := range work.Tracked {
				rel, err := filepath.Rel(repo, filepath.Join(top, filepath.FromSlash(file)))
				if err != nil {
					t.Fatal(err)
				}
				git(t, repo, "add", "--", rel)
			}
			git(t, repo, "commit", "-q", "-m", "Track")
		}
		local := Local{}
		if work.Broken {
			local.Bin = filepath.Join(t.TempDir(), "missing-git")
		}
		return local, top
	})
}

func TestLsFilesLeavesTheRepositoryUntouched(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "add", "--", "keep.txt")
	git(t, repo, "commit", "-q", "-m", "Track")
	// read-tree drops the stat cache commit wrote, which is the index shape
	// a later read is most tempted to refresh.
	git(t, repo, "read-tree", "HEAD")
	before := stamp(t, repo)

	if got := (Local{}).State(repo); got != safety.Tracked {
		t.Fatalf("State = %v, want Tracked", got)
	}

	if after := stamp(t, repo); after != before {
		t.Errorf("the repository changed:\n%s", after)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append([]string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_AUTHOR_NAME=Fixture",
		"GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture",
		"GIT_COMMITTER_EMAIL=fixture@example.invalid",
	}, withoutGit(os.Environ())...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func withoutGit(env []string) []string {
	var kept []string
	for _, v := range env {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		kept = append(kept, v)
	}
	return kept
}

func stamp(t *testing.T, repo string) string {
	t.Helper()
	var b strings.Builder
	root := filepath.Join(repo, ".git")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		line := rel + " " + info.Mode().String() + " " + info.ModTime().UTC().String()
		if d.Type().IsRegular() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			line += " " + hex.EncodeToString(sum[:])
		}
		b.WriteString(line)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
