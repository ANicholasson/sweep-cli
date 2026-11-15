package main

import (
	"strings"
	"testing"
)

func TestVersionFlagPrintsAVersionAndExitsZero(t *testing.T) {
	var stdout, stderr strings.Builder

	status := run([]string{"--version"}, &stdout, &stderr)

	if status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
	name, version, ok := strings.Cut(strings.TrimSuffix(stdout.String(), "\n"), " ")
	if !ok || name != "sweep" || version == "" {
		t.Errorf("stdout = %q, want %q followed by a version", stdout.String(), "sweep ")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestNoArgumentsIsAUsageError(t *testing.T) {
	var stdout, stderr strings.Builder

	status := run(nil, &stdout, &stderr)

	if status != 2 {
		t.Errorf("exit status = %d, want 2", status)
	}
	if !strings.Contains(stderr.String(), "-version") {
		t.Errorf("stderr = %q, want the usage", stderr.String())
	}
}
