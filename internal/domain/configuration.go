package domain

import "time"

// Configuration is what the per-user file set. Path is the file that was
// read, and it is empty when there was no file. Roots are absolute
// directories used when the command line gives none. An empty Roots means
// the current directory. StaleAfter is how old Last used must be before a
// Candidate is Stale. Ignore lists paths a Scan never reports, each one
// and everything beneath it.
type Configuration struct {
	Path       string
	Roots      []string
	StaleAfter time.Duration
	Ignore     []string
}
