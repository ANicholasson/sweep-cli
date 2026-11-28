package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/system/files"
	"github.com/ANicholasson/sweep-cli/internal/tui"
)

func screenCommand(stdin io.Reader, stdout, stderr io.Writer) int {
	in, out, ok := terminalFiles(stdin, stdout)
	if !ok {
		say(stderr, errors.New("no interactive terminal"))
		_, _ = fmt.Fprintln(stderr, "sweep: run sweep scan to list Candidates")
		return misused
	}
	result, ignore, err := scanTree(nil, true, nil)
	if err != nil {
		say(stderr, err)
		return misused
	}
	shown := result
	shown.Candidates = withoutHeldIgnore(result.Candidates, ignore)
	code, err := show(in, out, tui.New(shown), func(picked []domain.Candidate) (int, string) {
		return confirmClean(shown, result.Candidates, ignore, picked)
	})
	if err != nil {
		say(stderr, err)
		return incomplete
	}
	return code
}

func terminalFiles(stdin io.Reader, stdout io.Writer) (*os.File, *os.File, bool) {
	in, ok := stdin.(*os.File)
	if !ok || in == nil {
		return nil, nil, false
	}
	out, ok := stdout.(*os.File)
	if !ok || out == nil {
		return nil, nil, false
	}
	if !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) {
		return nil, nil, false
	}
	return in, out, true
}

func show(in, out *os.File, screen tui.Screen, confirm func([]domain.Candidate) (int, string)) (int, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return done, err
	}
	defer func() { _ = term.Restore(int(in.Fd()), state) }()

	if _, err := io.WriteString(out, "\033[?1049h\033[?25l"); err != nil {
		return done, err
	}
	defer func() { _, _ = io.WriteString(out, "\033[?25h\033[?1049l") }()

	if err := draw(out, frame(out, screen)); err != nil {
		return done, err
	}
	code := done
	var keys tui.Decoder
	buf := make([]byte, 64)
	for {
		n, err := in.Read(buf)
		for _, key := range keys.Push(buf[:n]) {
			switch screen.Press(key) {
			case tui.Quit:
				return code, nil
			case tui.Confirm:
				var text string
				code, text = confirm(screen.Picked())
				screen.ShowOutcome(text)
			}
		}
		if n > 0 {
			if drawErr := draw(out, frame(out, screen)); drawErr != nil {
				return code, drawErr
			}
		}
		if err != nil {
			return code, err
		}
	}
}

func confirmClean(result scan.Result, found []domain.Candidate, ignore []string, picked []domain.Candidate) (int, string) {
	built, err := plan.Record(files.Disk{}, result.Roots, picked)
	if err != nil {
		return incomplete, spoken(err)
	}
	data, err := plan.Marshal(built)
	if err != nil {
		return incomplete, spoken(err)
	}
	chosen, err := defaultPlanPath()
	if err != nil {
		return incomplete, spoken(err)
	}
	var msg bytes.Buffer
	if refusePlan(&msg, chosen, chosen, found, ignore) {
		return misused, msg.String()
	}
	if err := os.MkdirAll(filepath.Dir(chosen), 0o755); err != nil {
		return incomplete, spoken(err)
	}
	dest, real, err := planDestination(chosen)
	if err != nil {
		return misused, spoken(err)
	}
	if refusePlan(&msg, chosen, real, found, ignore) {
		return misused, msg.String()
	}
	if err := writePlan(dest, data); err != nil {
		code := incomplete
		if errors.Is(err, errNotPlan) {
			code = misused
		}
		return code, spoken(err)
	}
	var out, errOut bytes.Buffer
	code := actOnPlan(dest, &out, &errOut)
	return code, out.String() + errOut.String()
}

func spoken(err error) string {
	var buf bytes.Buffer
	say(&buf, err)
	return buf.String()
}

func frame(out *os.File, screen tui.Screen) string {
	width, height, err := term.GetSize(int(out.Fd()))
	if err != nil || height < 2 {
		return screen.Text()
	}
	// A newline on the last row scrolls the first row off the terminal.
	return screen.Page(width, height-1)
}

func draw(out *os.File, text string) error {
	_, err := io.WriteString(out, "\033[H\033[J"+strings.ReplaceAll(text, "\n", "\r\n"))
	return err
}
