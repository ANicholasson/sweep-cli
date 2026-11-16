// Package ecosystems holds one file per Ecosystem. Each of those files holds
// only an init that adds its Ecosystem, so adding an Ecosystem adds a file
// and edits none.
package ecosystems

import (
	"slices"

	"github.com/ANicholasson/sweep-cli/internal/domain"
)

var all []domain.Ecosystem

func add(e domain.Ecosystem) { all = append(all, e) }

// All lists every Ecosystem sweep knows.
func All() []domain.Ecosystem { return slices.Clone(all) }
