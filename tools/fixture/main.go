// Command fixture builds the Fixture, a fake developer machine, into the
// directory it is given, with its Answer key at answer-key.json. The
// directory must be empty or missing. Every byte is written by the tool, and
// two builds are identical, git data included.
//
// Run it from the module root:
//
//	go run ./tools/fixture <dir>
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/fixture <dir>")
		os.Exit(2)
	}
	if err := build(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "fixture:", err)
		os.Exit(1)
	}
}
