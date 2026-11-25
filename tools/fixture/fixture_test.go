package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
)

func TestTwoBuildsAreIdentical(t *testing.T) {
	first := tree(t, built(t))
	// git records whole seconds, so builds a second apart show any clock it reads.
	time.Sleep(time.Second)
	second := tree(t, built(t))

	if _, ok := first["projects/tracked-app/.git/index"]; !ok {
		t.Fatal("the Trap repository has no .git/index, so the comparison would miss it")
	}
	for _, d := range differences(first, second) {
		t.Error(d)
	}
}

func TestAnswerKeyMatchesTheFixture(t *testing.T) {
	matchesAnswerKey(t, built(t))
}

func TestPerUserGitFilesLeaveTheFixtureAlone(t *testing.T) {
	for _, configHome := range []string{"XDG_CONFIG_HOME", "HOME"} {
		t.Run(configHome, func(t *testing.T) {
			home := t.TempDir()
			gitDir := filepath.Join(home, ".config", "git")
			if err := os.MkdirAll(gitDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, line := range map[string]string{"ignore": "node_modules\n", "attributes": "* -text\n"} {
				if err := os.WriteFile(filepath.Join(gitDir, name), []byte(line), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			if configHome == "XDG_CONFIG_HOME" {
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			}

			matchesAnswerKey(t, built(t))
		})
	}
}

func matchesAnswerKey(t *testing.T, dir string) {
	t.Helper()
	key := answers(t, dir)

	if key.Version != 1 {
		t.Errorf("version = %d, want 1", key.Version)
	}
	if key.Root != "projects" {
		t.Errorf("root = %q, want %q", key.Root, "projects")
	}
	if len(key.Candidates) == 0 {
		t.Error("the answer key lists no Candidate")
	}
	for _, c := range key.Candidates {
		if !isDir(dir, c.Path) {
			t.Errorf("Candidate %s is not a directory in the Fixture", c.Path)
			continue
		}
		if !beneath(c.Path, key.Root) {
			t.Errorf("Candidate %s is outside the Root %s", c.Path, key.Root)
		}
		kind, ok := recognised(dir, c.Path)
		switch {
		case !ok:
			t.Errorf("Candidate %s is something no Rule recognises", c.Path)
		case kind != c.Kind:
			t.Errorf("Candidate %s has Kind %q, but the Rule that recognises it names %q", c.Path, c.Kind, kind)
		}
	}

	var outside, tracked, linked, bare int
	for _, tr := range key.Traps {
		if !isDir(dir, tr.Path) {
			t.Errorf("Trap %s is not a directory in the Fixture", tr.Path)
			continue
		}
		if strings.TrimSpace(tr.Reason) == "" {
			t.Errorf("Trap %s has no reason", tr.Path)
		}
		if _, ok := recognised(dir, tr.Path); ok {
			switch {
			case !beneath(tr.Path, key.Root):
				outside++
			case linkOnTheWay(dir, tr.Path):
				linked++
			case len(trackedFiles(t, dir, tr.Path)) > 0:
				tracked++
			default:
				t.Errorf("Trap %s is beneath the Root, yet git tracks nothing in it and no Link leads to it", tr.Path)
			}
			continue
		}
		if candidateName(path.Base(tr.Path)) {
			bare++
			continue
		}
		t.Errorf("Trap %s matches no Rule, and its name is not a Rule's Candidate", tr.Path)
	}
	if outside == 0 {
		t.Error("no Trap lies outside the Root")
	}
	if tracked == 0 {
		t.Error("no Trap holds files git tracks")
	}
	if linked == 0 {
		t.Error("no Trap is reached through a Link")
	}
	if bare == 0 {
		t.Error("no Trap is a directory named like a Candidate with no Manifest beside it")
	}
}

func TestCandidateHoldsAHardLinkedPair(t *testing.T) {
	dir := built(t)

	for _, c := range answers(t, dir).Candidates {
		var files []os.FileInfo
		err := filepath.WalkDir(filepath.Join(dir, filepath.FromSlash(c.Path)), func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			info, err := os.Stat(p)
			files = append(files, info)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if !hasPair(files) {
			t.Errorf("no two files in Candidate %s are one hard-linked file", c.Path)
		}
	}
}

func TestACandidateHoldsADirectoryARuleRecognises(t *testing.T) {
	dir := built(t)

	nested := 0
	for _, c := range answers(t, dir).Candidates {
		top := filepath.Join(dir, filepath.FromSlash(c.Path))
		err := filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == top || !d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(dir, p)
			if _, ok := recognised(dir, filepath.ToSlash(rel)); ok {
				nested++
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if nested == 0 {
		t.Error("no Candidate holds a directory a Rule recognises on its own, so nothing shows that a Scan stops at a Candidate")
	}
}

func TestBuildsIntoAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")

	if err := build(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "answer-key.json")); err != nil {
		t.Errorf("no answer key after building into a missing directory: %v", err)
	}
}

func TestRefusesAnythingButAnEmptyOrMissingDirectory(t *testing.T) {
	tests := []struct {
		name  string
		place func(target string) error
	}{
		{
			name: "a directory that holds a file",
			place: func(target string) error {
				if err := os.Mkdir(target, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(target, "notes.txt"), []byte("keep me\n"), 0o644)
			},
		},
		{
			name: "a file",
			place: func(target string) error {
				return os.WriteFile(target, []byte("keep me\n"), 0o644)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := t.TempDir()
			target := filepath.Join(parent, "target")
			if err := tt.place(target); err != nil {
				t.Fatal(err)
			}
			before := tree(t, parent)

			err := build(target)

			if err == nil {
				t.Error("build succeeded, want a refusal")
			}
			for _, d := range differences(before, tree(t, parent)) {
				t.Errorf("the refusal changed something: %s", d)
			}
		})
	}
}

func built(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := build(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func answers(t *testing.T, dir string) answerKey {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "answer-key.json"))
	if err != nil {
		t.Fatal(err)
	}
	var key answerKey
	if err := json.Unmarshal(data, &key); err != nil {
		t.Fatalf("answer-key.json does not parse: %v", err)
	}
	return key
}

// candidateName reports whether a Rule's Candidate is name.
func candidateName(name string) bool {
	for _, e := range ecosystems.All() {
		for _, r := range e.Rules {
			if w, ok := r.Where.(domain.InProject); ok && w.Candidate == name {
				return true
			}
		}
	}
	return false
}

// recognised reports the Kind of the Rule that finds p the way a Scan would,
// by its name and the Manifest beside it.
func recognised(dir, p string) (string, bool) {
	for _, e := range ecosystems.All() {
		for _, r := range e.Rules {
			w, ok := r.Where.(domain.InProject)
			if !ok || path.Base(p) != w.Candidate {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path.Join(path.Dir(p), w.Manifest)))); err == nil {
				return r.Kind.String(), true
			}
		}
	}
	return "", false
}

func isDir(dir, p string) bool {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p)))
	return err == nil && info.IsDir()
}

func trackedFiles(t *testing.T, dir, p string) []string {
	t.Helper()
	out, err := git(filepath.Join(dir, filepath.FromSlash(p)), "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(out))
}

func hasPair(files []os.FileInfo) bool {
	for i, a := range files {
		for _, b := range files[i+1:] {
			if os.SameFile(a, b) {
				return true
			}
		}
	}
	return false
}

func beneath(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

func linkOnTheWay(dir, p string) bool {
	cur := dir
	for _, name := range strings.Split(p, "/") {
		cur = filepath.Join(cur, name)
		info, err := os.Lstat(cur)
		if err != nil {
			return false
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
	}
	return false
}

type node struct {
	kind fs.FileMode
	data string
}

func tree(t *testing.T, dir string) map[string]node {
	t.Helper()
	fsys := os.DirFS(dir)
	nodes := map[string]node{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n := node{kind: d.Type()}
		if d.Type().IsRegular() {
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			n.data = string(data)
		}
		nodes[p] = n
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return nodes
}

func differences(a, b map[string]node) []string {
	var paths []string
	for p := range a {
		paths = append(paths, p)
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	var out []string
	for _, p := range paths {
		x, inA := a[p]
		y, inB := b[p]
		switch {
		case !inB:
			out = append(out, p+" is only in the first tree")
		case !inA:
			out = append(out, p+" is only in the second tree")
		case x.kind != y.kind:
			out = append(out, p+" is a "+x.kind.String()+" in one tree and a "+y.kind.String()+" in the other")
		case x.data != y.data:
			out = append(out, p+" has different bytes")
		}
	}
	return out
}
