package sweeptest_test

import (
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestTheFakeFilesPassTheContract(t *testing.T) {
	sweeptest.FilesContract(t, func(_ *testing.T, tree sweeptest.Tree) (scan.Files, string) {
		return sweeptest.NewFiles(tree), sweeptest.Top
	})
}
