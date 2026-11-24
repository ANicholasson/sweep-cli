package main

import (
	"errors"
	"strings"
	"testing"
)

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

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

func TestVersionFlagFailsWhenStdoutCannotBeWritten(t *testing.T) {
	var stderr strings.Builder

	status := run([]string{"--version"}, failingWriter{}, &stderr)

	if status == 0 {
		t.Errorf("exit status = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), errWriteFailed.Error()) {
		t.Errorf("stderr = %q, want it to say %q", stderr.String(), errWriteFailed)
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

func TestScanOffersNoFlagForDockerData(t *testing.T) {
	var stdout, stderr strings.Builder

	status := run([]string{"scan", "-h"}, &stdout, &stderr)

	if status != 2 {
		t.Errorf("exit status = %d, want 2", status)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
	if !strings.Contains(stderr.String(), "-json") {
		t.Errorf("stderr = %q, want the scan flags", stderr.String())
	}
	if strings.Contains(strings.ToLower(stderr.String()), "docker") || strings.Contains(stderr.String(), "pick") {
		t.Errorf("stderr = %q, want no flag that picks Docker data", stderr.String())
	}
}

func TestScanFailsWhenStdoutCannotBeWritten(t *testing.T) {
	var stderr strings.Builder

	status := run([]string{"scan", t.TempDir()}, failingWriter{}, &stderr)

	if status != 1 {
		t.Errorf("exit status = %d, want 1", status)
	}
	if !strings.Contains(stderr.String(), errWriteFailed.Error()) {
		t.Errorf("stderr = %q, want it to say %q", stderr.String(), errWriteFailed)
	}
}
