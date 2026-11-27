package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
)

var boundaries = []boundary{
	importsOnly{
		name:           "domain-imports",
		why:            "the domain does no IO",
		dir:            "internal/domain",
		testsMayImport: []string{"internal/sweeptest"},
		never:          []string{"os", "net", "syscall", "io/ioutil"},
	},
	onlyIn{
		name:  "only-trash-removes",
		why:   "the trash adapter is the only package that trashes or deletes anything",
		dir:   "internal/system/trash",
		names: []string{"Remove", "RemoveAll", "Rename", "Renameat", "Unlink", "Unlinkat", "Rmdir"},
	},
	importedOnlyBy{
		name: "only-cmd-imports-adapters",
		why:  "cmd/sweep is the composition root, the one package that wires an adapter to the domain",
		dir:  "internal/system",
		by:   "cmd/sweep",
	},
}

type source struct {
	path    string
	test    bool
	imports []imported
	selects []selection
}

type imported struct {
	pos  string
	path string
	dot  bool
}

type selection struct {
	pos  string
	name string
}

type violation struct {
	pos      string
	boundary string
	what     string
}

func (v violation) String() string {
	return v.pos + ": " + v.boundary + ": " + v.what
}

type boundary interface {
	rule() (name, sentence string)
	governs(src source) bool
	broken(module string, src source) []violation
}

type guard interface {
	guarded() string
}

type importsOnly struct {
	name, why      string
	dir            string
	testsMayImport []string
	never          []string
}

type onlyIn struct {
	name, why string
	dir       string
	names     []string
}

type importedOnlyBy struct {
	name, why string
	dir       string
	by        string
}

func (b importsOnly) rule() (string, string) {
	tests := ""
	if len(b.testsMayImport) > 0 {
		tests = " Test files may also import from " + orList(b.testsMayImport) + "."
	}
	return b.name, fmt.Sprintf("Go files beneath %s import only the standard library and %s, and never %s, nor a package beneath one of them, because %s.%s",
		b.dir, b.dir, orList(b.never), b.why, tests)
}

func (b importsOnly) governs(src source) bool { return beneath(src.path, b.dir) }

func (b importsOnly) broken(module string, src source) []violation {
	var out []violation
	report := func(imp imported, what string) {
		out = append(out, violation{imp.pos, b.name, "imports " + strconv.Quote(imp.path) + ": " + what})
	}
	for _, imp := range src.imports {
		dir, own := strings.CutPrefix(imp.path, module+"/")
		refused, isRefused := firstBeneath(imp.path, b.never)
		switch {
		case isRefused:
			report(imp, refused+" and everything beneath it is refused")
		case own && beneath(dir, b.dir):
		case own && src.test && beneathAny(dir, b.testsMayImport):
		case !own && standard(imp.path):
		default:
			report(imp, "neither the standard library nor "+b.dir)
		}
	}
	return out
}

func (b onlyIn) rule() (string, string) {
	return b.name, fmt.Sprintf("Outside %s no Go file selects %s from anything, and none has a dot import, because %s. No type information is used, so the names are refused on every receiver.",
		b.dir, orList(b.names), b.why)
}

func (b onlyIn) governs(src source) bool { return !beneath(src.path, b.dir) }

func (b onlyIn) broken(_ string, src source) []violation {
	var out []violation
	for _, imp := range src.imports {
		if imp.dot {
			out = append(out, violation{imp.pos, b.name, "dot-imports " + strconv.Quote(imp.path) + ", which would let " + orList(b.names) + " appear without a selector"})
		}
	}
	for _, sel := range src.selects {
		for _, name := range b.names {
			if sel.name == name {
				out = append(out, violation{sel.pos, b.name, "selects " + sel.name + " outside " + b.dir})
			}
		}
	}
	return out
}

func (b importedOnlyBy) rule() (string, string) {
	return b.name, fmt.Sprintf("Outside %s no Go file imports a package beneath %s, because %s. So the report and the interactive screen never import an adapter, and no package but %s imports both the domain and an adapter. A test of an adapter is written in the adapter's own package, so it imports none.",
		b.by, b.dir, b.why, b.by)
}

func (b importedOnlyBy) governs(src source) bool { return !beneath(src.path, b.by) }

func (b importedOnlyBy) broken(module string, src source) []violation {
	var out []violation
	for _, imp := range src.imports {
		if dir, own := strings.CutPrefix(imp.path, module+"/"); own && beneath(dir, b.dir) {
			out = append(out, violation{imp.pos, b.name, "imports " + strconv.Quote(imp.path) + ": only " + b.by + " imports an adapter"})
		}
	}
	return out
}

func (b importedOnlyBy) guarded() string { return b.dir }

func check(fsys fs.FS, boundaries []boundary) ([]violation, error) {
	module, err := modulePath(fsys)
	if err != nil {
		return nil, err
	}
	var out []violation
	governed := make([]int, len(boundaries))
	held := make([]int, len(boundaries))
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, unreadable, err := read(fsys, p)
		if err != nil {
			return err
		}
		out = append(out, unreadable...)
		for i, b := range boundaries {
			if b.governs(src) {
				governed[i]++
				out = append(out, b.broken(module, src)...)
			}
			if g, ok := b.(guard); ok && beneath(src.path, g.guarded()) {
				held[i]++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, b := range boundaries {
		name, _ := b.rule()
		if governed[i] == 0 {
			out = append(out, violation{".", name, "governs no Go file: the directory it names has moved or is empty, so move the boundary with it"})
		}
		if g, ok := b.(guard); ok && held[i] == 0 {
			out = append(out, violation{".", name, "guards " + g.guarded() + ", which holds no Go file: it has moved or is empty, so move the boundary with it"})
		}
	}
	return out, nil
}

func read(fsys fs.FS, p string) (source, []violation, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return source{}, nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, p, data, parser.SkipObjectResolution)
	var unreadable []violation
	if err != nil {
		pos, msg := p, err.Error()
		var list scanner.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			pos, msg = list[0].Pos.String(), list[0].Msg
		}
		unreadable = append(unreadable, violation{pos, "readable", "does not parse (" + msg + "), so no boundary can vouch for it"})
	}
	src := source{path: p, test: strings.HasSuffix(p, "_test.go")}
	if file == nil {
		return src, unreadable, nil
	}
	at := func(pos token.Pos) string { return fset.Position(pos).String() }
	for _, spec := range file.Imports {
		ip, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue // the parser has reported it
		}
		src.imports = append(src.imports, imported{
			pos:  at(spec.Path.Pos()),
			path: ip,
			dot:  spec.Name != nil && spec.Name.Name == ".",
		})
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			src.selects = append(src.selects, selection{pos: at(sel.Sel.Pos()), name: sel.Sel.Name})
		}
		return true
	})
	return src, unreadable, nil
}

func modulePath(fsys fs.FS) (string, error) {
	data, err := fs.ReadFile(fsys, "go.mod")
	if err != nil {
		return "", fmt.Errorf("not at a module root: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", errors.New("go.mod has no module line")
}

// standard uses the go command's own test for a standard library path: its
// first element has no dot. "C" passes that test and is not one.
func standard(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".") && importPath != "C"
}

func beneath(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

func beneathAny(p string, dirs []string) bool {
	_, ok := firstBeneath(p, dirs)
	return ok
}

func firstBeneath(p string, dirs []string) (string, bool) {
	for _, dir := range dirs {
		if beneath(p, dir) {
			return dir, true
		}
	}
	return "", false
}

func orList(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " or " + words[len(words)-1]
}
