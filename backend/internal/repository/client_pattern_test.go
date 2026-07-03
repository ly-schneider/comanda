package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestReposResolveClientFromContext fails when a repository method reads the
// injected ent client field directly instead of going through the ec(ctx)
// helper. The injected client is the raw RLS-bound app pool with no
// app.tenant_id GUC, so bypassing ClientFromContext fail-closes every read to
// zero rows and every write to a 42501 RLS violation — silently, because the
// fallback only misbehaves outside tests that use a privileged client.
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
