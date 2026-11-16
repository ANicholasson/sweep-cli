package files

import (
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestAJunctionIsALink(t *testing.T) {
	top := sweeptest.OnDisk(t, sweeptest.Tree{"dir/f.txt": sweeptest.File{Bytes: 1}})
	dir, junction := filepath.Join(top, "dir"), filepath.Join(top, "to-dir")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", junction, dir).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v\n%s", err, out)
	}

	entries, err := Disk{}.List(top)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(entries, func(e scan.Entry) bool { return e.Name == "to-dir" })
	if i < 0 || entries[i].Is != scan.Link {
		t.Fatalf("List = %+v, want to-dir as a Link", entries)
	}
	of, err := Disk{}.Space(junction)
	if err != nil {
		t.Fatal(err)
	}
	to, err := Disk{}.Space(dir)
	if err != nil {
		t.Fatal(err)
	}
	if of.File == to.File {
		t.Errorf("the junction has the FileID of the directory it leads to, so Space followed it")
	}
}
