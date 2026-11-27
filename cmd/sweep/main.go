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
	"strconv"
	"strings"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
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
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sweep", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: sweep")
		_, _ = fmt.Fprintln(stderr, "       sweep scan [--json] [--no-shared-caches] [Root...]")
		_, _ = fmt.Fprintln(stderr, "       "+cleanDryUsage)
		_, _ = fmt.Fprintln(stderr, "       "+cleanActUsage)
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
	case "":
		return screenCommand(stdin, stdout, stderr)
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
	noShared := flags.Bool("no-shared-caches", false, "do not add Shared cache Roots")
	given, err := parseInterspersed(flags, args)
	if err != nil {
		return misused
	}
	result, _, err := scanTree(given, !*noShared, nil)
	if err != nil {
		say(stderr, err)
		return misused
	}

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
		writeCleanUsage(stderr)
		flags.PrintDefaults()
	}
	dry := flags.Bool("dry-run", false, "write a Plan and remove nothing")
	planPath := flags.String("plan", "", "file the Plan is written to, instead of the per-user cache")
	noShared := flags.Bool("no-shared-caches", false, "do not add Shared cache Roots")
	var pick repeated
	flags.Var(&pick, "pick", "add this Candidate to the Plan")
	var kind repeated
	flags.Var(&kind, "kind", "narrow the Pick to this Kind; repeat to name more than one")
	var ecosystem repeated
	flags.Var(&ecosystem, "ecosystem", "narrow the Pick to this Ecosystem; repeat to name more than one")
	var caches repeated
	flags.Var(&caches, "pick-caches", "pick the Shared caches of this Ecosystem")
	var staleDays daysFlag
	flags.Var(&staleDays, "stale-days", "days before a Candidate is Stale, for this run")
	all := flags.Bool("all", false, "pick every Candidate, whatever its age")
	given, err := parseInterspersed(flags, args)
	if err != nil {
		return misused
	}
	if !*dry {
		if *planPath == "" || len(pick) > 0 || len(kind) > 0 || len(ecosystem) > 0 || len(caches) > 0 || *noShared || *all || staleDays.set || len(given) > 0 {
			writeCleanUsage(stderr)
			return misused
		}
		return actOnPlan(*planPath, stdout, stderr)
	}
	kinds, err := kindsOf(kind)
	if err != nil {
		say(stderr, err)
		return misused
	}
	names, err := ecosystemsOf(ecosystem)
	if err != nil {
		say(stderr, err)
		return misused
	}
	var staleAfter *time.Duration
	if staleDays.set {
		staleAfter = &staleDays.after
	}
	result, ignore, err := scanTree(given, !*noShared, staleAfter)
	if err != nil {
		say(stderr, err)
		return misused
	}
	// A Candidate is taken whole, so one that holds an Ignore path cannot
	// be in the Plan: a Clean would remove that path. The location check
	// still uses the Candidates the Scan found, so a Plan inside that
	// Candidate or its Project is refused.
	found := result.Candidates
	result.Candidates = withoutHeldIgnore(result.Candidates, ignore)
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
	built, choices, err := plan.Build(disk, result, plan.Pick{
		Named:      named,
		Kinds:      kinds,
		Ecosystems: names,
		Caches:     []string(caches),
		AnyAge:     *all,
		StaleNamed: staleDays.set,
	})
	if err != nil {
		var unknown plan.UnknownPath
		var ecosystem plan.UnknownEcosystem
		say(stderr, err)
		if errors.As(err, &unknown) || errors.As(err, &ecosystem) {
			return misused
		}
		return incomplete
	}
	chosen := *planPath
	if chosen == "" {
		chosen, err = defaultPlanPath()
		if err != nil {
			say(stderr, err)
			return incomplete
		}
	}
	// Refuse before creating a directory. A Plan inside a Project becomes
	// that Project's newest file, and the next dry run then drops it.
	if refusePlan(stderr, chosen, chosen, found, ignore) {
		return misused
	}
	if *planPath == "" {
		if err := os.MkdirAll(filepath.Dir(chosen), 0o755); err != nil {
			say(stderr, err)
			return incomplete
		}
	}
	dest, real, err := planDestination(chosen)
	if err != nil {
		say(stderr, err)
		return misused
	}
	if refusePlan(stderr, chosen, real, found, ignore) {
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

// actOnPlan carries out the Plan at planPath. It removes nothing until the
// Plan has been read and the trash can take a Candidate. A Candidate the
// trash cannot take stays where it was.
func actOnPlan(planPath string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(planPath)
	if err != nil {
		say(stderr, err)
		return incomplete
	}
	parsed, err := plan.Unmarshal(data)
	if err != nil {
		say(stderr, err)
		return misused
	}
	mover, err := trash.Open()
	if err != nil {
		say(stderr, err)
		if errors.Is(err, trash.ErrUnavailable) {
			return misused
		}
		return incomplete
	}
	result := clean.Run(files.Disk{}, git.Local{}, mover, parsed)
	if err := report.Clean(stdout, result); err != nil {
		say(stderr, err)
		return incomplete
	}
	if len(result.Skipped) > 0 || len(result.Failed) > 0 {
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

// defaultPlanPath is sweep/plan.json in the operating system's per-user
// cache directory. The Plan is not written in the current directory, so a
// dry run from inside a Project does not change that Project's Last used.
func defaultPlanPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sweep", "plan.json"), nil
}

// refusePlan reports whether planPath would land inside an Ignore path, a
// Candidate or a Project and prints the usage error. where is the path to
// judge, after a Link on the way has been followed when it can be.
// planPath is the path named in the message.
func refusePlan(stderr io.Writer, planPath, where string, candidates []domain.Candidate, ignore []string) bool {
	if p, ok := insideIgnore(where, ignore); ok {
		say(stderr, fmt.Errorf("the Plan %s would be written inside the Ignore path %s", planPath, p))
		return true
	}
	if c, ok := insideCandidate(where, candidates); ok {
		say(stderr, fmt.Errorf("the Plan would be written inside the Candidate %s", c))
		return true
	}
	if project, ok := insideProject(where, candidates); ok {
		say(stderr, fmt.Errorf("the Plan %s would be written inside the Project %s", planPath, project))
		return true
	}
	return false
}

// insideIgnore reports the Ignore path that contains where. where may be
// the Ignore path itself. The spelling from the Configuration is what the
// message names.
//
// withoutHeldIgnore drops a Candidate that holds an Ignore path. The
// Candidate itself being the Ignore path is already absent from the Scan.
func withoutHeldIgnore(candidates []domain.Candidate, ignore []string) []domain.Candidate {
	if len(ignore) == 0 {
		return candidates
	}
	kept := make([]domain.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if !holdsIgnore(c.Path, ignore) {
			kept = append(kept, c)
		}
	}
	return kept
}

func holdsIgnore(path string, ignore []string) bool {
	for _, p := range ignore {
		if p != path && ignoreCovers(path, p) {
			return true
		}
	}
	return false
}

func insideIgnore(where string, ignore []string) (string, bool) {
	if len(ignore) == 0 {
		return "", false
	}
	landedWhere, err := landed(where)
	if err != nil {
		return "", false
	}
	for _, p := range ignore {
		if ignoreCovers(p, landedWhere) {
			return p, true
		}
	}
	return "", false
}

func ignoreCovers(root, path string) bool {
	abs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	if safety.Beneath(abs, path, false) {
		return true
	}
	formed, err := (files.Disk{}).EachLink(abs)
	if err != nil {
		return false
	}
	target := path
	if full, err := (files.Disk{}).EachLink(path); err == nil {
		target = full
	}
	return safety.Beneath(formed, target, false)
}

func insideCandidate(dest string, candidates []domain.Candidate) (string, bool) {
	paths := make([]string, 0, len(candidates))
	for _, c := range candidates {
		paths = append(paths, c.Path)
	}
	return insideDirectory(dest, paths)
}

func insideProject(dest string, candidates []domain.Candidate) (string, bool) {
	var paths []string
	seen := map[string]bool{}
	for _, c := range candidates {
		if c.Project == "" || seen[c.Project] {
			continue
		}
		seen[c.Project] = true
		paths = append(paths, c.Project)
	}
	return insideDirectory(dest, paths)
}

// insideDirectory reports which of dirs contains dest. A directory is
// matched by file identity, so another spelling of it counts. A Link on
// the way is followed. A directory that does not exist yet is skipped, so
// the directory that would hold the Plan is still found.
func insideDirectory(dest string, dirs []string) (string, bool) {
	disk := files.Disk{}
	want := map[scan.FileID]string{}
	for _, p := range dirs {
		space, err := disk.Space(p)
		if err != nil {
			continue
		}
		if _, ok := want[space.File]; !ok {
			want[space.File] = p
		}
	}
	real, err := landed(dest)
	if err != nil {
		return "", false
	}
	dir := real
	for {
		next := filepath.Dir(dir)
		if next == dir {
			return "", false
		}
		dir = next
		space, err := disk.Space(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false
		}
		if path, ok := want[space.File]; ok {
			return path, true
		}
	}
}

// landed is where dest would be written once every existing Link on the
// way is followed. Components that are not there yet stay on the end.
func landed(dest string) (string, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	var missing []string
	cur := abs
	for {
		_, err := os.Lstat(cur)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
	resolved, err := files.ResolveDir(cur)
	if err != nil {
		return "", err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, missing[i])
	}
	return resolved, nil
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
	if err := trash.Place(tmp, path); err != nil {
		return err
	}
	renamed = true
	return nil
}

func say(stderr io.Writer, err error) {
	_, _ = fmt.Fprintln(stderr, "sweep:", strings.ReplaceAll(err.Error(), "\n", "\nsweep: "))
}

const (
	cleanDryUsage = "sweep clean --dry-run [--plan file] [--pick path] [--pick-caches Ecosystem] [--kind Kind] [--ecosystem Name] [--stale-days n] [--all] [--no-shared-caches] [Root...]"
	cleanActUsage = "sweep clean --plan file"
)

func writeCleanUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: "+cleanDryUsage)
	_, _ = fmt.Fprintln(w, "       "+cleanActUsage)
}

type daysFlag struct {
	set   bool
	after time.Duration
}

func (d *daysFlag) String() string {
	if !d.set {
		return ""
	}
	return strconv.Itoa(int(d.after / (24 * time.Hour)))
}

func (d *daysFlag) Set(s string) error {
	n, err := strconv.ParseInt(s, 10, 0)
	if err != nil || n < 0 {
		return fmt.Errorf("%s is not a whole number of days", s)
	}
	days := time.Duration(n) * 24 * time.Hour
	if n > 0 && days/(24*time.Hour) != time.Duration(n) {
		return fmt.Errorf("%d is too large", n)
	}
	d.set = true
	d.after = days
	return nil
}

func kindsOf(names []string) ([]domain.Kind, error) {
	var out []domain.Kind
	var errs []error
	for _, name := range names {
		k, ok := domain.KindByName(name)
		if !ok {
			errs = append(errs, fmt.Errorf("unknown Kind %q", name))
			continue
		}
		out = append(out, k)
	}
	return out, errors.Join(errs...)
}

func ecosystemsOf(names []string) ([]string, error) {
	known := map[string]bool{}
	for _, e := range ecosystems.All() {
		known[e.Name] = true
	}
	var out []string
	var errs []error
	for _, name := range names {
		if !known[name] {
			errs = append(errs, fmt.Errorf("unknown Ecosystem %q", name))
			continue
		}
		out = append(out, name)
	}
	return out, errors.Join(errs...)
}

type repeated []string

func (r *repeated) String() string { return strings.Join(*r, ",") }

func (r *repeated) Set(s string) error {
	*r = append(*r, s)
	return nil
}

func scanTree(given []string, shared bool, staleAfter *time.Duration) (scan.Result, []string, error) {
	cfg, err := configuration()
	if err != nil {
		return scan.Result{}, nil, err
	}
	if len(given) == 0 {
		given = cfg.Roots
	}
	roots, err := rootsOf(given)
	if err != nil {
		return scan.Result{}, nil, err
	}
	ignore, err := resolveIgnore(cfg.Ignore)
	if err != nil {
		return scan.Result{}, nil, err
	}
	after := cfg.StaleAfter
	if staleAfter != nil {
		after = *staleAfter
	}
	settings := scan.Settings{StaleAfter: after, Ignore: ignore}
	if shared && cfg.SharedCaches {
		if home, err := os.UserHomeDir(); err == nil {
			settings.Home = home
			settings.SharedCaches = true
		}
	}
	result := scan.Scan(files.Disk{}, git.Local{}, docker.Local{}, roots, ecosystems.All(), time.Now(), settings)
	result.Configuration = cfg.Path
	return result, ignore, nil
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
