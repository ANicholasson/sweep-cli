package domain

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"testing"
)

//go:embed kind.go
var kindSource string

const domainPath = "github.com/ANicholasson/sweep-cli/internal/domain"

func TestKindIsTheGlossaryList(t *testing.T) {
	var names []string
	for _, k := range kinds {
		names = append(names, k.String())
	}
	want := []string{"Dependencies", "Build output", "Cache", "Models", "Worktree"}
	if !slices.Equal(names, want) {
		t.Fatalf("Kinds are %q, want %q", names, want)
	}
}

func TestARuleCannotNameAKindOutsideTheList(t *testing.T) {
	fset := token.NewFileSet()
	kindFile, err := parser.ParseFile(fset, "kind.go", kindSource, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := (&types.Config{}).Check(domainPath, fset, []*ast.File{kindFile}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ecosystemFile := func(kind string) error {
		src := "package ecosystems\n\nimport \"" + domainPath + "\"\n\nvar _ domain.Kind = " + kind + "\n"
		f, err := parser.ParseFile(fset, "ecosystem.go", src, 0)
		if err != nil {
			t.Fatalf("%s does not parse: %v", kind, err)
		}
		conf := types.Config{Importer: only{pkg}}
		_, err = conf.Check("ecosystems", fset, []*ast.File{f}, nil)
		return err
	}

	if err := ecosystemFile("domain.Cache"); err != nil {
		t.Fatalf("a Kind from the list does not build: %v", err)
	}
	for _, kind := range []string{
		`"Logs"`,
		`domain.Kind{name: "Logs"}`,
		`domain.Kind{"Logs"}`,
		`domain.Kind(struct{ name string }{"Logs"})`,
	} {
		if err := ecosystemFile(kind); err == nil {
			t.Errorf("%s builds in an Ecosystem file, so a Rule could name a Kind outside the list", kind)
		}
	}
}

type only struct{ pkg *types.Package }

func (o only) Import(path string) (*types.Package, error) {
	if path != o.pkg.Path() {
		return nil, fmt.Errorf("only %s can be imported", o.pkg.Path())
	}
	return o.pkg, nil
}
