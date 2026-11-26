package domain

// Ecosystem is a toolchain whose leftovers sweep knows. A user picks one by
// its Name, so no two Ecosystems share a Name, whatever the case.
type Ecosystem struct {
	Name  string
	Rules []Rule
}

// Rule is one pattern in an Ecosystem that recognises a Candidate.
type Rule struct {
	Kind   Kind
	Reason Reason
	// Where is an InProject or an InSharedCache, and nothing else compiles.
	Where where
}

// Reason is the sentence a Rule gives for why its Candidates are safe to
// reclaim.
type Reason string

// InProject finds a directory named Candidate beside a file named Manifest,
// the file that marks a Project.
type InProject struct {
	Manifest  string
	Candidate string
}

// InSharedCache finds a Shared cache at its location on each operating
// system. A location is a slash-separated path inside the home directory. An
// empty one means the cache does not exist on that operating system.
type InSharedCache struct {
	Linux, MacOS, Windows string
	// Split makes each entry directly inside the cache its own Candidate.
	// Unset, the whole cache is one Candidate.
	Split bool
}

type where interface{ where() }

func (InProject) where()     {}
func (InSharedCache) where() {}

// SharedCache reports whether the Rule finds a Shared cache.
func (r Rule) SharedCache() bool {
	_, ok := r.Where.(InSharedCache)
	return ok
}
