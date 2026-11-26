// Command sweep finds the disk space that developer tools leave behind and
// reclaims it safely.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/report"
	"github.com/ANicholasson/sweep-cli/internal/system/config"
	"github.com/ANicholasson/sweep-cli/internal/system/docker"
	"github.com/ANicholasson/sweep-cli/internal/system/files"
	"github.com/ANicholasson/sweep-cli/internal/system/git"
	"github.com/ANicholasson/sweep-cli/internal/system/trash"
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
		_, _ = fmt.Fprintln(stderr, "       sweep clean --dry-run [--plan file] [--pick path] [Root...]")
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
	switch flags.Arg(0) {
	case "scan":
		return scanCommand(flags.Args()[1:], stdout, stderr)
	case "clean":
		return cleanCommand(flags.Args()[1:], stdout, stderr)
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
	cfg, err := configuration()
	if err != nil {
		say(stderr, err)
		return misused
	}
	if len(given) == 0 {
		given = cfg.Roots
	}
	roots, err := rootsOf(given)
	if err != nil {
		say(stderr, err)
		return misused
	}
	ignore, err := resolveIgnore(cfg.Ignore)
	if err != nil {
		say(stderr, err)
		return misused
	}

	result := scan.Scan(files.Disk{}, git.Local{}, docker.Local{}, roots, ecosystems.All(), time.Now(), scan.Settings{
		StaleAfter: cfg.StaleAfter,
		Ignore:     ignore,
	})
	result.Configuration = cfg.Path

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

func cleanCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sweep clean", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: sweep clean --dry-run [--plan file] [--pick path] [Root...]")
		flags.PrintDefaults()
	}
	dry := flags.Bool("dry-run", false, "write a Plan and remove nothing")
	planPath := flags.String("plan", "sweep-plan.json", "file the Plan is written to")
	var pick repeated
	flags.Var(&pick, "pick", "add this Candidate to the Plan")
	given, err := parseInterspersed(flags, args)
	if err != nil {
		return misused
	}
	if !*dry {
		flags.Usage()
		return misused
	}
	roots, err := rootsOf(given)
	if err != nil {
		say(stderr, err)
		return misused
	}
	var named []string
	for _, p := range pick {
		resolved, err := files.Resolve(p)
		if err != nil {
			say(stderr, fmt.Errorf("%s is not a Candidate: %w", p, err))
			return misused
		}
		named = append(named, resolved)
	}

	disk := files.Disk{}
	result := scan.Scan(disk, git.Local{}, docker.Local{}, roots, ecosystems.All(), time.Now(), scan.Settings{StaleAfter: domain.StaleAfter})
	built, choices, err := plan.Build(disk, result, named)
	if err != nil {
		var unknown plan.UnknownPath
		say(stderr, err)
		if errors.As(err, &unknown) {
			return misused
		}
		return incomplete
	}
	dest, real, err := planDestination(*planPath)
	if err != nil {
		say(stderr, err)
		return misused
	}
	if c, ok := insideCandidate(real, result.Candidates); ok {
		say(stderr, fmt.Errorf("the Plan would be written inside the Candidate %s", c))
		return misused
	}
	data, err := plan.Marshal(built)
	if err != nil {
		say(stderr, err)
		return incomplete
	}
	if err := writePlan(dest, data); err != nil {
		say(stderr, err)
		if errors.Is(err, errNotPlan) {
			return misused
		}
		return incomplete
	}
	if err := report.DryRun(stdout, result, choices, dest); err != nil {
		say(stderr, err)
		return incomplete
	}
	if len(result.Unreadable) > 0 {
		return incomplete
	}
	return done
}

// planDestination splits the Plan path into the path to create and the path
// after every Link on the way. The created path keeps the caller's spelling,
// so a directory that is only a Link to the same place is not rewritten.
// The resolved path is what a Candidate is compared with. The file itself
// is refused when it is a Link.
func planDestination(path string) (write, real string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	parent, base := filepath.Dir(abs), filepath.Base(abs)
	resolved, err := files.ResolveDir(parent)
	if err != nil {
		return "", "", fmt.Errorf("%s cannot be written: %w", path, err)
	}
	real = filepath.Join(resolved, base)
	info, err := os.Lstat(real)
	if os.IsNotExist(err) {
		return abs, real, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("%s cannot be written: %w", path, err)
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return "", "", fmt.Errorf("%s is a Link", path)
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("%s is a directory", path)
	}
	return abs, real, nil
}

func insideCandidate(dest string, candidates []domain.Candidate) (string, bool) {
	disk := files.Disk{}
	want := map[scan.FileID]string{}
	for _, c := range candidates {
		space, err := disk.Space(c.Path)
		if err != nil {
			continue
		}
		want[space.File] = c.Path
	}
	dir := dest
	for {
		next := filepath.Dir(dir)
		if next == dir {
			return "", false
		}
		dir = next
		space, err := disk.Space(dir)
		if err != nil {
			return "", false
		}
		if path, ok := want[space.File]; ok {
			return path, true
		}
	}
}

var errNotPlan = errors.New("exists and is not a Plan")

// writePlan creates path, or replaces it when it is already a Plan. The
// bytes are written and synced in a separate file first. path is replaced
// only after that write finishes. The separate file is removed on every
// failure, and left only after the rename has succeeded.
func writePlan(path string, data []byte) error {
	old, err := os.ReadFile(path)
	if err == nil {
		if !plan.Is(old) {
			return fmt.Errorf("%s %w", path, errNotPlan)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".sweep-plan-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = trash.Drop(tmp)
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	renamed = true
	return nil
}

func say(stderr io.Writer, err error) {
	_, _ = fmt.Fprintln(stderr, "sweep:", strings.ReplaceAll(err.Error(), "\n", "\nsweep: "))
}

type repeated []string

func (r *repeated) String() string { return strings.Join(*r, ",") }

func (r *repeated) Set(s string) error {
	*r = append(*r, s)
	return nil
}

func configuration() (domain.Configuration, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return domain.Configuration{}, err
	}
	return config.Load(dir)
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

// resolveIgnore checks that each path can be followed. The caller's
// spelling is kept. The Scan compares that spelling and the walked path
// after every Link in both has been followed, because a Root keeps an
// earlier Link in the caller's spelling.
func resolveIgnore(paths []string) ([]string, error) {
	out := make([]string, len(paths))
	for i, p := range paths {
		_, err := files.ResolveDir(p)
		if errors.Is(err, os.ErrNotExist) {
			out[i] = p
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s cannot be ignored: %w", p, err)
		}
		out[i] = p
	}
	return out, nil
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
