package main_test

import (
	"testing"
)

func TestSweepWithoutATerminalLeavesTheFixtureAndPointsAtScan(t *testing.T) {
	b := build(t)
	dir, _ := b.buildFixture(t)
	before := listing(t, dir)

	got := b.run(t, dir)

	if got.status != 2 {
		t.Errorf("exit status = %d, want 2", got.status)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want nothing", got.stdout)
	}
	const want = "sweep: no interactive terminal\nsweep: run sweep scan to list Candidates\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("the Fixture changed: %s", d)
	}
}
