package domain

import "testing"

func node() Ecosystem {
	return Ecosystem{Name: "Node", Rules: []Rule{
		{
			Kind:   Dependencies,
			Reason: "npm install puts it back.",
			Where:  InProject{Manifest: "package.json", Candidate: "node_modules"},
		},
		{
			Kind:   Cache,
			Reason: "npm downloads it again.",
			Where:  InSharedCache{Linux: ".npm/_cacache", MacOS: ".npm/_cacache", Windows: "AppData/Local/npm-cache/_cacache"},
		},
	}}
}

func withRule(change func(*Rule)) []Ecosystem {
	e := node()
	change(&e.Rules[0])
	return []Ecosystem{e}
}

func TestARuleNamesOnlyAKindFromTheList(t *testing.T) {
	for _, k := range kinds {
		if err := Validate(withRule(func(r *Rule) { r.Kind = k })); err != nil {
			t.Errorf("Kind %q is refused: %v", k, err)
		}
	}
	for _, k := range []Kind{{}, {"Artifacts"}, {"cache"}} {
		if Validate(withRule(func(r *Rule) { r.Kind = k })) == nil {
			t.Errorf("Kind %q is accepted", k)
		}
	}
}

func TestEveryRuleCarriesAReason(t *testing.T) {
	for _, reason := range []Reason{"", " \t\n"} {
		if Validate(withRule(func(r *Rule) { r.Reason = reason })) == nil {
			t.Errorf("Reason %q is accepted", reason)
		}
	}
}

func TestValidateAcceptsTheModel(t *testing.T) {
	if err := Validate([]Ecosystem{node()}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRefusesWhatBreaksTheModel(t *testing.T) {
	shared := func(c InSharedCache) []Ecosystem {
		return withRule(func(r *Rule) { r.Where = c })
	}
	cargo := Ecosystem{Name: "Cargo", Rules: []Rule{{
		Kind:   BuildOutput,
		Reason: "cargo build writes it again.",
		Where:  InProject{Manifest: "Cargo.toml", Candidate: "target"},
	}}}
	renamed := func(name string) Ecosystem { e := cargo; e.Name = name; return e }
	sharing := func(c InSharedCache) Ecosystem {
		e := cargo
		e.Rules = []Rule{{Kind: Cache, Reason: "cargo downloads it again.", Where: c}}
		return e
	}
	cases := map[string][]Ecosystem{
		"an Ecosystem with no Name":       {renamed(" ")},
		"an Ecosystem with no Rules":      {{Name: "Node"}},
		"two Ecosystems with one Name":    {node(), renamed("node")},
		"a Rule with no Where":            withRule(func(r *Rule) { r.Where = nil }),
		"a Manifest that is a path":       withRule(func(r *Rule) { r.Where = InProject{Manifest: "a/package.json", Candidate: "node_modules"} }),
		"a Candidate that is a path":      withRule(func(r *Rule) { r.Where = InProject{Manifest: "package.json", Candidate: `a\node_modules`} }),
		"a Project Rule with no Manifest": withRule(func(r *Rule) { r.Where = InProject{Candidate: "node_modules"} }),
		"a Candidate named ..":            withRule(func(r *Rule) { r.Where = InProject{Manifest: "package.json", Candidate: ".."} }),
		"two Rules finding one Candidate": {node(), {Name: "Yarn", Rules: node().Rules[:1]}},
		"a Shared cache on no system":     shared(InSharedCache{}),
		"an absolute location":            shared(InSharedCache{Linux: "/var/cache/npm"}),
		"a location above home":           shared(InSharedCache{Linux: "../other/.npm"}),
		"a location that is home":         shared(InSharedCache{Linux: "."}),
		"a location that is not clean":    shared(InSharedCache{Linux: ".npm/../.npm"}),
		"a location under a tilde":        shared(InSharedCache{MacOS: "~/.npm"}),
		"a location with a drive":         shared(InSharedCache{Windows: "C:/Users/a/npm-cache"}),
		"a location with backslashes":     shared(InSharedCache{Windows: `AppData\Local\npm-cache`}),
		"a location with a variable":      shared(InSharedCache{Windows: "%LocalAppData%/npm-cache"}),
		"two Shared caches at one place":  {node(), sharing(InSharedCache{Linux: ".npm/_cacache"})},
		"a Shared cache inside another":   {node(), sharing(InSharedCache{MacOS: ".npm"})},
	}
	for name, ecosystems := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(ecosystems)
			if err == nil {
				t.Fatal("accepted")
			}
			t.Log(err)
		})
	}
}
