//go:build windows

package main_test

import (
	"runtime"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startTerminal(t *testing.T, bin, dir string, env []string) *termSession {
	t.Helper()
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		t.Fatal(err)
	}
	var pc windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 200, Y: 80}, inRead, outWrite, 0, &pc); err != nil {
		t.Fatal(err)
	}

	attr, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := attr.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, pseudoConsolePtr(pc), unsafe.Sizeof(pc)); err != nil {
		t.Fatal(err)
	}
	cmdLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{bin}))
	if err != nil {
		t.Fatal(err)
	}
	dirp, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	block, err := windowsEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	si := &windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.ProcThreadAttributeList = attr.List()
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	err = windows.CreateProcess(nil, cmdLine, nil, nil, false, flags, &block[0], dirp, &si.StartupInfo, &pi)
	runtime.KeepAlive(block)
	runtime.KeepAlive(cmdLine)
	runtime.KeepAlive(pc)
	if err != nil {
		attr.Delete()
		t.Fatal(err)
	}
	// The pseudoconsole has its own copy of these ends. Close them only
	// after the child exists, as the console session is established then.
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)
	_ = windows.CloseHandle(pi.Thread)

	s := &termSession{in: &pipeWriter{h: inWrite}, done: make(chan struct{})}
	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			var n uint32
			readErr := windows.ReadFile(outRead, buf, &n, nil)
			if n > 0 {
				s.appendOut(append([]byte(nil), buf[:n]...))
			}
			if readErr != nil {
				close(readDone)
				return
			}
		}
	}()
	go func() {
		_, _ = windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		var code uint32
		_ = windows.GetExitCodeProcess(pi.Process, &code)
		s.exit = int(code)
		_ = windows.CloseHandle(pi.Process)
		windows.ClosePseudoConsole(pc)
		attr.Delete()
		<-readDone
		_ = windows.CloseHandle(outRead)
		close(s.done)
	}()
	s.stop = func() {
		select {
		case <-s.done:
		default:
			_ = windows.TerminateProcess(pi.Process, 1)
		}
	}
	return s
}

// pipeWriter sends key bytes into the pseudoconsole input pipe.
type pipeWriter struct{ h windows.Handle }

func (p *pipeWriter) Write(b []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *pipeWriter) Close() error {
	return windows.CloseHandle(p.h)
}

// pseudoConsolePtr is the pseudoconsole handle as the attribute value.
// UpdateProcThreadAttribute takes that handle, not a pointer to the variable.
func pseudoConsolePtr(h windows.Handle) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&h))
}

func windowsEnv(env []string) ([]uint16, error) {
	sorted := append([]string(nil), env...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToUpper(envKey(sorted[i])) < strings.ToUpper(envKey(sorted[j]))
	})
	var out []uint16
	for _, e := range sorted {
		p, err := windows.UTF16FromString(e)
		if err != nil {
			return nil, err
		}
		out = append(out, p...)
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	out = append(out, 0)
	return out, nil
}

func envKey(kv string) string {
	key, _, _ := strings.Cut(kv, "=")
	return key
}
