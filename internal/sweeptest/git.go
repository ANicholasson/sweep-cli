package sweeptest

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
)

// Work is a tree a Git port is asked about. Paths are slash-separated and
// relative to the top directory the opener returns.
type Work struct {
	// Files are the regular files in the tree.
	Files []string
	// Repo is the work tree root, or "" when the tree holds no repository.
	Repo string
	// Tracked are the files git tracks. The real adapter commits each in
	// the innermost repository that contains it, and in no other.
	Tracked []string
	// Nested are work trees inside the top, besides Repo. A directory
	// holds a tracked file when any of these repositories tracks one
	// inside it, including a repository the enclosing work tree does not
	// track.
	Nested []string
	// Broken reports Unknown for every path, as when the git binary is missing.
	Broken bool
}

// Git is the fake of safety.Git. The zero value reports Outside for every
// path. Paths under Top stay slash-separated, so a Scan of the in-memory
// tree can ask it with the paths Files returns.
type Git struct {
	broken  bool
	answers map[string]safety.State
}

var _ safety.Git = Git{}

// NewGit answers from work. A directory that holds a tracked file is
// Tracked, whether the enclosing repository or a nested one tracks it.
// Any other directory inside a work tree is Untracked, and a directory
// outside every work tree is Outside.
func NewGit(top string, work Work) Git {
	g := Git{broken: work.Broken}
	if work.Broken {
		return g
	}
	var repos []string
	if work.Repo != "" {
		repos = append(repos, joinUnder(top, work.Repo))
	}
	for _, repo := range work.Nested {
		repos = append(repos, joinUnder(top, repo))
	}
	tracked := map[string]bool{}
	for _, file := range work.Tracked {
		tracked[joinUnder(top, file)] = true
	}
	answers := map[string]safety.State{}
	consider := func(dir string) {
		switch {
		case holdsTracked(dir, tracked):
			answers[dir] = safety.Tracked
		case insideAny(dir, repos):
			answers[dir] = safety.Untracked
		default:
			answers[dir] = safety.Outside
		}
	}
	for _, file := range work.Files {
		for _, dir := range ancestors(joinUnder(top, file)) {
			consider(dir)
		}
	}
	for file := range tracked {
		for _, dir := range ancestors(file) {
			consider(dir)
		}
	}
	g.answers = answers
	return g
}

func holdsTracked(dir string, tracked map[string]bool) bool {
	for file := range tracked {
		if within(file, dir) {
			return true
		}
	}
	return false
}

func insideAny(dir string, repos []string) bool {
	for _, repo := range repos {
		if within(dir, repo) {
			return true
		}
	}
	return false
}

func (g Git) State(p string) safety.State {
	if g.broken {
		return safety.Unknown
	}
	if state, ok := g.answers[p]; ok {
		return state
	}
	return safety.Outside
}

// joinUnder joins rel onto top. A top of slash-separated form, such as Top,
// stays slash-separated. An operating-system temp directory is joined with
// the separator of this machine.
func joinUnder(top, rel string) string {
	if strings.HasPrefix(top, "/") && !strings.Contains(top, `\`) {
		return path.Join(top, rel)
	}
	return filepath.Join(top, filepath.FromSlash(rel))
}

func ancestors(p string) []string {
	var out []string
	for {
		i := strings.LastIndexAny(p, `/\`)
		if i <= 0 {
			break
		}
		p = p[:i]
		if strings.HasSuffix(p, ":") {
			break
		}
		out = append(out, p)
	}
	return out
}

func within(p, dir string) bool {
	if p == dir {
		return true
	}
	sep := "/"
	if strings.Contains(dir, `\`) {
		sep = `\`
	}
	return strings.HasPrefix(p, dir+sep)
}
