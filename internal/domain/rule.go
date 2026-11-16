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

// Candidate is a directory or file that a Rule of an Ecosystem recognised.
type Candidate struct {
	Path      string
	Ecosystem string
	Rule      Rule
}

type where interface{ where() }

func (InProject) where()     {}
func (InSharedCache) where() {}
