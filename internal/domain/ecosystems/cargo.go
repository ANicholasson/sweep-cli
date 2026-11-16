package ecosystems

import "github.com/ANicholasson/sweep-cli/internal/domain"

func init() {
	add(domain.Ecosystem{
		Name: "Cargo",
		Rules: []domain.Rule{
			{
				Kind:   domain.BuildOutput,
				Reason: "cargo build writes it again from the sources beside Cargo.toml.",
				Where:  domain.InProject{Manifest: "Cargo.toml", Candidate: "target"},
			},
			{
				Kind:   domain.Cache,
				Reason: "cargo downloads again the crates a later build needs.",
				Where: domain.InSharedCache{
					Linux:   ".cargo/registry",
					MacOS:   ".cargo/registry",
					Windows: ".cargo/registry",
				},
			},
		},
	})
}
