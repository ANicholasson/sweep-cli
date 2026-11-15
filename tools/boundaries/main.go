// Command boundaries holds the package layout of
// docs/adr/0001-hexagonal-packages-with-feature-folders.md. It reads every
// Go file in the module, whatever operating system or build tag the file is
// for, and exits non-zero when one of them breaks a boundary.
//
// Run it from the module root:
//
//	go run ./tools/boundaries
package main

import (
	"fmt"
	"os"
)

func main() {
	found, err := check(os.DirFS("."), boundaries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "boundaries:", err)
		os.Exit(2)
	}
	if len(found) == 0 {
		return
	}
	broken := map[string]bool{}
	for _, v := range found {
		fmt.Fprintln(os.Stderr, v)
		broken[v.boundary] = true
	}
	fmt.Fprintln(os.Stderr)
	for _, b := range boundaries {
		if name, sentence := b.rule(); broken[name] {
			fmt.Fprintf(os.Stderr, "%s: %s\n", name, sentence)
		}
	}
	fmt.Fprintln(os.Stderr, "The boundaries are listed in tools/boundaries/boundaries.go.")
	os.Exit(1)
}
