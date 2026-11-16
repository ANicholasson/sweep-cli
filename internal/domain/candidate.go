package domain

// Root is a directory a Scan looks beneath, as a path the files adapter
// reads.
type Root string

// Size is the disk space a Candidate occupies on its own, in bytes, with
// each hard-linked file counted once.
type Size int64

// Candidate is a directory or file that a Rule of an Ecosystem recognised.
type Candidate struct {
	Path      string
	Size      Size
	Ecosystem string
	Rule      Rule
}
