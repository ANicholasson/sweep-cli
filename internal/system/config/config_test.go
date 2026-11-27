package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
)

func TestAMissingFileIsTheDefaults(t *testing.T) {
	got, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "" || got.StaleAfter != domain.StaleAfter || len(got.Roots) != 0 || len(got.Ignore) != 0 || !got.SharedCaches {
		t.Errorf("got %+v, want the defaults and no file", got)
	}
}

func TestAnEmptyFileNamesItselfAndKeepsTheDefaults(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "")
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != path || got.StaleAfter != domain.StaleAfter || len(got.Roots) != 0 || len(got.Ignore) != 0 || !got.SharedCaches {
		t.Errorf("got %+v, want %s and the defaults", got, path)
	}
}

func TestTheFileSetsRootsTheThresholdAndIgnore(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects")
	parent := filepath.Dir(root)
	keep := filepath.Join(dir, "keep")
	written := filepath.Join(parent, "projects", "..", "keep")
	path := write(t, dir, "roots = ['"+root+"']\nstale_days = 14\nignore = ['"+written+"']\n")

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != path {
		t.Errorf("Path = %s, want %s", got.Path, path)
	}
	if len(got.Roots) != 1 || got.Roots[0] != root {
		t.Errorf("Roots = %v, want %s", got.Roots, root)
	}
	if got.StaleAfter != 14*24*time.Hour {
		t.Errorf("StaleAfter = %s, want 14 days", got.StaleAfter)
	}
	if len(got.Ignore) != 1 || got.Ignore[0] != keep {
		t.Errorf("Ignore = %v, want %s", got.Ignore, keep)
	}
}

func TestZeroDaysIsAThreshold(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "stale_days = 0\n")
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.StaleAfter != 0 {
		t.Errorf("StaleAfter = %s, want 0", got.StaleAfter)
	}
}

func TestNoKeySwitchesOffASafetyRuleOrMakesDeleteTheDefault(t *testing.T) {
	for _, key := range []string{"git", "root", "fingerprint", "delete"} {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			body := key + " = false\n"
			if key == "delete" {
				body = "delete = true\n"
			}
			write(t, dir, body)
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), `"`+key+`"`) {
				t.Fatalf("error = %v, want it to name %s", err, key)
			}
		})
	}
}

func TestABadFileNamesTheKey(t *testing.T) {
	for name, body := range map[string]string{
		"syntax":     "roots = [\n",
		"days":       "stale_days = \"soon\"\n",
		"negative":   "stale_days = -3\n",
		"fraction":   "stale_days = 1.5\n",
		"relative":   "roots = ['projects']\n",
		"ignore":     "ignore = ['projects']\n",
		"empty":      "ignore = ['']\n",
		"not a list": "roots = '/tmp'\n",
		"unknown":    "colour = 'blue'\n",
		"switch":     "shared_caches = 'no'\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := write(t, dir, body)
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("error = %v, want it to name %s", err, path)
			}
			switch name {
			case "syntax":
			default:
				key := "stale_days"
				if strings.Contains(body, "roots") {
					key = "roots"
				}
				if strings.Contains(body, "ignore") {
					key = "ignore"
				}
				if strings.Contains(body, "shared_caches") {
					key = "shared_caches"
				}
				if name == "unknown" {
					key = "colour"
				}
				if !strings.Contains(err.Error(), key) {
					t.Errorf("error = %v, want it to name %s", err, key)
				}
			}
		})
	}
}

func TestSharedCachesStaysOnUntilTheFileTurnsItOff(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "shared_caches = false\n")
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.SharedCaches {
		t.Errorf("SharedCaches = true, want false")
	}
	write(t, dir, "shared_caches = true\n")
	got, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SharedCaches {
		t.Errorf("SharedCaches = false, want true")
	}
}

func write(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "sweep", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
