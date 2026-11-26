package ecosystems

import "github.com/ANicholasson/sweep-cli/internal/domain"

func init() {
	add(domain.Ecosystem{
		Name: "Cargo",
		Rules: []domain.Rule{
			{
				Kind:   domain.BuildOutput,
				Reason: "cargo build writes it again from Cargo.toml.",
				Where:  domain.InProject{Manifest: "Cargo.toml", Candidate: "target"},
			},
		},
	})
}
