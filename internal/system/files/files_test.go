package files

import (
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestDiskPassesTheFilesContract(t *testing.T) {
	sweeptest.FilesContract(t, func(t *testing.T, tree sweeptest.Tree) (scan.Files, string) {
		return Disk{}, sweeptest.OnDisk(t, tree)
	})
}
