package archtest

import (
	"go/ast"
	"slices"
	"strings"
	"testing"
)

// TestTrustedOnlyInModules: Trusted домена — операции без policy для других доменов; их берут только
// адаптеры module/ потребителей, иначе вход (HTTP, NATS) обошёл бы авторизацию. Свой module/
// поставщика только провайдит NewTrusted: интерфейс над *Trusted в нём открыл бы доверенные методы
// своему же входу.
func TestTrustedOnlyInModules(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if f.test {
			continue
		}
		domain, sub, inDomain := splitDomain(f.pkg)
		file := parseGo(t, f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Trusted" {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			provider, provSub, ok := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/"))
			if ok && provSub == "" && (!inDomain || sub != "module" || provider == domain) {
				t.Errorf("%s uses %s.Trusted: operations without policy are for adapters in module/ of consumer domains", f.path, id.Name)
			}
			return true
		})
	}
}

// TestTrustedErrors: Trusted отдаёт ошибки домена только из TrustedErrors — их переводят адаптеры
// потребителей, другая ушла бы им 500. Исходы репозитория держит тест trusted.go, свои return — этот.
func TestTrustedErrors(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "" || !strings.HasSuffix(f.path, "/trusted.go") {
			continue
		}
		file := parseGo(t, f.path)
		listed := map[string]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "TrustedErrors" || len(vs.Values) != 1 {
				return true
			}
			if lit, ok := vs.Values[0].(*ast.CompositeLit); ok {
				for _, elt := range lit.Elts {
					if id, ok := elt.(*ast.Ident); ok {
						listed[id.Name] = true
					}
				}
			}
			return false
		})
		sentinels := domainErrors(t, domain)
		ast.Inspect(file, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			ast.Inspect(ret, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && slices.Contains(sentinels, id.Name) && !listed[id.Name] {
					t.Errorf("%s returns %s, which is not in TrustedErrors: consumer adapters translate only the listed errors", f.path, id.Name)
				}
				return true
			})
			return false
		})
	}
}

// TestNoFxAs: порт привязывает тип результата провайдера или identity-closure (architecture.md § fx);
// fx.As — вторая идиома, и порт между доменами через неё обошёл бы правило archkit об identity-closure.
func TestNoFxAs(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if f.test || !slices.Contains(f.imports, "go.uber.org/fx") {
			continue
		}
		file := parseGo(t, f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "As" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && byName[id.Name] == "go.uber.org/fx" {
				t.Errorf("%s uses fx.As: a port is bound by the provider's result type or an identity closure (architecture.md § fx)", f.path)
			}
			return true
		})
	}
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}
