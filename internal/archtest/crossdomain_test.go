package archtest

import (
	"go/ast"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestTrustedOnlyInModules: Trusted домена — операции без policy для других доменов; их берут только
// адаптеры module/ потребителей, иначе вход (HTTP, NATS) обошёл бы авторизацию. Свой module/
// поставщика только провайдит NewTrusted: интерфейс над *Trusted в нём открыл бы доверенные методы
// своему же входу.
func TestTrustedOnlyInModules(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func trustedOnlyInModules(c *check) {
	for _, f := range c.files {
		if f.test {
			continue
		}
		domain, sub, inDomain := splitDomain(f.pkg)
		file := c.parse(f.path)
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
				c.Errorf("%s uses %s.Trusted: operations without policy are for adapters in module/ of consumer domains", f.path, id.Name)
			}
			return true
		})
	}
}

// TestTrustedErrors: метод Trusted отдаёт ошибки домена только из своей строки TrustedErrors (метод → ошибки) — их
// переводит адаптер потребителя, другая ушла бы ему 500. Методы — по получателю в любом файле корня; исходы порта
// держит тест Trusted, свои return — этот.
func TestTrustedErrors(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func trustedErrors(c *check) {
	byDomain := map[string][]goFile{} // домен → не-тестовые файлы корня
	for _, f := range c.files {
		if domain, sub, ok := splitDomain(f.pkg); ok && sub == "" && !f.test {
			byDomain[domain] = append(byDomain[domain], f)
		}
	}
	for domain, files := range byDomain {
		var methods []*ast.FuncDecl
		var listed map[string]map[string]bool // метод → ошибки его строки
		where := "internal/domain/" + domain
		for _, f := range files {
			file := c.parse(f.path)
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && receiverName(fn) == "Trusted" {
					methods = append(methods, fn)
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				vs, ok := n.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "TrustedErrors" || len(vs.Values) != 1 {
					return true
				}
				listed = trustedErrorLines(vs.Values[0])
				return false
			})
		}
		if len(methods) == 0 {
			continue
		}
		if listed == nil {
			c.Errorf("%s declares Trusted without TrustedErrors = map[string][]error{<method>: {<errors>}}: consumer adapters translate the listed errors of each method", where)
			continue
		}
		sentinels := domainErrors(c, domain)
		declared := map[string]bool{}
		for _, m := range methods {
			declared[m.Name.Name] = true
			allowed, ok := listed[m.Name.Name]
			if !ok {
				c.Errorf("%s: Trusted.%s has no line in TrustedErrors: its consumers would not know which errors to translate", where, m.Name.Name)
				continue
			}
			ast.Inspect(m.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				ast.Inspect(ret, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && slices.Contains(sentinels, id.Name) && !allowed[id.Name] {
						c.Errorf("%s: Trusted.%s returns %s, which is not in its line of TrustedErrors: consumer adapters translate only the listed errors", where, m.Name.Name, id.Name)
					}
					return true
				})
				return false
			})
		}
		for name := range listed {
			if !declared[name] {
				c.Errorf("%s: TrustedErrors lists %s, which Trusted does not declare", where, name)
			}
		}
	}
}

// trustedErrorLines — строки литерала map[string][]error{"Метод": {ErrA, ErrB}}: метод → имена ошибок; nil — не
// такой литерал.
func trustedErrorLines(expr ast.Expr) map[string]map[string]bool {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	if _, isMap := lit.Type.(*ast.MapType); !isMap {
		return nil
	}
	lines := map[string]map[string]bool{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.BasicLit)
		if !ok {
			continue
		}
		method, err := strconv.Unquote(key.Value)
		if err != nil {
			continue
		}
		lines[method] = map[string]bool{}
		if errs, ok := kv.Value.(*ast.CompositeLit); ok {
			for _, e := range errs.Elts {
				if id, ok := e.(*ast.Ident); ok {
					lines[method][id.Name] = true
				}
			}
		}
	}
	return lines
}

// TestModulesEmbedNoForeignTypes: порт между доменами — типовой адаптер, который объявляет каждый метод и
// переводит его ошибки. Структура module/ со встроенным типом чужого домена отдала бы порту его методы и
// sentinel'ы как есть — тот же обход, что identity-closure.
func TestModulesEmbedNoForeignTypes(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func modulesEmbedNoForeignTypes(c *check) {
	for _, f := range c.files {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "module" || f.test {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				typ := field.Type
				if star, ok := typ.(*ast.StarExpr); ok {
					typ = star.X
				}
				sel, ok := typ.(*ast.SelectorExpr)
				if !ok || len(field.Names) != 0 {
					continue
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok {
					continue
				}
				if provider, provSub, ok := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/")); ok && provSub == "" && provider != domain {
					c.Errorf("%s embeds %s.%s: a port between domains is a typed adapter that declares each method and maps its errors", f.path, id.Name, sel.Sel.Name)
				}
			}
			return true
		})
	}
}

// TestNoFxAs: порт привязывает тип результата провайдера или identity-closure (architecture.md § fx);
// fx.As — вторая идиома, и порт между доменами через неё обошёл бы правило archkit об identity-closure.
func TestNoFxAs(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func noFxAs(c *check) {
	for _, f := range c.files {
		if f.test || !slices.Contains(f.imports, "go.uber.org/fx") {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "As" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && byName[id.Name] == "go.uber.org/fx" {
				c.Errorf("%s uses fx.As: a port is bound by the provider's result type or an identity closure (architecture.md § fx)", f.path)
			}
			return true
		})
	}
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}
