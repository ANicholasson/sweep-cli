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
	// Tracked are the files git tracks. Each sits under Repo. The real
	// adapter commits these and no others.
	Tracked []string
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

// NewGit answers from work. A directory under Repo that holds a tracked
// file is Tracked, any other directory under Repo is Untracked, and a
// directory outside Repo is Outside.
func NewGit(top string, work Work) Git {
	g := Git{broken: work.Broken}
	if work.Broken {
		return g
	}
	repo := ""
	if work.Repo != "" {
		repo = joinUnder(top, work.Repo)
	}
	answers := map[string]safety.State{}
	seed := func(file string, state safety.State) {
		for _, dir := range ancestors(file) {
			if repo == "" || !within(dir, repo) {
				if _, seen := answers[dir]; !seen {
					answers[dir] = safety.Outside
				}
				continue
			}
			if answers[dir] != safety.Tracked {
				answers[dir] = state
			}
		}
	}
	for _, file := range work.Files {
		seed(joinUnder(top, file), safety.Untracked)
	}
	for _, file := range work.Tracked {
		seed(joinUnder(top, file), safety.Tracked)
	}
	g.answers = answers
	return g
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
