// Package safety holds the Safety rules. A Scan applies the Git rule and the
// Root rule, and a Clean calls the same checks again before it touches a path.
package safety

// State is what the Git rule knows about one path.
type State uint8

const (
	// Outside means the path is not inside a work tree. The Git rule does
	// not keep it.
	Outside State = iota + 1
	// Untracked means the path is inside a work tree and holds no file git
	// tracks. The Git rule does not keep it.
	Untracked
	// Tracked means the path holds a file git tracks. The Git rule keeps it.
	Tracked
	// Unknown means git is missing or failed, so there is no answer. The
	// Git rule keeps the path.
	Unknown
)

// Git is the port that reports the State of a path. It only reads.
type Git interface {
	State(path string) State
}

// Keeps reports whether the Git rule keeps path. A path it keeps is never
// a Candidate. It keeps a path that holds a file git tracks, and any path
// whose State git cannot report, because a missing git cannot say whether
// the path is inside a work tree.
func Keeps(git Git, path string) bool {
	switch git.State(path) {
	case Outside, Untracked:
		return false
	default:
		return true
	}
}
