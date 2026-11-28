//go:build windows

package main_test

import (
	"os"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startTerminal(t *testing.T, bin, dir string, env []string) *termSession {
	t.Helper()
	sa := &windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, sa, 0); err != nil {
		t.Fatal(err)
	}
	if err := windows.CreatePipe(&outRead, &outWrite, sa, 0); err != nil {
		t.Fatal(err)
	}
	var pc windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 200, Y: 80}, inRead, outWrite, 0, &pc); err != nil {
		t.Fatal(err)
	}
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)

	attr, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := attr.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&pc), unsafe.Sizeof(pc)); err != nil {
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
	runtime.KeepAlive(attr)
	runtime.KeepAlive(block)
	runtime.KeepAlive(pc)
	attr.Delete()
	if err != nil {
		t.Fatal(err)
	}
	_ = windows.CloseHandle(pi.Thread)

	in := os.NewFile(uintptr(inWrite), "in")
	out := os.NewFile(uintptr(outRead), "out")
	s := &termSession{in: in, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := out.Read(buf)
			if n > 0 {
				s.appendOut(append([]byte(nil), buf[:n]...))
			}
			if readErr != nil {
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
		_ = out.Close()
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

func windowsEnv(env []string) ([]uint16, error) {
	var out []uint16
	for _, e := range env {
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
