package main

import (
	"slices"
	"testing"
	"testing/fstest"
)

func TestBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		remove []string
		files  map[string]string
		want   []string
	}{
		{
			name: "a clean module holds",
		},

		{
			name:  "the domain imports os",
			files: map[string]string{"internal/domain/scan/scan.go": "package scan\n\nimport \"os\"\n\nvar _ = os.Getenv\n"},
			want:  []string{`internal/domain/scan/scan.go:3:8: domain-imports: imports "os": os and everything beneath it is refused`},
		},
		{
			name:  "under another name",
			files: map[string]string{"internal/domain/scan/scan.go": "package scan\n\nimport disk \"os\"\n\nvar _ = disk.Getenv\n"},
			want:  []string{`internal/domain/scan/scan.go:3:13: domain-imports: imports "os": os and everything beneath it is refused`},
		},
		{
			name:  "for its side effects only",
			files: map[string]string{"internal/domain/scan/scan.go": "package scan\n\nimport _ \"net\"\n"},
			want:  []string{`internal/domain/scan/scan.go:3:10: domain-imports: imports "net": net and everything beneath it is refused`},
		},
		{
			name:  "a package beneath a refused one",
			files: map[string]string{"internal/domain/clean/clean.go": "package clean\n\nimport (\n\t\"os/exec\"\n\t\"net/http\"\n)\n"},
			want: []string{
				`internal/domain/clean/clean.go:4:2: domain-imports: imports "os/exec": os and everything beneath it is refused`,
				`internal/domain/clean/clean.go:5:2: domain-imports: imports "net/http": net and everything beneath it is refused`,
			},
		},
		{
			name:  "syscall and io/ioutil do the same IO under other names",
			files: map[string]string{"internal/domain/scan/scan.go": "package scan\n\nimport (\n\t\"syscall\"\n\t\"io/ioutil\"\n\t\"syscall/js\"\n)\n"},
			want: []string{
				`internal/domain/scan/scan.go:4:2: domain-imports: imports "syscall": syscall and everything beneath it is refused`,
				`internal/domain/scan/scan.go:5:2: domain-imports: imports "io/ioutil": io/ioutil and everything beneath it is refused`,
				`internal/domain/scan/scan.go:6:2: domain-imports: imports "syscall/js": syscall and everything beneath it is refused`,
			},
		},
		{
			name:  "io itself is not io/ioutil",
			files: map[string]string{"internal/domain/plan/plan.go": "package plan\n\nimport (\n\t\"io\"\n\t\"io/fs\"\n)\n"},
		},
		{
			name:  "in a file for another operating system",
			files: map[string]string{"internal/domain/scan/scan_windows.go": "package scan\n\nimport \"os\"\n"},
			want:  []string{`internal/domain/scan/scan_windows.go:3:8: domain-imports: imports "os": os and everything beneath it is refused`},
		},
		{
			name:  "in a file behind a build tag",
			files: map[string]string{"internal/domain/scan/scan.go": "//go:build never\n\npackage scan\n\nimport \"os\"\n"},
			want:  []string{`internal/domain/scan/scan.go:5:8: domain-imports: imports "os": os and everything beneath it is refused`},
		},
		{
			name:  "the domain imports a dependency",
			files: map[string]string{"internal/domain/plan/plan.go": "package plan\n\nimport \"golang.org/x/text/width\"\n"},
			want:  []string{`internal/domain/plan/plan.go:3:8: domain-imports: imports "golang.org/x/text/width": neither the standard library nor internal/domain`},
		},
		{
			name:  "the domain imports an adapter",
			files: map[string]string{"internal/domain/clean/clean.go": "package clean\n\nimport \"example.com/sweep/internal/system/trash\"\n"},
			want:  []string{`internal/domain/clean/clean.go:3:8: domain-imports: imports "example.com/sweep/internal/system/trash": neither the standard library nor internal/domain`},
		},
		{
			name:  "the domain uses cgo",
			files: map[string]string{"internal/domain/scan/scan.go": "package scan\n\nimport \"C\"\n"},
			want:  []string{`internal/domain/scan/scan.go:3:8: domain-imports: imports "C": neither the standard library nor internal/domain`},
		},
		{
			name: "the domain imports itself and the standard library",
			files: map[string]string{
				"internal/domain/plan/plan.go": "package plan\n\nimport (\n\t\"io/fs\"\n\t\"strings\"\n\n\t\"example.com/sweep/internal/domain\"\n\t\"example.com/sweep/internal/domain/scan\"\n)\n",
			},
		},
		{
			name:  "a domain test imports the fakes",
			files: map[string]string{"internal/domain/scan/scan_test.go": "package scan_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/sweep/internal/sweeptest\"\n)\n"},
		},
		{
			name:  "a domain file that is not a test imports the fakes",
			files: map[string]string{"internal/domain/scan/scan.go": "package scan\n\nimport \"example.com/sweep/internal/sweeptest\"\n"},
			want:  []string{`internal/domain/scan/scan.go:3:8: domain-imports: imports "example.com/sweep/internal/sweeptest": neither the standard library nor internal/domain`},
		},
		{
			name:  "a domain test imports os",
			files: map[string]string{"internal/domain/scan/scan_test.go": "package scan\n\nimport \"os\"\n"},
			want:  []string{`internal/domain/scan/scan_test.go:3:8: domain-imports: imports "os": os and everything beneath it is refused`},
		},
		{
			name:  "os outside the domain is nobody's business",
			files: map[string]string{"internal/system/files/files.go": "package files\n\nimport \"os\"\n\nvar _ = os.Lstat\n"},
		},
		{
			name:   "the domain has moved",
			remove: []string{"internal/domain/doc.go"},
			files:  map[string]string{"internal/core/doc.go": "package core\n"},
			want:   []string{`.: domain-imports: governs no Go file: the directory it names has moved or is empty, so move the boundary with it`},
		},

		{
			name:  "an adapter that is not the trash calls os.RemoveAll",
			files: map[string]string{"internal/system/files/files.go": "package files\n\nimport \"os\"\n\nfunc Purge(p string) error { return os.RemoveAll(p) }\n"},
			want:  []string{`internal/system/files/files.go:5:40: only-trash-removes: selects RemoveAll outside internal/system/trash`},
		},
		{
			name:  "under another name",
			files: map[string]string{"cmd/sweep/main.go": "package main\n\nimport disk \"os\"\n\nfunc main() { disk.Remove(\"x\") }\n"},
			want:  []string{`cmd/sweep/main.go:5:20: only-trash-removes: selects Remove outside internal/system/trash`},
		},
		{
			name:  "as a value, never called here",
			files: map[string]string{"internal/report/report.go": "package report\n\nimport \"os\"\n\nvar undo = os.Remove\n"},
			want:  []string{`internal/report/report.go:5:15: only-trash-removes: selects Remove outside internal/system/trash`},
		},
		{
			name:  "through a dot import",
			files: map[string]string{"internal/tui/tui.go": "package tui\n\nimport . \"os\"\n\nfunc clear(p string) { RemoveAll(p) }\n"},
			want:  []string{`internal/tui/tui.go:3:10: only-trash-removes: dot-imports "os", which would let Remove, RemoveAll, Unlink, Unlinkat or Rmdir appear without a selector`},
		},
		{
			name:  "on an os.Root",
			files: map[string]string{"internal/system/files/files.go": "package files\n\nimport \"os\"\n\nfunc purge(root *os.Root) error { return root.RemoveAll(\".\") }\n"},
			want:  []string{`internal/system/files/files.go:5:47: only-trash-removes: selects RemoveAll outside internal/system/trash`},
		},
		{
			name:  "on an interface, in a file that never imports os",
			files: map[string]string{"internal/domain/clean/clean.go": "package clean\n\ntype disk interface{ Remove(string) error }\n\nfunc run(d disk) error { return d.Remove(\"x\") }\n"},
			want:  []string{`internal/domain/clean/clean.go:5:35: only-trash-removes: selects Remove outside internal/system/trash`},
		},
		{
			name:  "in a file for another operating system",
			files: map[string]string{"internal/system/files/files_windows.go": "package files\n\nimport \"os\"\n\nfunc purge(p string) { os.Remove(p) }\n"},
			want:  []string{`internal/system/files/files_windows.go:5:27: only-trash-removes: selects Remove outside internal/system/trash`},
		},
		{
			name: "through syscall and golang.org/x/sys/unix",
			files: map[string]string{
				"internal/system/files/files_unix.go":  "package files\n\nimport \"syscall\"\n\nfunc purge(p string) { syscall.Unlink(p); syscall.Rmdir(p) }\n",
				"internal/system/files/files_linux.go": "package files\n\nimport \"golang.org/x/sys/unix\"\n\nfunc purgeAt(fd int, p string) { unix.Unlinkat(fd, p, 0) }\n",
			},
			want: []string{
				`internal/system/files/files_linux.go:5:39: only-trash-removes: selects Unlinkat outside internal/system/trash`,
				`internal/system/files/files_unix.go:5:32: only-trash-removes: selects Unlink outside internal/system/trash`,
				`internal/system/files/files_unix.go:5:51: only-trash-removes: selects Rmdir outside internal/system/trash`,
			},
		},
		{
			name:  "in a test",
			files: map[string]string{"internal/system/files/files_test.go": "package files\n\nimport \"os\"\n\nfunc tidy(p string) { os.RemoveAll(p) }\n"},
			want:  []string{`internal/system/files/files_test.go:5:26: only-trash-removes: selects RemoveAll outside internal/system/trash`},
		},
		{
			name:  "in a directory the go command ignores",
			files: map[string]string{"tools/fixture/testdata/wipe.go": "package main\n\nimport \"os\"\n\nfunc main() { os.RemoveAll(\"/\") }\n"},
			want:  []string{`tools/fixture/testdata/wipe.go:5:18: only-trash-removes: selects RemoveAll outside internal/system/trash`},
		},
		{
			name: "the trash adapter removes, on every operating system",
			files: map[string]string{
				"internal/system/trash/trash_linux.go":   "package trash\n\nimport \"os\"\n\nfunc Delete(p string) error { return os.RemoveAll(p) }\n",
				"internal/system/trash/trash_windows.go": "package trash\n\nimport \"os\"\n\nfunc Delete(p string) error { return os.RemoveAll(p) }\n",
			},
		},
		{
			name:  "the trash adapter has moved",
			files: map[string]string{"internal/system/bin/bin.go": "package bin\n\nimport \"os\"\n\nfunc Delete(p string) error { return os.RemoveAll(p) }\n"},
			want:  []string{`internal/system/bin/bin.go:5:41: only-trash-removes: selects RemoveAll outside internal/system/trash`},
		},
		{
			name:  "a directory that only starts like the trash adapter",
			files: map[string]string{"internal/system/trashcan/can.go": "package trashcan\n\nimport \"os\"\n\nvar _ = os.Remove\n"},
			want:  []string{`internal/system/trashcan/can.go:5:12: only-trash-removes: selects Remove outside internal/system/trash`},
		},

		{
			name:  "a file that does not parse",
			files: map[string]string{"internal/report/report.go": "package report\n\nfunc {\n"},
			want:  []string{`internal/report/report.go:3:6: readable: does not parse (expected 'IDENT', found '{'), so no boundary can vouch for it`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"go.mod":                 {Data: []byte("module example.com/sweep\n\ngo 1.27\n")},
				"cmd/sweep/main.go":      {Data: []byte("package main\n\nfunc main() {}\n")},
				"internal/domain/doc.go": {Data: []byte("package domain\n")},
			}
			for _, name := range tt.remove {
				delete(fsys, name)
			}
			for name, content := range tt.files {
				fsys[name] = &fstest.MapFile{Data: []byte(content)}
			}

			found, err := check(fsys, boundaries)
			if err != nil {
				t.Fatal(err)
			}

			var got []string
			for _, v := range found {
				got = append(got, v.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("violations:\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestOutsideAModuleNothingIsJudged(t *testing.T) {
	_, err := check(fstest.MapFS{"main.go": {Data: []byte("package main\n")}}, boundaries)
	if err == nil {
		t.Error("check succeeded without a go.mod")
	}
}
