package archtest

import (
	"go/ast"
	"go/token"
	"maps"
	"path"
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

// fxHooks — хуки жизненного цикла fx: логика старта в module/ ушла бы из-под тестов usecase.
var fxHooks = []string{"Lifecycle", "Hook", "StartHook", "StopHook", "StartStopHook"}

// TestModulesOnlyWire: module/ домена связывает граф и переводит типы, а не решает. Его файлы — module.go (Module и
// привязки) и adapters.go (адаптеры): тип и конструктор адаптера — в adapters.go, и он видит то же, что корень
// домена, — кит, драйвер и клиент живут в infra/<адаптер>; функция module.go и литерал в нём — один return
// (привязка порта, сборка значения, job); хуков fx (Lifecycle, Hook) нет — подписку ставит natskit.SubscribeOnStart
// во входе, job — timerkit. Другой домен адаптер зовёт через его Trusted: методы Usecase спрашивают policy вызывающего.
func TestModulesOnlyWire(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func modulesOnlyWire(c *check) {
	adapters := map[string]map[string]bool{} // пакет module/ → типы, которые он объявляет
	for _, f := range c.files {
		if _, sub, ok := splitDomain(f.pkg); ok && sub == "module" && !f.test {
			if adapters[f.pkg] == nil {
				adapters[f.pkg] = map[string]bool{}
			}
			ast.Inspect(c.parse(f.path), func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok {
					adapters[f.pkg][ts.Name.Name] = true
				}
				return true
			})
		}
	}
	visible := rootVisible(c)
	for _, f := range c.files {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "module" || f.test {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		switch path.Base(f.path) {
		case "module.go":
			moduleGo(c, f.path, file, adapters[f.pkg])
		case "adapters.go":
			for _, imp := range f.imports {
				if !visible(imp) {
					c.Errorf("%s imports %s: an adapter translates types and errors between domains and sees what a domain root sees — a kit, a driver or a client lives in infra/<adapter> of the domain", f.path, imp)
				}
			}
		default:
			c.Errorf("%s: module/ is module.go (Module and bindings) and adapters.go (adapters)", f.path)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if byName[id.Name] == "go.uber.org/fx" && slices.Contains(fxHooks, sel.Sel.Name) {
				c.Errorf("%s uses fx.%s: module/ wires, it does not act on start or stop — logic lives in a tested usecase", f.path, sel.Sel.Name)
			}
			if provider, provSub, ok := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/")); ok && provSub == "" && provider != domain && sel.Sel.Name == "Usecase" {
				c.Errorf("%s uses %s.Usecase: an adapter calls another domain through its Trusted — Usecase methods ask the caller's policy", f.path, id.Name)
			}
			return true
		})
	}
}

// rootVisible — импорты, которые видит корень домена (archkit TestLayout): stdlib без I/O, rootLibs, корни доменов,
// пакеты pkg/ и internal/platform/, чьи файлы импортируют только видимое (или из uncheckedPackages).
func rootVisible(c *check) func(imp string) bool {
	imports := map[string][]string{} // пакет модуля → импорты его не-тестовых файлов
	for _, f := range c.files {
		if !f.test {
			imports[f.pkg] = append(imports[f.pkg], f.imports...)
		}
	}
	memo := map[string]bool{}
	var visible func(imp string) bool
	visible = func(imp string) bool {
		rel, inModule := strings.CutPrefix(imp, module+"/")
		switch {
		case !inModule:
			return slices.Contains(rootLibs, imp) || !strings.Contains(strings.Split(imp, "/")[0], ".") && !ioStdlib(imp)
		case !under(rel, "pkg") && !under(rel, "internal/platform"):
			_, sub, ok := splitDomain(rel)
			return ok && sub == ""
		case slices.Contains(uncheckedPackages, rel):
			return true
		}
		if v, done := memo[rel]; done {
			return v
		}
		memo[rel] = true // цикл импортов не компилируется: запись до обхода — от бесконечной рекурсии
		memo[rel] = !slices.ContainsFunc(imports[rel], func(i string) bool { return !visible(i) })
		return memo[rel]
	}
	return visible
}

// ioStdlib — stdlib, которая ходит в ОС, сеть или БД (как у archkit); парсеры из net — не I/O.
func ioStdlib(imp string) bool {
	if slices.Contains([]string{"net/mail", "net/netip", "net/url"}, imp) {
		return false
	}
	return slices.ContainsFunc([]string{"database/sql", "net", "os", "syscall"}, func(p string) bool { return under(imp, p) })
}

// moduleGo: module.go не объявляет типов и конструкторов адаптера (результат с типом module/ — adapters), а каждая
// его функция и литерал — один return.
func moduleGo(c *check, rel string, file *ast.File, adapters map[string]bool) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				c.Errorf("%s declares a type: an adapter's type and constructor live in adapters.go", rel)
			}
		case *ast.FuncDecl:
			if d.Type.Results != nil && mentions(d.Type.Results, adapters) {
				c.Errorf("%s declares %s, a constructor of an adapter: an adapter's type and constructor live in adapters.go", rel, d.Name.Name)
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		_, body := funcParts(n)
		if body != nil && (len(body.List) != 1 || !isReturn(body.List[0])) {
			c.Errorf("%s: a function of module.go is one return (a port binding or a value): logic lives in a tested usecase or adapter", rel)
		}
		return true
	})
}

func isReturn(stmt ast.Stmt) bool {
	_, ok := stmt.(*ast.ReturnStmt)
	return ok
}

// TestDomainDependencies: порт домена (интерфейс его корня) — результат функции только в самом домене: в cmd/,
// platform или чужом module/ привязка отдала бы порту чужой тип мимо адаптера, который переводит ошибки. Домены
// зависят друг от друга адаптерами module/, и этот граф ацикличен: цикл склеивает два домена в один.
func TestDomainDependencies(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func domainDependencies(c *check) {
	ports := map[string]map[string]bool{} // корень домена (путь импорта) → его интерфейсы
	for _, d := range c.domains() {
		ports[module+"/internal/domain/"+d] = map[string]bool{}
		for _, file := range rootFiles(c, d) {
			ast.Inspect(file, func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok {
					if _, ok := ts.Type.(*ast.InterfaceType); ok {
						ports[module+"/internal/domain/"+d][ts.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	edges := map[string][]string{} // домен → домены, которые знает его module/
	for _, f := range c.files {
		if f.test {
			continue
		}
		domain, sub, inDomain := splitDomain(f.pkg)
		if inDomain && sub == "module" {
			for _, imp := range f.imports {
				if other, otherSub, ok := splitDomain(strings.TrimPrefix(imp, module+"/")); ok && otherSub == "" && other != domain {
					edges[domain] = append(edges[domain], other)
				}
			}
		}
		if !slices.ContainsFunc(f.imports, func(imp string) bool { return ports[imp] != nil }) {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			fn, _ := funcParts(n)
			if fn == nil || fn.Results == nil {
				return true
			}
			for _, res := range fn.Results.List {
				sel, ok := res.Type.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok || !ports[byName[id.Name]][sel.Sel.Name] {
					continue
				}
				if owner, _, _ := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/")); !inDomain || domain != owner {
					c.Errorf("%s returns %s.%s, a port of %s: a port is bound inside its domain — another domain's type gets there through an adapter in internal/domain/%s/module", f.path, id.Name, sel.Sel.Name, owner, owner)
				}
			}
			return true
		})
	}
	for _, cycle := range domainCycles(edges) {
		c.Errorf("domains depend on each other through module/: %s — a consumer port goes one way, the other side subscribes to an event", strings.Join(cycle, " → "))
	}
}

// domainCycles — циклы графа доменов: каждый — путь от домена к нему же.
func domainCycles(edges map[string][]string) [][]string {
	var cycles [][]string
	seen := map[string]bool{}
	for _, start := range slices.Sorted(maps.Keys(edges)) {
		var walk func(d string, path []string)
		walk = func(d string, path []string) {
			for _, next := range edges[d] {
				switch {
				case next == start:
					cycle := append(slices.Clone(path), next)
					key := strings.Join(slices.Sorted(slices.Values(cycle[:len(cycle)-1])), ",")
					if !seen[key] {
						seen[key] = true
						cycles = append(cycles, cycle)
					}
				case !slices.Contains(path, next):
					walk(next, append(path, next))
				}
			}
		}
		walk(start, []string{start})
	}
	return cycles
}
