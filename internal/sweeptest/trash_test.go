package sweeptest_test

import (
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestTheFakeTrashPassesTheContract(t *testing.T) {
	sweeptest.TrashContract(t, func(_ *testing.T, tree sweeptest.Tree) (clean.Trash, sweeptest.TrashView, string) {
		files := sweeptest.NewFiles(tree)
		return sweeptest.NewTrash(files), files.View(), sweeptest.Top
	})
}
