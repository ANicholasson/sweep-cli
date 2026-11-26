//go:build !linux

package trash

// Open refuses. This operating system has no trash adapter yet, so a Clean
// removes nothing.
func Open() (Mover, error) {
	return Mover{}, ErrUnavailable
}

// In refuses. This operating system has no trash adapter yet.
func In(string) Mover { return Mover{} }

// Move refuses. The path is not removed.
func (Mover) Move(string) error { return ErrUnavailable }

// Restore refuses. There is no trash to restore from.
func (Mover) Restore(string) error { return ErrUnavailable }
