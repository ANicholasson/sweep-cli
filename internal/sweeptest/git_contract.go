package sweeptest

import (
	"path/filepath"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
)

// GitContract is what the Git rule relies on from a safety.Git.
//
// open builds the Work and returns the Git that answers it and the path of
// its top.
func GitContract(t *testing.T, open func(t *testing.T, work Work) (safety.Git, string)) {
	work := Work{
		Repo: "repo",
		Files: []string{
			"repo/package.json",
			"repo/node_modules/tracked.js",
			"repo/vendor/package.json",
			"repo/vendor/node_modules/loose.js",
			"beside/package.json",
			"beside/node_modules/other.js",
		},
		Tracked: []string{
			"repo/package.json",
			"repo/node_modules/tracked.js",
		},
	}

	t.Run("a directory outside a work tree is outside", func(t *testing.T) {
		git, top := open(t, work)
		if got := git.State(at(top, "beside/node_modules")); got != safety.Outside {
			t.Errorf("State = %v, want Outside", got)
		}
	})

	t.Run("a directory that holds a tracked file is tracked", func(t *testing.T) {
		git, top := open(t, work)
		if got := git.State(at(top, "repo/node_modules")); got != safety.Tracked {
			t.Errorf("State = %v, want Tracked", got)
		}
	})

	t.Run("a directory inside a work tree that holds no tracked file is untracked", func(t *testing.T) {
		git, top := open(t, work)
		if got := git.State(at(top, "repo/vendor/node_modules")); got != safety.Untracked {
			t.Errorf("State = %v, want Untracked", got)
		}
	})

	t.Run("a nested repository's tracked file keeps the directory that holds it", func(t *testing.T) {
		for _, name := range []string{"outside every work tree", "inside a work tree that does not track it"} {
			t.Run(name, func(t *testing.T) {
				nested := Work{
					Nested:  []string{"app/node_modules/pkg"},
					Files:   []string{"app/package.json", "app/node_modules/pkg/keep.js"},
					Tracked: []string{"app/node_modules/pkg/keep.js"},
				}
				if name != "outside every work tree" {
					nested.Repo = "app"
					nested.Tracked = append(nested.Tracked, "app/package.json")
				}
				git, top := open(t, nested)
				if got := git.State(at(top, "app/node_modules")); got != safety.Tracked {
					t.Errorf("State = %v, want Tracked", got)
				}
			})
		}
	})

	t.Run("a nested repository that tracks nothing leaves the directory outside", func(t *testing.T) {
		git, top := open(t, Work{
			Nested: []string{"app/node_modules/pkg"},
			Files:  []string{"app/package.json", "app/node_modules/pkg/keep.js"},
		})
		if got := git.State(at(top, "app/node_modules")); got != safety.Outside {
			t.Errorf("State = %v, want Outside", got)
		}
	})

	t.Run("every directory is unknown when git cannot answer", func(t *testing.T) {
		broken := work
		broken.Broken = true
		git, top := open(t, broken)
		for _, rel := range []string{"repo/node_modules", "beside/node_modules"} {
			if got := git.State(at(top, rel)); got != safety.Unknown {
				t.Errorf("State of %s = %v, want Unknown", rel, got)
			}
		}
	})
}

func at(top, rel string) string {
	return filepath.Join(top, filepath.FromSlash(rel))
}
