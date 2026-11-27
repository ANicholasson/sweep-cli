package plan_test

import (
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestASharedCacheIsPickedOnlyWhenNamed(t *testing.T) {
	// The binary cannot show a Shared cache: Scan leaves those Rules alone.
	files := sweeptest.NewFiles(sweeptest.Tree{
		"home/.npm/_cacache/index": sweeptest.File{Bytes: 8},
		"app/.cache/keep.txt":      sweeptest.File{Bytes: 8},
		"app/node_modules/a.js":    sweeptest.File{Bytes: 8},
	})
	cache := aged("/top/home/.npm/_cacache", "Node", domain.Rule{Kind: domain.Cache, Where: domain.InSharedCache{Linux: ".npm/_cacache"}})
	project := aged("/top/app/.cache", "Node", domain.Rule{Kind: domain.Cache, Where: domain.InProject{Manifest: "package.json", Candidate: ".cache"}})
	deps := aged("/top/app/node_modules", "Node", domain.Rule{Kind: domain.Dependencies, Where: domain.InProject{Manifest: "package.json", Candidate: "node_modules"}})
	result := scan.Result{
		Roots:      []domain.Root{sweeptest.Top},
		Candidates: []domain.Candidate{cache, project, deps},
		StaleAfter: 90 * 24 * time.Hour,
	}

	t.Run("kind, ecosystem, age and --all leave the Shared cache", func(t *testing.T) {
		built, choices, err := plan.Build(files, result, plan.Pick{
			Kinds:      []domain.Kind{domain.Cache},
			Ecosystems: []string{"Node"},
			AnyAge:     true,
			StaleNamed: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if paths := pickedPaths(built); len(paths) != 1 || paths[0] != project.Path {
			t.Fatalf("picked = %v, want only the Cache that is not a Shared cache", paths)
		}
		if reason, picked := choiceFor(t, choices, cache.Path); picked || reason != "Shared cache" {
			t.Errorf("Shared cache picked = %v, reason = %q, want left out as a Shared cache", picked, reason)
		}
		if reason, picked := choiceFor(t, choices, deps.Path); picked || reason != "not named with --kind" {
			t.Errorf("Dependencies picked = %v, reason = %q, want left out by --kind", picked, reason)
		}
	})

	t.Run("the default leaves a Stale Shared cache", func(t *testing.T) {
		built, choices, err := plan.Build(files, result, plan.Pick{})
		if err != nil {
			t.Fatal(err)
		}
		if paths := pickedPaths(built); len(paths) != 2 {
			t.Fatalf("picked = %v, want the two Candidates that are not a Shared cache", paths)
		}
		if reason, picked := choiceFor(t, choices, cache.Path); picked || reason != "Shared cache" {
			t.Errorf("Shared cache picked = %v, reason = %q", picked, reason)
		}
	})

	t.Run("--pick names a Shared cache", func(t *testing.T) {
		built, choices, err := plan.Build(files, result, plan.Pick{Named: []string{cache.Path}})
		if err != nil {
			t.Fatal(err)
		}
		if reason, picked := choiceFor(t, choices, cache.Path); !picked || reason != "named with --pick" {
			t.Errorf("Shared cache picked = %v, reason = %q, want it named with --pick", picked, reason)
		}
		holds := false
		for _, c := range built.Picked {
			if c.Path == cache.Path {
				holds = true
			}
		}
		if !holds {
			t.Errorf("the Plan does not hold %s", cache.Path)
		}
	})
}

func aged(path, ecosystem string, rule domain.Rule) domain.Candidate {
	return domain.Candidate{
		Path:      path,
		Ecosystem: ecosystem,
		Rule:      rule,
		LastUsed:  time.Now().Add(-60 * 24 * time.Hour),
		Stale:     true,
	}
}

func pickedPaths(built plan.Plan) []string {
	var paths []string
	for _, c := range built.Picked {
		paths = append(paths, c.Path)
	}
	return paths
}

func choiceFor(t *testing.T, choices []plan.Choice, path string) (string, bool) {
	t.Helper()
	for _, c := range choices {
		if c.Candidate.Path == path {
			return c.Reason, c.Picked
		}
	}
	t.Fatalf("%s is not a Choice", path)
	return "", false
}
