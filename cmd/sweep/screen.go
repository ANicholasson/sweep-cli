package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
	for {
		key, err := readKey(in)
		if err != nil {
			return err
		}
		if screen.Press(key) {
			return nil
		}
		if err := draw(out, frame(out, screen)); err != nil {
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

// An arrow is the escape sequence when it arrives together, and the bytes
// that follow a lone escape when the terminal sends them just after.
func readKey(in *os.File) ([]byte, error) {
	buf := make([]byte, 8)
	n, err := in.Read(buf)
	if err != nil {
		return nil, err
	}
	if n == 1 && buf[0] == 0x1b {
		more, err := readEscape(in)
		if err != nil {
			return nil, err
		}
		if len(more) == 0 {
			return buf[:1], nil
		}
		return append([]byte{0x1b}, more...), nil
	}
	return buf[:n], nil
}

func readEscape(in *os.File) ([]byte, error) {
	if err := in.SetReadDeadline(time.Now().Add(25 * time.Millisecond)); err != nil {
		return nil, nil
	}
	defer func() { _ = in.SetReadDeadline(time.Time{}) }()
	buf := make([]byte, 7)
	n, err := in.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, nil
	}
	return nil, err
}
