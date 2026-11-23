package safety

import "testing"

type script map[string]State

func (s script) State(path string) State { return s[path] }

func TestKeepsFollowsTheGitRule(t *testing.T) {
	git := script{
		"outside": Outside,
		"loose":   Untracked,
		"tracked": Tracked,
		"dark":    Unknown,
	}

	for _, path := range []string{"outside", "loose"} {
		if Keeps(git, path) {
			t.Errorf("the Git rule kept %s, which holds no file git tracks", path)
		}
	}
	for _, path := range []string{"tracked", "dark", "unasked"} {
		if !Keeps(git, path) {
			t.Errorf("the Git rule did not keep %s", path)
		}
	}
}
