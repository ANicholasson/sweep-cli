package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseDFSumsImagesAndBuildCache(t *testing.T) {
	out := []byte(strings.Join([]string{
		`{"Type":"Images","Size":"1.5kB","Active":"0","TotalCount":"1","Reclaimable":"0B"}`,
		`{"Type":"Containers","Size":"9GB"}`,
		`{"Type":"Local Volumes","Size":"4MB"}`,
		`{"Type":"Build Cache","Size":"2MB"}`,
	}, "\n"))

	got, err := parseDF(out)
	if err != nil {
		t.Fatal(err)
	}
	// 1.5kB is 1500 bytes and 2MB is 2000000. Containers and volumes are not Docker data.
	if got != 1500+2_000_000 {
		t.Errorf("size = %d, want 2001500", got)
	}
}

func TestParseDFReadsAZeroTotal(t *testing.T) {
	got, err := parseDF([]byte("{\"Type\":\"Images\",\"Size\":\"0B\"}\n{\"Type\":\"Build Cache\",\"Size\":\"0B\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("size = %d, want 0", got)
	}
}

func TestParseDFUsesDockerDecimalUnits(t *testing.T) {
	for text, want := range map[string]int64{
		"1.086GB": 1_086_000_000,
		"1.024kB": 1024,
		"512B":    512,
	} {
		got, err := parseHuman(text)
		if err != nil || got != want {
			t.Errorf("%s = %d, %v, want %d", text, got, err, want)
		}
	}
}

func TestParseDFRefusesAnAnswerThatIsNotBothTotals(t *testing.T) {
	for _, out := range []string{
		`{"Type":"Images","Size":"1B"}`,
		`{"Type":"Images","Size":"1B"}` + "\n" + `{"Type":"Images","Size":"2B"}` + "\n" + `{"Type":"Build Cache","Size":"1B"}`,
		"not json",
		`{"Type":"Images","Size":"soon"}` + "\n" + `{"Type":"Build Cache","Size":"0B"}`,
	} {
		if _, err := parseDF([]byte(out)); err == nil {
			t.Errorf("parseDF(%q) succeeded, want an error", out)
		}
	}
}

func TestLocalReadsSystemDFAndNothingElse(t *testing.T) {
	bin := buildStub(t)
	log := filepath.Join(t.TempDir(), "args")
	t.Setenv("SWEEP_STUB_LOG", log)

	t.Run("images and build cache", func(t *testing.T) {
		t.Setenv("SWEEP_STUB_MODE", "ok")
		got := (Local{Bin: bin}).Data()
		size, ok := got.Size()
		if !ok || size != 1500+2_000_000 || got.Mark() != "reported only" {
			t.Fatalf("Data() = %d, %v, %q, want 2001500 reported only", size, ok, got.Mark())
		}
		assertDFCommand(t, log)
	})

	t.Run("daemon down", func(t *testing.T) {
		t.Setenv("SWEEP_STUB_MODE", "down")
		got := (Local{Bin: bin}).Data()
		if _, ok := got.Size(); ok || got.Mark() != "docker is not running" {
			t.Fatalf("Data() mark = %q, want docker is not running and no size", got.Mark())
		}
		assertDFCommand(t, log)
	})

	t.Run("unreadable answer", func(t *testing.T) {
		t.Setenv("SWEEP_STUB_MODE", "bad")
		got := (Local{Bin: bin}).Data()
		if _, ok := got.Size(); ok || got.Mark() != "docker did not answer" {
			t.Fatalf("Data() mark = %q, want docker did not answer and no size", got.Mark())
		}
	})

	t.Run("current daemon message", func(t *testing.T) {
		t.Setenv("SWEEP_STUB_MODE", "down-api")
		got := (Local{Bin: bin}).Data()
		if _, ok := got.Size(); ok || got.Mark() != "docker is not running" {
			t.Fatalf("Data() mark = %q, want docker is not running and no size", got.Mark())
		}
	})
}

func TestLocalStopsWaitingWhenDockerStalls(t *testing.T) {
	bin := buildStub(t)
	t.Setenv("SWEEP_STUB_MODE", "hang")
	started := time.Now()
	got := (Local{Bin: bin, Wait: 200 * time.Millisecond}).Data()
	elapsed := time.Since(started)
	if _, ok := got.Size(); ok || got.Mark() != "docker did not answer" {
		t.Fatalf("Data() mark = %q, want docker did not answer and no size", got.Mark())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Data waited %s, want the deadline to end it", elapsed)
	}
}

func TestLocalWhenDockerIsMissing(t *testing.T) {
	got := (Local{Bin: filepath.Join(t.TempDir(), "missing-docker")}).Data()
	if _, ok := got.Size(); ok || got.Mark() != "docker is missing" {
		t.Fatalf("Data() mark = %q, want docker is missing and no size", got.Mark())
	}

	t.Setenv("PATH", t.TempDir())
	got = (Local{}).Data()
	if _, ok := got.Size(); ok || got.Mark() != "docker is missing" {
		t.Fatalf("Data() from PATH mark = %q, want docker is missing and no size", got.Mark())
	}
}

func assertDFCommand(t *testing.T, log string) {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "system\ndf\n--format\n{{json .}}\n" {
		t.Fatalf("command = %q, want only docker system df", data)
	}
}

func buildStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(stubSource), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := "module stubdocker\n\ngo " + strings.TrimPrefix(runtime.Version(), "go") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "stub"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, msg)
	}
	return out
}

const stubSource = `package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

func main() {
	got := os.Args[1:]
	_ = os.WriteFile(os.Getenv("SWEEP_STUB_LOG"), []byte(strings.Join(got, "\n")+"\n"), 0o644)
	want := []string{"system", "df", "--format", "{{json .}}"}
	if !slices.Equal(got, want) {
		fmt.Fprintf(os.Stderr, "refusing %q\n", got)
		os.Exit(2)
	}
	switch os.Getenv("SWEEP_STUB_MODE") {
	case "down":
		fmt.Fprintln(os.Stderr, "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")
		os.Exit(1)
	case "down-api":
		fmt.Fprintln(os.Stderr, "failed to connect to the docker API at unix:///var/run/docker.sock; check if the path is correct and if the daemon is running")
		os.Exit(1)
	case "hang":
		time.Sleep(time.Hour)
	case "bad":
		fmt.Println("nope")
	default:
		fmt.Println("{\"Type\":\"Images\",\"Size\":\"1.5kB\"}")
		fmt.Println("{\"Type\":\"Containers\",\"Size\":\"1B\"}")
		fmt.Println("{\"Type\":\"Local Volumes\",\"Size\":\"0B\"}")
		fmt.Println("{\"Type\":\"Build Cache\",\"Size\":\"2MB\"}")
	}
}
`
