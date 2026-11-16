package ecosystems_test

import (
	"embed"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
)

//go:embed *.go
var sources embed.FS

func TestEveryEcosystemObeysTheModel(t *testing.T) {
	all := ecosystems.All()
	if len(all) == 0 {
		t.Fatal("no Ecosystem is added")
	}
	if err := domain.Validate(all); err != nil {
		t.Fatal(err)
	}
}

func TestEachEcosystemFileDeclaresOnlyItsEcosystem(t *testing.T) {
	entries, err := sources.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == "ecosystems.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		src, err := sources.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !onlyAdds(f) {
			t.Errorf("%s declares more than an init that calls add once, so a name it declares could collide with the same name in another Ecosystem file", name)
		}
	}
	if files != len(ecosystems.All()) {
		t.Errorf("%d Ecosystem files add %d Ecosystems", files, len(ecosystems.All()))
	}
}

func onlyAdds(f *ast.File) bool {
	var funcs []*ast.FuncDecl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.IMPORT {
				return false
			}
		case *ast.FuncDecl:
			funcs = append(funcs, d)
		}
	}
	if len(funcs) != 1 || funcs[0].Name.Name != "init" || funcs[0].Recv != nil || len(funcs[0].Body.List) != 1 {
		return false
	}
	stmt, ok := funcs[0].Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	fun, ok := call.Fun.(*ast.Ident)
	return ok && fun.Name == "add"
}
