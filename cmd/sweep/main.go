// Command sweep finds the disk space that developer tools leave behind and
// reclaims it safely.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sweep", flag.ContinueOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		if _, err := fmt.Fprintln(stdout, "sweep", version()); err != nil {
			_, _ = fmt.Fprintln(stderr, "sweep:", err)
			return 1
		}
		return 0
	}
	flags.Usage()
	return 2
}

// version is what the go command recorded in the binary, so a release needs
// no -ldflags: a tag for a release, a pseudo-version for a build from a
// checkout, and "(devel)" when nothing was recorded, as under go run.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
