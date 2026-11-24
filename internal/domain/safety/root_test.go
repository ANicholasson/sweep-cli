package safety

import (
	"strings"
	"testing"
)

func TestBeneathFollowsTheRootRule(t *testing.T) {
	tests := []struct {
		name string
		root string
		path string
		link bool
		want bool
	}{
		{name: "the root itself", root: "/projects", path: "/projects", want: true},
		{name: "a trailing separator", root: "/projects", path: "/projects/", want: true},
		{name: "a child", root: "/projects", path: "/projects/app/node_modules", want: true},
		{name: "a sibling", root: "/projects", path: "/other", want: false},
		{name: "dotdot that leaves", root: "/projects", path: "/projects/../etc", want: false},
		{name: "dotdot that stays inside", root: "/projects", path: "/projects/foo/../app", want: true},
		{name: "a drive letter's case", root: `C:\Projects`, path: `c:\Projects\app`, want: true},
		{name: "a segment's case", root: `C:\Projects`, path: `C:\projects\app`, want: false},
		{name: "posix case", root: "/Projects", path: "/projects", want: false},
		{name: "mixed separators", root: `C:\Projects`, path: `C:/Projects\app`, want: true},
		{name: "a UNC child", root: `\\server\share\projects`, path: `\\server\SHARE\projects\app`, want: true},
		{name: "another UNC host", root: `\\server\share\projects`, path: `\\other\share\projects\app`, want: false},
		{name: "a UNC sibling", root: `\\server\share\projects`, path: `\\server\share\other`, want: false},
		{name: "dotdot on a drive", root: `C:\Projects`, path: `C:\Projects\..\Windows`, want: false},
		{name: "dotdot stays on the volume", root: `C:\Projects`, path: `C:\..\Windows`, want: false},
		{name: "the extended prefix", root: `C:\Projects`, path: `\\?\C:\Projects\app`, want: true},
		{name: "the extended UNC prefix", root: `\\server\share\a`, path: `\\?\UNC\server\share\a\b`, want: true},
		{name: "a drive-relative path", root: `C:\Projects`, path: `C:Projects\app`, want: false},
		{name: "a relative path", root: "/projects", path: "projects/app", want: false},
		{name: "an empty path", root: "/projects", path: "", want: false},
		{name: "a NUL", root: "/projects", path: "/projects/a\x00b", want: false},
		{name: "a Link on the way", root: "/projects", path: "/projects/app", link: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Beneath(tt.root, tt.path, tt.link); got != tt.want {
				t.Errorf("Beneath(%q, %q, %v) = %v, want %v", tt.root, tt.path, tt.link, got, tt.want)
			}
		})
	}
}

func TestBeneathHandlesAVeryLongPath(t *testing.T) {
	parts := make([]string, 4000)
	for i := range parts {
		parts[i] = "node_modules"
	}
	path := "/projects/" + strings.Join(parts, "/")
	if !Beneath("/projects", path, false) {
		t.Fatal("a path of 4000 segments is not beneath its root")
	}
}

func FuzzBeneath(f *testing.F) {
	f.Add(`/projects`, `/projects/app/node_modules`, false)
	f.Add(`/projects`, `/projects/../etc`, false)
	f.Add(`/projects`, `/projects/foo/../app`, false)
	f.Add(`C:\Projects`, `c:\Projects\app`, false)
	f.Add(`C:\Projects`, `C:\projects\app`, false)
	f.Add(`C:\Projects`, `C:/Projects\..\Windows`, false)
	f.Add(`\\server\share\projects`, `\\server\SHARE\projects\app`, false)
	f.Add(`\\server\share\projects`, `\\?\UNC\server\share\projects\..\other`, false)
	f.Add(`/projects`, `/projects/`+strings.Repeat("a/", 2000)+"b", false)
	f.Add(`C:\Projects`, `C:\Projects\app`, true)
	f.Add(`/`, `/`, false)
	f.Add(`/`, `/child`, false)
	f.Fuzz(func(t *testing.T, root, path string, link bool) {
		first := Beneath(root, path, link)
		if second := Beneath(root, path, link); first != second {
			t.Fatalf("Beneath(%q, %q, %v) is %v then %v", root, path, link, first, second)
		}
		if link && first {
			t.Fatalf("Beneath(%q, %q, true) = true", root, path)
		}
		if link || !first {
			return
		}
		sep := "/"
		if strings.Contains(path, `\`) && !strings.Contains(path, "/") {
			sep = `\`
		}
		child := path
		if !strings.HasSuffix(child, "/") && !strings.HasSuffix(child, `\`) {
			child += sep
		}
		child += "child"
		if !Beneath(root, child, false) {
			t.Fatalf("Beneath(%q, %q) and not %q", root, path, child)
		}
	})
}
