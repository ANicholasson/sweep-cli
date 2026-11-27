package trash

import (
	"errors"

	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
)

// A Mover is the Trash port on every operating system. On the ones without
// an adapter, Move and Restore refuse and remove nothing.
var _ clean.Trash = Mover{}

// ErrUnavailable is returned when this operating system cannot move a
// Candidate to the trash. Nothing was removed.
var ErrUnavailable = errors.New("this operating system has no trash yet, so nothing was removed")
