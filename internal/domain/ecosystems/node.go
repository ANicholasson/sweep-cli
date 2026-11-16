package ecosystems

import "github.com/ANicholasson/sweep-cli/internal/domain"

func init() {
	add(domain.Ecosystem{
		Name: "Node",
		Rules: []domain.Rule{
			{
				Kind:   domain.Dependencies,
				Reason: "npm install puts it back from package.json and the lockfile.",
				Where:  domain.InProject{Manifest: "package.json", Candidate: "node_modules"},
			},
			{
				Kind:   domain.Cache,
				Reason: "npm downloads again whatever a later install needs.",
				Where: domain.InSharedCache{
					Linux:   ".npm/_cacache",
					MacOS:   ".npm/_cacache",
					Windows: "AppData/Local/npm-cache",
				},
			},
		},
	})
}
