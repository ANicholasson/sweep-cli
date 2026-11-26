// Package config reads the per-user configuration file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/ANicholasson/sweep-cli/internal/domain"
)

// Load reads dir/sweep/config.toml. A missing file returns the defaults and
// an empty Path: no Roots, the Stale threshold domain.StaleAfter, an empty
// Ignore list, and Shared caches on. The file is TOML. The only keys are
// roots, stale_days, ignore and shared_caches. Any other key is an error
// that names it, so a key cannot switch off a Safety rule or make Delete
// the default. roots and ignore are lists of absolute paths. stale_days is
// a whole number of days, and zero is a threshold of zero. When stale_days
// is absent the threshold is domain.StaleAfter. shared_caches is true or
// false, and when it is absent Shared caches stay on. A malformed file or
// a bad value names the file and, when a key is at fault, that key.
func Load(dir string) (domain.Configuration, error) {
	path := filepath.Join(dir, "sweep", "config.toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Configuration{StaleAfter: domain.StaleAfter, SharedCaches: true}, nil
	}
	if err != nil {
		return domain.Configuration{}, fmt.Errorf("%s: %w", path, err)
	}
	cfg, err := parse(data)
	if err != nil {
		return domain.Configuration{}, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Path = path
	return cfg, nil
}

func parse(data []byte) (domain.Configuration, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return domain.Configuration{}, err
	}
	var errs []error
	for _, key := range unknownKeys(raw) {
		errs = append(errs, fmt.Errorf("unknown key %q", key))
	}
	cfg := domain.Configuration{StaleAfter: domain.StaleAfter, SharedCaches: true}
	if v, ok := raw["roots"]; ok {
		roots, err := absolutePaths("roots", v)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.Roots = roots
		}
	}
	if v, ok := raw["stale_days"]; ok {
		days, err := daysOf(v)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.StaleAfter = days
		}
	}
	if v, ok := raw["ignore"]; ok {
		paths, err := absolutePaths("ignore", v)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.Ignore = paths
		}
	}
	if v, ok := raw["shared_caches"]; ok {
		on, err := boolOf("shared_caches", v)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.SharedCaches = on
		}
	}
	return cfg, errors.Join(errs...)
}

func boolOf(key string, v any) (bool, error) {
	on, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("key %q: want true or false", key)
	}
	return on, nil
}

func unknownKeys(raw map[string]any) []string {
	var keys []string
	for key := range raw {
		switch key {
		case "roots", "stale_days", "ignore", "shared_caches":
		default:
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func daysOf(v any) (time.Duration, error) {
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("key %q: want a whole number of days", "stale_days")
	}
	if n < 0 {
		return 0, fmt.Errorf("key %q: %d is not a whole number of days", "stale_days", n)
	}
	d := time.Duration(n) * 24 * time.Hour
	if n > 0 && d/(24*time.Hour) != time.Duration(n) {
		return 0, fmt.Errorf("key %q: %d is too large", "stale_days", n)
	}
	return d, nil
}

func absolutePaths(key string, v any) ([]string, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("key %q: want a list of absolute paths", key)
	}
	out := make([]string, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("key %q: want a list of absolute paths", key)
		}
		if s == "" {
			return nil, fmt.Errorf("key %q: empty path", key)
		}
		if !filepath.IsAbs(s) {
			return nil, fmt.Errorf("key %q: %s is not an absolute path", key, s)
		}
		out[i] = filepath.Clean(s)
	}
	return out, nil
}
