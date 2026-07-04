package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestReposResolveClientFromContext fails any repo method that reads the
// injected client field directly instead of via ec(ctx): the raw app pool has
// no app.tenant_id GUC, so RLS silently fail-closes.
func TestReposResolveClientFromContext(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Body == nil || fn.Name.Name == "ec" {
					continue
				}
				recv := receiverName(fn)
				if recv == "" {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					sel, ok := n.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "client" {
						return true
					}
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == recv {
						t.Errorf("%s: %s accesses %s.client directly; use r.ec(ctx) / ClientFromContext",
							fset.Position(sel.Pos()), fn.Name.Name, recv)
					}
					return true
				})
			}
		}
	}
}

func receiverName(fn *ast.FuncDecl) string {
	if len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}
