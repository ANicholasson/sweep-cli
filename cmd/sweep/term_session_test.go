package main_test

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type termSession struct {
	mu     sync.Mutex
	raw    bytes.Buffer
	stderr bytes.Buffer
	in     io.WriteCloser
	stop   func()
	done   chan struct{}
	exit   int
	err    error
}

func (s *termSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stderr.Write(p)
}

func (s *termSession) appendOut(p []byte) {
	s.mu.Lock()
	s.raw.Write(p)
	s.mu.Unlock()
}

func (s *termSession) frame() string {
	s.mu.Lock()
	raw := s.raw.String()
	s.mu.Unlock()
	const clear = "\033[H\033[J"
	if i := strings.LastIndex(raw, clear); i >= 0 {
		raw = raw[i+len(clear):]
	}
	return strings.ReplaceAll(raw, "\r", "")
}

func (s *termSession) errText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stderr.String()
}

func (s *termSession) exited() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *termSession) send(t *testing.T, keys []byte) {
	t.Helper()
	if _, err := s.in.Write(keys); err != nil {
		t.Fatalf("send %q: %v\n%s", keys, err, s.frame())
	}
}

func (s *termSession) waitFor(t *testing.T, part string) string {
	t.Helper()
	return s.waitForAll(t, part)
}

func (s *termSession) waitForAll(t *testing.T, parts ...string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		frame := s.frame()
		if containsAll(frame, parts) {
			return frame
		}
		if s.exited() {
			t.Fatalf("screen exited %d before %q\n%s\n%s", s.exit, parts, frame, s.errText())
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("screen did not show %q\n%s\n%s", parts, s.frame(), s.errText())
	return ""
}

func containsAll(frame string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(frame, part) {
			return false
		}
	}
	return true
}

func (s *termSession) finish(t *testing.T) int {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(15 * time.Second):
		t.Fatalf("screen did not exit\n%s\n%s", s.frame(), s.errText())
	}
	if s.err != nil {
		t.Fatal(s.err)
	}
	return s.exit
}

func (s *termSession) close() {
	if s.stop != nil {
		s.stop()
	}
	if s.in != nil {
		_ = s.in.Close()
	}
}

func (s *termSession) highlight(t *testing.T, part string) {
	t.Helper()
	for range 80 {
		if rowMarked(s.frame(), part, "> ") {
			return
		}
		before := s.frame()
		s.send(t, []byte("j"))
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			frame := s.frame()
			if rowMarked(frame, part, "> ") || frame != before {
				break
			}
			if s.exited() {
				t.Fatalf("screen exited while moving to %s\n%s", part, frame)
			}
			time.Sleep(15 * time.Millisecond)
		}
	}
	if !rowMarked(s.frame(), part, "> ") {
		t.Fatalf("did not highlight %s\n%s", part, s.frame())
	}
}

func rowMarked(frame, part, prefix string) bool {
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, part) {
			return true
		}
	}
	return false
}
