package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/ANicholasson/sweep-cli/internal/tui"
)

func screenCommand(stdin io.Reader, stdout, stderr io.Writer) int {
	in, out, ok := terminalFiles(stdin, stdout)
	if !ok {
		say(stderr, errors.New("no interactive terminal"))
		_, _ = fmt.Fprintln(stderr, "sweep: run sweep scan to list Candidates")
		return misused
	}
	result, _, err := scanTree(nil, true, nil)
	if err != nil {
		say(stderr, err)
		return misused
	}
	if err := show(in, out, tui.New(result)); err != nil {
		say(stderr, err)
		return incomplete
	}
	return done
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

func show(in, out *os.File, screen tui.Screen) error {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(int(in.Fd()), state) }()

	if _, err := io.WriteString(out, "\033[?1049h\033[?25l"); err != nil {
		return err
	}
	defer func() { _, _ = io.WriteString(out, "\033[?25h\033[?1049l") }()

	if err := draw(out, frame(out, screen)); err != nil {
		return err
	}
	var keys tui.Decoder
	buf := make([]byte, 64)
	for {
		n, err := in.Read(buf)
		quit := false
		for _, key := range keys.Push(buf[:n]) {
			if screen.Press(key) {
				quit = true
				break
			}
		}
		if quit {
			return nil
		}
		if n > 0 {
			if drawErr := draw(out, frame(out, screen)); drawErr != nil {
				return drawErr
			}
		}
		if err != nil {
			return err
		}
	}
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
