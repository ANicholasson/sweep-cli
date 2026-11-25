package domain

import "time"

// Root is a directory a Scan looks beneath, as a path the files adapter
// reads.
type Root string

// Size is the disk space a Candidate occupies on its own, in bytes, with
// each hard-linked file counted once.
type Size int64

// StaleAfter is how old a Candidate's Last used must be before a Scan marks
// it Stale.
const StaleAfter = 30 * 24 * time.Hour

// Candidate is a directory or file that a Rule of an Ecosystem recognised.
type Candidate struct {
	Path      string
	Size      Size
	Ecosystem string
	Rule      Rule
	// Project is the directory the Ecosystem's manifest marks. It is empty
	// when the Candidate has no Project.
	Project string
	// LastUsed is the newest modification among the Project's own files,
	// outside its Candidates. A Candidate with no Project takes the newest
	// modification inside it.
	LastUsed time.Time
	// Stale is true when LastUsed is older than StaleAfter.
	Stale bool
}
