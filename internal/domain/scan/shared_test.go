package scan_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func cacheEcosystem(where domain.InSharedCache) domain.Ecosystem {
	return domain.Ecosystem{Name: "Node", Rules: []domain.Rule{{
		Kind:   domain.Cache,
		Reason: "npm downloads again whatever a later install needs.",
		Where:  where,
	}}}
}

func cacheWhere() domain.InSharedCache {
	return domain.InSharedCache{
		Linux:   ".npm/_cacache",
		MacOS:   ".npm/_cacache",
		Windows: "AppData/Local/npm-cache/_cacache",
	}
}

func cacheTreePath() string {
	if runtime.GOOS == "windows" {
		return "home/AppData/Local/npm-cache/_cacache/index-v5/a"
	}
	return "home/.npm/_cacache/index-v5/a"
}

func TestASharedCacheInsideHomeIsAddedAndTakenWhole(t *testing.T) {
	nested := "home/.npm/_cacache/pkg"
	if runtime.GOOS == "windows" {
		nested = "home/AppData/Local/npm-cache/_cacache/pkg"
	}
	files := sweeptest.NewFiles(sweeptest.Tree{
		cacheTreePath():                 sweeptest.File{Bytes: 32},
		nested + "/package.json":        sweeptest.File{Bytes: 1},
		nested + "/node_modules/a/x.js": sweeptest.File{Bytes: 1},
		"app/package.json":              sweeptest.File{Bytes: 1},
		"app/node_modules/a/x.js":       sweeptest.File{Bytes: 1},
	})
	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/app"}, []domain.Ecosystem{cacheEcosystem(cacheWhere()), web}, scanNow, scan.Settings{
		StaleAfter:   domain.StaleAfter,
		Home:         sweeptest.Top + "/home",
		SharedCaches: true,
	})

	var cache, project string
	for _, c := range got.Candidates {
		switch c.Rule.Kind {
		case domain.Cache:
			cache = c.Path
			if c.Project != "" || c.Ecosystem != "Node" {
				t.Errorf("Shared cache = %+v, want no Project and Node", c)
			}
		case domain.Dependencies:
			project = c.Path
		default:
			t.Errorf("unexpected Candidate %s", c.Path)
		}
	}
	if project != sweeptest.Top+"/app/node_modules" {
		t.Errorf("project Candidate = %s, want app/node_modules", project)
	}
	want := sweeptest.Top + "/home/.npm/_cacache"
	if runtime.GOOS == "windows" {
		want = sweeptest.Top + "/home/AppData/Local/npm-cache/_cacache"
	}
	if cache != want {
		t.Errorf("Shared cache = %s, want %s", cache, want)
	}
	if len(got.Added) != 1 || string(got.Added[0]) != want {
		t.Errorf("added = %v, want %s", got.Added, want)
	}
	if len(got.Roots) != 2 || string(got.Roots[1]) != want {
		t.Errorf("roots = %v, want the caller's Root and then the Shared cache", got.Roots)
	}
}

func TestALocationOutsideHomeIsNotAdded(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"outside/cache/pack": sweeptest.File{Bytes: 8},
		"home/note":          sweeptest.File{Bytes: 1},
	})
	outside := cacheEcosystem(domain.InSharedCache{Linux: "../outside/cache", MacOS: "../outside/cache", Windows: "../outside/cache"})
	absolute := cacheEcosystem(domain.InSharedCache{Linux: "/outside/cache", MacOS: "/outside/cache", Windows: "/outside/cache"})
	for _, eco := range []domain.Ecosystem{outside, absolute} {
		got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/home"}, []domain.Ecosystem{eco}, scanNow, scan.Settings{
			Home:         sweeptest.Top + "/home",
			SharedCaches: true,
		})
		if len(got.Added) != 0 || len(got.Candidates) != 0 {
			t.Errorf("location %v added %v and found %+v, want nothing outside home", eco.Rules[0].Where, got.Added, got.Candidates)
		}
	}
}

func TestAPathTheAdapterNamesOutsideHomeIsNotAdded(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"home/cache/pack": sweeptest.File{Bytes: 8},
	})
	got := scan.Scan(redirect{files}, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{
		cacheEcosystem(domain.InSharedCache{Linux: "cache", MacOS: "cache", Windows: "cache"}),
	}, scanNow, scan.Settings{Home: sweeptest.Top + "/home", SharedCaches: true})

	if len(got.Added) != 0 || len(got.Candidates) != 0 {
		t.Errorf("added %v, candidates %+v, want nothing: the adapter named a path outside home", got.Added, got.Candidates)
	}
}

func TestSharedCachesOffLooksOnlyBeneathTheGivenRoots(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		cacheTreePath():      sweeptest.File{Bytes: 8},
		"app/package.json":   sweeptest.File{Bytes: 1},
		"app/node_modules/x": sweeptest.File{Bytes: 1},
	})
	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/app"}, []domain.Ecosystem{cacheEcosystem(cacheWhere()), web}, scanNow, scan.Settings{
		StaleAfter: domain.StaleAfter,
		Home:       sweeptest.Top + "/home",
	})
	if len(got.Added) != 0 || len(got.Candidates) != 1 || got.Candidates[0].Path != sweeptest.Top+"/app/node_modules" {
		t.Errorf("added %v, candidates %+v, want only the Candidate beneath the given Root", got.Added, got.Candidates)
	}
}

func TestASplitSharedCacheListsEachEntry(t *testing.T) {
	where := domain.InSharedCache{Linux: "models", MacOS: "models", Windows: "models", Split: true}
	files := sweeptest.NewFiles(sweeptest.Tree{
		"home/models/one/weight":                  sweeptest.File{Bytes: 4},
		"home/models/one/pkg/package.json":        sweeptest.File{Bytes: 1},
		"home/models/one/pkg/node_modules/a/x.js": sweeptest.File{Bytes: 1},
		"home/models/two/weight":                  sweeptest.File{Bytes: 4},
	})
	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/home"}, []domain.Ecosystem{cacheEcosystem(where), web}, scanNow, scan.Settings{
		Home:         sweeptest.Top + "/home",
		SharedCaches: true,
	})
	if len(got.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want the two entries and not the cache or the Project inside one", got.Candidates)
	}
	for _, c := range got.Candidates {
		if c.Rule.Kind != domain.Cache || c.Project != "" {
			t.Errorf("candidate = %+v, want a Cache with no Project", c)
		}
	}
}

func TestAMissingSharedCacheIsNotAdded(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{"home/note": sweeptest.File{Bytes: 1}})
	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/home"}, []domain.Ecosystem{cacheEcosystem(cacheWhere())}, scanNow, scan.Settings{
		Home:         sweeptest.Top + "/home",
		SharedCaches: true,
	})
	if len(got.Added) != 0 || len(got.Candidates) != 0 || len(got.Unreadable) != 0 {
		t.Errorf("added %v, candidates %d, unreadable %v, want nothing and no error", got.Added, len(got.Candidates), got.Unreadable)
	}
}

// redirect reports home/cache at a path outside home. The directory is real
// in the fake; only the path the adapter returns leaves home.
type redirect struct{ scan.Files }

func (r redirect) List(dir string) ([]scan.Entry, error) {
	entries, err := r.Files.List(dir)
	if err != nil {
		return nil, err
	}
	if dir != sweeptest.Top+"/home" {
		return entries, nil
	}
	for i := range entries {
		if entries[i].Name == "cache" {
			entries[i].Path = "/outside/cache"
		}
	}
	return entries, nil
}

func TestAnUnreadableStepOnTheWayIsReportedAndNotAdded(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"home/locked/cache/pack": sweeptest.File{Bytes: 1},
		"app/note":               sweeptest.File{Bytes: 1},
	})
	files.Unreadable("home/locked", errors.New("permission denied"))
	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/app"}, []domain.Ecosystem{
		cacheEcosystem(domain.InSharedCache{Linux: "locked/cache", MacOS: "locked/cache", Windows: "locked/cache"}),
	}, scanNow, scan.Settings{Home: sweeptest.Top + "/home", SharedCaches: true})
	if len(got.Added) != 0 || len(got.Candidates) != 0 {
		t.Errorf("added %v, candidates %+v, want nothing", got.Added, got.Candidates)
	}
	if len(got.Unreadable) != 1 || got.Unreadable[0].Path != sweeptest.Top+"/home/locked" {
		t.Errorf("unreadable = %+v, want the locked directory", got.Unreadable)
	}
}
