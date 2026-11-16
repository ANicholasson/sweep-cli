package domain

// Kind is what sort of leftover a Candidate is. Its only field is unexported,
// so outside this package the five Kinds below and the zero value are the
// only Kinds there are, and Validate refuses the zero value.
type Kind struct{ name string }

var (
	Dependencies = Kind{"Dependencies"}
	BuildOutput  = Kind{"Build output"}
	Cache        = Kind{"Cache"}
	Models       = Kind{"Models"}
	Worktree     = Kind{"Worktree"}
)

var kinds = [...]Kind{Dependencies, BuildOutput, Cache, Models, Worktree}

// String is the Kind's name as GLOSSARY.md writes it.
func (k Kind) String() string { return k.name }
