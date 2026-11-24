// Command sweep finds the disk space that developer tools leave behind and
// reclaims it safely.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/report"
	"github.com/ANicholasson/sweep-cli/internal/system/files"
	"github.com/ANicholasson/sweep-cli/internal/system/git"
)

const (
	done       = 0
	incomplete = 1
	misused    = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sweep", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: sweep scan [--json] [Root...]")
		_, _ = fmt.Fprintln(stderr, "       sweep --version")
		flags.PrintDefaults()
	}
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return misused
	}
	if *showVersion {
		if _, err := fmt.Fprintln(stdout, "sweep", version()); err != nil {
			_, _ = fmt.Fprintln(stderr, "sweep:", err)
			return incomplete
		}
		return done
	}
	if flags.Arg(0) == "scan" {
		return scanCommand(flags.Args()[1:], stdout, stderr)
	}
	flags.Usage()
	return misused
}

func scanCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sweep scan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "print the Scan as JSON")
	given, err := parseInterspersed(flags, args)
	if err != nil {
		return misused
	}
	roots, err := rootsOf(given)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "sweep:", strings.ReplaceAll(err.Error(), "\n", "\nsweep: "))
		return misused
	}

	result := scan.Scan(files.Disk{}, git.Local{}, roots, ecosystems.All())

	format := report.Table
	if *asJSON {
		format = report.JSON
	}
	if err := report.Scan(stdout, result, format); err != nil {
		_, _ = fmt.Fprintln(stderr, "sweep:", err)
		return incomplete
	}
	if len(result.Unreadable) > 0 {
		return incomplete
	}
	return done
}

func parseInterspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		left := flags.Args()
		stoppedAtDashes := len(left) < len(args) && args[len(args)-len(left)-1] == "--"
		if len(left) == 0 || stoppedAtDashes {
			return append(rest, left...), nil
		}
		rest, args = append(rest, left[0]), left[1:]
	}
}

func rootsOf(given []string) ([]domain.Root, error) {
	if len(given) == 0 {
		given = []string{"."}
	}
	var roots []domain.Root
	var errs []error
	for _, p := range given {
		resolved, err := files.Resolve(p)
		if err == nil {
			var info os.FileInfo
			info, err = os.Lstat(resolved)
			if err == nil && !info.IsDir() {
				err = errors.New("it is not a directory")
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s cannot be a Root: %w", p, err))
			continue
		}
		roots = append(roots, domain.Root(resolved))
	}
	return roots, errors.Join(errs...)
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
