package sweeptest_test

import (
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestTheFakeGitPassesTheContract(t *testing.T) {
	sweeptest.GitContract(t, func(t *testing.T, work sweeptest.Work) (safety.Git, string) {
		top := t.TempDir()
		return sweeptest.NewGit(top, work), top
	})
}
