package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Settings are read by string key (n.f("capacity"), pf(p, "capacity", 0)) and nothing in the compiler ties those keys to
// the registry. This test reads the engine's own source and fails when a literal key is read that no component
// declares (a typo, or a setting that was renamed in the registry only), and when a setting is declared that no
// code reads and that is not listed as display-only below.

// settingsDeclaredButNotRead are registry settings that no engine code reads by a literal key. Keep this list
// empty unless a setting is read through a computed key or is only shown to the learner; say why.
var settingsDeclaredButNotRead = map[string]string{
	"variability": "read as n.p[\"variability\"] in serviceDist, not through n.s",
	"heartbeatMs": "declared on the coordinator but nothing reads it yet: a candidate for removal (it is in the served registry, so removing it is a wire change)",
}

func literalSettingReads(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	reads := map[string][]string{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			ast.Inspect(f, func(x ast.Node) bool {
				call, ok := x.(*ast.CallExpr)
				if !ok {
					return true
				}
				var keyArg ast.Expr
				switch fn := call.Fun.(type) {
				case *ast.SelectorExpr: // n.f("key"), n.s("key"), n.b("key")
					if (fn.Sel.Name == "f" || fn.Sel.Name == "s" || fn.Sel.Name == "b") && len(call.Args) == 1 {
						keyArg = call.Args[0]
					}
				case *ast.Ident: // pf(p, "key", def), ps(p, "key"), pb(p, "key")
					if (fn.Name == "pf" || fn.Name == "ps" || fn.Name == "pb") && len(call.Args) >= 2 {
						keyArg = call.Args[1]
					}
				}
				lit, ok := keyArg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				reads[key] = append(reads[key], fset.Position(call.Pos()).String())
				return true
			})
		}
	}
	return reads
}

func declaredSettings() map[string]bool {
	declared := map[string]bool{}
	for _, sp := range Specs() {
		for _, p := range sp.Params {
			declared[p.Key] = true
		}
	}
	return declared
}

func TestEverySettingReadByKeyIsDeclaredInTheRegistry(t *testing.T) {
	declared := declaredSettings()
	for key, where := range literalSettingReads(t) {
		if !declared[key] {
			t.Errorf("setting %q is read but no component declares it (%s)", key, where[0])
		}
	}
}

func TestEverySettingInTheRegistryIsRead(t *testing.T) {
	reads := literalSettingReads(t)
	var unread []string
	for key := range declaredSettings() {
		if _, ok := reads[key]; !ok {
			if _, listed := settingsDeclaredButNotRead[key]; !listed {
				unread = append(unread, key)
			}
		}
	}
	sort.Strings(unread)
	if len(unread) > 0 {
		t.Errorf("settings declared in the registry but never read by a literal key (add each to settingsDeclaredButNotRead with a reason, or remove it): %v", unread)
	}
}
