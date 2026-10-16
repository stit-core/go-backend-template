package archtest

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	humaPath    = "github.com/danielgtaylor/huma/v2"
	natskitPath = "git.stit.tech/stit-core/golibinfra/natskit"
	golibinfra  = "git.stit.tech/stit-core/golibinfra"
)

// TestDeliveryPackages: вход домена — пакет delivery/<транспорт> и его mocks. Правила входа читают этот пакет:
// подпакет под ним (delivery/http/admin) они бы не увидели.
func TestDeliveryPackages(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func deliveryPackages(c *check) {
	seen := map[string]bool{}
	for _, f := range c.files {
		_, sub, ok := splitDomain(f.pkg)
		if !ok || !under(sub, "delivery") || seen[f.pkg] {
			continue
		}
		seen[f.pkg] = true
		if parts := strings.Split(sub, "/"); len(parts) != 2 && (len(parts) != 3 || parts[2] != "mocks") {
			c.Errorf("%s: a domain input is one package delivery/<transport> with its mocks — the rules of HTTP types, error tables, policy and event contracts read that package", f.pkg)
		}
	}
}

// humaShortcuts — короткие формы регистрации huma: операция без таблицы ошибок, мимо правил huma.Register.
var humaShortcuts = []string{"Get", "Post", "Put", "Patch", "Delete", "Head"}

const ssePath = "github.com/danielgtaylor/huma/v2/sse"

// registrars — функции, которые заводят операцию HTTP, и номер их аргумента-хендлера.
var registrars = map[string]map[string]int{
	humaPath: {"Register": 2},
	ssePath:  {"Register": 3},
}

// TestInputsRegisterInDelivery: операцию HTTP регистрирует вызов huma.Register (поток — sse.Register) в
// delivery/http домена, хендлер — метод, функция или литерал этого пакета; подписку на события заводит natskit в
// delivery/nats. Правила входа читают эти пакеты и этих хендлеров: операция в platform, module/ или cmd/,
// регистрация значением-функцией, обёрткой или с хендлером-декоратором обошла бы их.
func TestInputsRegisterInDelivery(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func inputsRegisterInDelivery(c *check) {
	files := c.files
	declared := map[string]map[string]bool{} // пакет delivery/http → «m:<метод>», «f:<функция>»
	for _, f := range files {
		if _, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/http" && !f.test {
			if declared[f.pkg] == nil {
				declared[f.pkg] = map[string]bool{}
			}
			for _, decl := range c.parse(f.path).Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					declared[f.pkg][map[bool]string{true: "m:", false: "f:"}[fn.Recv != nil]+fn.Name.Name] = true
				}
			}
		}
	}
	for _, f := range files {
		if f.test || !slices.ContainsFunc(f.imports, func(imp string) bool { return imp == humaPath || imp == ssePath || imp == natskitPath }) {
			continue
		}
		file := c.parse(f.path)
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); imp.Name != nil && imp.Name.Name == "." && (p == humaPath || p == ssePath || p == natskitPath) {
				c.Errorf("%s dot-imports %s: archtest would not see the operations and subscriptions it registers", f.path, p)
			}
		}
		byName := importsByName(file)
		_, sub, inDomain := splitDomain(f.pkg)
		inHTTP, inNATS := inDomain && sub == "delivery/http", inDomain && sub == "delivery/nats"
		called := map[ast.Expr]bool{} // функции вызовов: регистратор вне вызова — значение
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				called[unwrapIndex(call.Fun)] = true
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				id, ok := x.X.(*ast.Ident)
				if !ok {
					return true
				}
				pkg, name := byName[id.Name], x.Sel.Name
				_, registrar := registrars[pkg][name]
				switch {
				case pkg == humaPath && slices.Contains(humaShortcuts, name):
					c.Errorf("%s registers an operation with huma.%s: an operation is registered by huma.Register with its error table", f.path, name)
				case registrar && !inHTTP:
					c.Errorf("%s registers an HTTP operation (%s): operations live in internal/domain/<d>/delivery/http, where the rules of error tables and policy read them", f.path, types.ExprString(x))
				case registrar && !called[x]:
					c.Errorf("%s uses %s as a value: an operation is registered by a direct call, which the rules of error tables and policy read", f.path, types.ExprString(x))
				case pkg == natskitPath && strings.HasPrefix(name, "Subscribe") && !inNATS:
					c.Errorf("%s subscribes to events (natskit.%s): subscriptions live in internal/domain/<d>/delivery/nats, where the consumer's contract test checks them", f.path, name)
				}
			case *ast.CallExpr:
				pkg, name := kitCall(x, byName)
				if arg, ok := registrars[pkg][name]; ok && inHTTP && len(x.Args) > arg && !localHandler(x.Args[arg], byName, declared[f.pkg]) {
					c.Errorf("%s registers handler %s, which is not a method, function or literal of delivery/http: the rules of error tables and policy read only those", f.path, types.ExprString(x.Args[arg]))
				}
			}
			return true
		})
	}
}

// unwrapIndex — функция без параметров типа: huma.Register[I, O] → huma.Register.
func unwrapIndex(fun ast.Expr) ast.Expr {
	switch x := fun.(type) {
	case *ast.IndexExpr:
		return x.X
	case *ast.IndexListExpr:
		return x.X
	}
	return fun
}

// localHandler — хендлер — литерал, функция пакета или метод пакета на его значении (h.list).
func localHandler(expr ast.Expr, byName map[string]string, declared map[string]bool) bool {
	switch h := expr.(type) {
	case *ast.FuncLit:
		return true
	case *ast.Ident:
		return declared["f:"+h.Name]
	case *ast.SelectorExpr:
		id, ok := h.X.(*ast.Ident)
		return ok && byName[id.Name] == "" && declared["m:"+h.Sel.Name]
	}
	return false
}

// TestSubscriptionsHaveContractTest: схему подписки со схемой поставщика сверяет контракт-тест потребителя
// delivery/nats/contract_test.go — каждой подписке Register (bus.Subscriptions()) событие настоящего usecase
// поставщика. Без него схема подписки держится только на её собственном тесте, а он пишет payload сам. Правило
// держит присутствие теста, вызова Subscriptions и источник событий: событие, записанное рукой (outboxtest.Event),
// с subject домена этого репо — только payload прежней версии, в функции с «снять в релизе» в комментарии (event.md
// § Versioning); событие другого сервиса — из его контракта. Что сверка верна, держит сам тест.
func TestSubscriptionsHaveContractTest(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func subscriptionsHaveContractTest(c *check) {
	subscribers := map[string]bool{} // пакеты delivery/nats с подпиской
	for _, f := range c.files {
		if _, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/nats" && !f.test && slices.Contains(f.imports, natskitPath) {
			subscribers[f.pkg] = true
		}
	}
	domains := c.domains()
	for pkg := range subscribers {
		contract := pkg + "/contract_test.go"
		if _, err := os.Stat(filepath.Join(c.root, contract)); err != nil {
			c.Errorf("%s subscribes to events without %s: the contract test delivers every subscription an event of the provider's usecase (rule events-and-messaging)", pkg, contract)
			continue
		}
		file := c.parse(contract)
		checked := false
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			checked = checked || ok && sel.Sel.Name == "Subscriptions"
			return !checked
		})
		if !checked {
			c.Errorf("%s does not check the subscriptions of Register against the provider events it delivers (bus.Subscriptions()): a subscription without an event would pass", contract)
		}
		subjects := packageStrings(c, pkg, file)
		byName := importsByName(file)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Doc != nil && strings.Contains(fn.Doc.Text(), "снять в релизе") {
				continue
			}
			for _, subject := range handEventSubjects(decl, byName, module+"/"+pkg, subjects) {
				if d, _, _ := strings.Cut(subject, ".events."); slices.Contains(domains, d) && strings.Contains(subject, ".events.") {
					c.Errorf("%s writes an event of %s by hand (%q): an event of a provider of this repo comes from its usecase; a recorded payload of the previous version sits in a function whose comment says «снять в релизе …» (event.md § Versioning)", contract, d, subject)
				}
			}
		}
	}
}

const outboxtestPath = golibinfra + "/testkit/outboxtest"

// packageStrings — строковые константы пакета pkg и файла file: имя → значение.
func packageStrings(c *check, pkg string, file *ast.File) map[string]string {
	out := map[string]string{}
	collect := func(f *ast.File) {
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.CONST {
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						if i < len(vs.Values) {
							if val, ok := stringLit(vs.Values[i]); ok {
								out[name.Name] = val
							}
						}
					}
				}
			}
		}
	}
	for _, f := range c.files {
		if f.pkg == pkg && !f.test {
			collect(c.parse(f.path))
		}
	}
	collect(file)
	return out
}

// handEventSubjects — subject'ы событий, записанных рукой в узле: литерал outboxtest.Event{…} (и элемент
// []outboxtest.Event{{…}}), чей Subject — строковый литерал или константа пакета подписки (pkgPath) либо файла.
func handEventSubjects(node ast.Node, byName map[string]string, pkgPath string, consts map[string]string) []string {
	var out []string
	event := func(lit *ast.CompositeLit) {
		for i, elt := range lit.Elts {
			value := elt
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if !isIdent(kv.Key, "Subject") {
					continue
				}
				value = kv.Value
			} else if i != 0 {
				continue
			}
			switch v := value.(type) {
			case *ast.BasicLit:
				if val, ok := stringLit(v); ok {
					out = append(out, val)
				}
			case *ast.Ident:
				if val, ok := consts[v.Name]; ok {
					out = append(out, val)
				}
			case *ast.SelectorExpr:
				if id, ok := v.X.(*ast.Ident); ok && byName[id.Name] == pkgPath {
					if val, ok := consts[v.Sel.Name]; ok {
						out = append(out, val)
					}
				}
			}
		}
	}
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if isSelector(lit.Type, byName, outboxtestPath, "Event") {
			event(lit)
		}
		if arr, ok := lit.Type.(*ast.ArrayType); ok && isSelector(arr.Elt, byName, outboxtestPath, "Event") {
			for _, elt := range lit.Elts {
				if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
					event(inner)
				}
			}
		}
		return true
	})
	return out
}

// kitCall — пакет и имя вызова <пакет>.<имя>(…), и с параметрами типа (huma.Register[I, O]); "" — не такой вызов.
func kitCall(call *ast.CallExpr, byName map[string]string) (string, string) {
	sel, ok := unwrapIndex(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return byName[id.Name], sel.Sel.Name
}

// TestDeliveryReachesUsecaseOnly: вход домена зовёт usecase своего домена портом пакета. Из корня он берёт
// данные — сущности, DTO, ошибки, — а не порты, usecase, Trusted и их зависимости; из golibinfra — только
// deliveryKits (layout_settings_test.go), database/sql и pgx — нет: иначе операция прошла бы мимо usecase и его
// policy. Клиент stdlib (net/http) и хранилище за пакетом internal/platform или pkg/ правило не видит — ревью.
func TestDeliveryReachesUsecaseOnly(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func deliveryReachesUsecaseOnly(c *check) {
	services := map[string]map[string]bool{} // домен → имена корня, за которыми зависимости
	for _, f := range c.files {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || f.test || !under(sub, "delivery") || path.Base(sub) == "mocks" {
			continue
		}
		for _, imp := range f.imports {
			kit, isKit := strings.CutPrefix(imp, golibinfra+"/")
			if isKit && !slices.Contains(deliveryKits, kit) || under(imp, "database/sql") || under(imp, "github.com/jackc/pgx") {
				c.Errorf("%s imports %s: delivery reaches the domain through its usecase port — stores, clients and drivers sit behind the usecase's ports", f.path, imp)
			}
		}
		if services[domain] == nil {
			services[domain] = rootServices(c, domain)
		}
		root := module + "/internal/domain/" + domain
		file := c.parse(f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && byName[id.Name] == root && services[domain][sel.Sel.Name] {
				c.Errorf("%s uses %s.%s: delivery takes data types from its domain root and calls the domain through its Usecase port — ports, the usecase, Trusted and their dependencies are wired in module/", f.path, id.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// rootServices — имена корня домена, за которыми зависимости, а не данные: структуры с fx.In и типы корня в их полях
// (порты), параметры конструкторов Usecase и Trusted, сами Usecase и Trusted, типы, которые упоминают эти имена, и
// функции с ними в сигнатуре (NewUsecase, NewPolicy). Интерфейс корня вне зависимостей — данные: sum-type сущности,
// приёмник, который метод usecase получает параметром.
func rootServices(c *check, domain string) map[string]bool {
	var (
		specs []*ast.TypeSpec
		funcs []*ast.FuncDecl
		deps  []*ast.StructType
	)
	declared := map[string]bool{}                                 // типы корня
	services := map[string]bool{"Usecase": true, "Trusted": true} // и структуры с fx.In
	for _, file := range rootFiles(c, domain) {
		byName := importsByName(file)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				funcs = append(funcs, fn)
			}
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				specs = append(specs, ts)
				declared[ts.Name.Name] = true
				if st, ok := ts.Type.(*ast.StructType); ok && embedsFxIn(st, byName) {
					deps = append(deps, st)
					services[ts.Name.Name] = true
				}
			}
		}
	}
	addTypes := func(node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				return false
			case *ast.Ident:
				services[x.Name] = services[x.Name] || declared[x.Name]
			}
			return true
		})
	}
	for _, st := range deps {
		for _, field := range st.Fields.List {
			addTypes(field.Type)
		}
	}
	for _, fn := range funcs {
		if fn.Type.Results != nil && mentions(fn.Type.Results, map[string]bool{"Usecase": true, "Trusted": true}) {
			addTypes(fn.Type.Params)
		}
	}
	for grew := true; grew; {
		grew = false
		for _, ts := range specs {
			if !services[ts.Name.Name] && mentions(ts.Type, services) {
				services[ts.Name.Name], grew = true, true
			}
		}
	}
	for _, fn := range funcs {
		if mentions(fn.Type, services) {
			services[fn.Name.Name] = true
		}
	}
	return services
}

// rootFiles — не-тестовые файлы корня домена.
func rootFiles(c *check, domain string) []*ast.File {
	var out []*ast.File
	for _, f := range c.files {
		if d, sub, ok := splitDomain(f.pkg); ok && d == domain && sub == "" && !f.test {
			out = append(out, c.parse(f.path))
		}
	}
	return out
}

// mentions — в типе есть тип своего пакета из names; имена полей и параметров и типы других пакетов — не они.
func mentions(node ast.Node, names map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			return false
		case *ast.Field:
			found = found || mentions(x.Type, names)
			return false
		case *ast.Ident:
			found = found || names[x.Name]
		}
		return !found
	})
	return found
}

// TestDeliveryPortsBoundToUsecase: порт входа (интерфейс пакета delivery/<транспорт>) получает usecase своего
// домена — identity-closure над *<домен>.Usecase в его module/. Другое упоминание порта вне входа — поле fx.Out,
// алиас, параметр — отказ: порт над репозиторием, Trusted или хранилищем кита отдал бы входу операции мимо usecase и
// policy. Экспортируемый тип-функция пакета входа вне входа — тоже отказ: портом он не служит, к usecase его не
// привязать, а функция репозитория или Trusted дала бы то же.
func TestDeliveryPortsBoundToUsecase(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func deliveryPortsBoundToUsecase(c *check) {
	files := c.files
	ports := map[string]map[string]bool{}     // путь импорта пакета delivery/<транспорт> → его порты-интерфейсы
	funcTypes := map[string]map[string]bool{} // … → его экспортируемые типы-функции
	for _, f := range files {
		if _, sub, ok := splitDomain(f.pkg); !ok || f.test || !under(sub, "delivery") {
			continue
		}
		pkg := module + "/" + f.pkg
		if ports[pkg] == nil {
			ports[pkg], funcTypes[pkg] = map[string]bool{}, map[string]bool{}
		}
		ast.Inspect(c.parse(f.path), func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				switch ts.Type.(type) {
				case *ast.InterfaceType:
					ports[pkg][ts.Name.Name] = true
				case *ast.FuncType:
					funcTypes[pkg][ts.Name.Name] = ast.IsExported(ts.Name.Name)
				}
			}
			return true
		})
	}
	for _, f := range files {
		_, sub, inDomain := splitDomain(f.pkg)
		if f.test || inDomain && under(sub, "delivery") || !slices.ContainsFunc(f.imports, func(imp string) bool { return ports[imp] != nil }) {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		bound := map[ast.Expr]bool{} // результаты closure над *Usecase в module/ своего домена
		ast.Inspect(file, func(n ast.Node) bool {
			fn, body := funcParts(n)
			if fn == nil || fn.Results == nil || len(fn.Results.List) != 1 {
				return true
			}
			result := fn.Results.List[0].Type
			if domain, ok := portDomain(result, byName, ports); ok && f.pkg == "internal/domain/"+domain+"/module" &&
				usecaseClosure(fn, body, byName, module+"/internal/domain/"+domain) {
				bound[result] = true
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || bound[sel] {
				return true
			}
			port := types.ExprString(sel)
			if domain, ok := portDomain(sel, byName, ports); ok {
				c.Errorf("%s uses %s otherwise than as the result of func(uc *%s.Usecase) %s { return uc } in internal/domain/%s/module: an input port gets the usecase of its domain and its policy", f.path, port, domain, port, domain)
			}
			if domain, ok := portDomain(sel, byName, funcTypes); ok {
				c.Errorf("%s uses %s, a function type of internal/domain/%s/delivery: an input port is an interface of delivery, bound to *%s.Usecase by func(uc *%s.Usecase) <Port> { return uc } in its module — a function would hand the input an operation past the usecase and its policy", f.path, port, domain, domain, domain)
			}
			return true
		})
	}
}

// portDomain — домен входа, если выражение — <пакет delivery/<транспорт>>.<имя из names> (порт или тип-функция).
func portDomain(expr ast.Expr, byName map[string]string, names map[string]map[string]bool) (string, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || !names[byName[id.Name]][sel.Sel.Name] {
		return "", false
	}
	domain, _, _ := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/"))
	return domain, true
}

// funcParts — сигнатура и тело объявленной функции или литерала; nil — не функция.
func funcParts(n ast.Node) (*ast.FuncType, *ast.BlockStmt) {
	switch fn := n.(type) {
	case *ast.FuncDecl:
		return fn.Type, fn.Body
	case *ast.FuncLit:
		return fn.Type, fn.Body
	}
	return nil, nil
}

// usecaseClosure — func(uc *<root>.Usecase) Port { return uc }.
func usecaseClosure(fn *ast.FuncType, body *ast.BlockStmt, byName map[string]string, root string) bool {
	params := fn.Params.List
	if len(params) != 1 || len(params[0].Names) != 1 || body == nil || len(body.List) != 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	ret, isReturn := body.List[0].(*ast.ReturnStmt)
	return ok && isReturn && isSelector(star.X, byName, root, "Usecase") && len(ret.Results) == 1 &&
		isIdent(ret.Results[0], params[0].Names[0].Name)
}

// TestSecuredOperationsAskPolicy: метод usecase за операцией под Security (не httpkit.Public) спрашивает policy
// (…Policy.Can…) — сам или своим методом и раньше вызова порта зависимостей: чтение до решения выдаёт, есть ли
// ресурс; чтение своего (ключ вызывающего — аргумент) — нет. Без policy — только чтение самого вызывающего: его UserID из identity.From — аргумент вызова порта
// зависимостей (uc.d.Repo.GetByID — Me), а не лога, кита или функции пакета. Иначе операция отдала бы ресурс любому
// вошедшему. Операцию правило читает литералом в вызове регистрации или переменной пакета (operation); другую форму —
// отказ: открытость операции была бы не видна.
func TestSecuredOperationsAskPolicy(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func securedOperationsAskPolicy(c *check) {
	byDomain := map[string][]goFile{} // домен → не-тестовые файлы delivery/http
	for _, f := range c.files {
		if domain, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/http" && !f.test {
			byDomain[domain] = append(byDomain[domain], f)
		}
	}
	for domain, files := range byDomain {
		root := loadPolicyRoot(c, domain)
		parsed := make([]*ast.File, len(files))
		ifaces := map[string]bool{}
		funcs := map[string]*ast.FuncDecl{} // методы и функции пакета
		for i, f := range files {
			parsed[i] = c.parse(f.path)
			for _, decl := range parsed[i].Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					funcs[fn.Name.Name] = fn
				}
			}
			ast.Inspect(parsed[i], func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok {
					if _, ok := ts.Type.(*ast.InterfaceType); ok {
						ifaces[ts.Name.Name] = true
					}
				}
				return true
			})
		}
		ports := map[string]bool{}       // поля пакета с типом-портом: через них хендлер зовёт usecase
		vars := map[string]ast.Expr{}    // значения переменных пакета: операцию объявляют и так (operation)
		local := map[string]bool{}       // имена, которые объявляют или пишут функции пакета
		pkgByName := map[string]string{} // импорты пакета
		for _, file := range parsed {
			maps.Copy(pkgByName, importsByName(file))
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) == len(vs.Values) {
							for j, name := range vs.Names {
								vars[name.Name] = vs.Values[j]
							}
						}
					}
				case *ast.FuncDecl:
					for name := range writes(d) {
						local[name] = true
					}
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if field, ok := n.(*ast.Field); ok {
					if id, ok := field.Type.(*ast.Ident); ok && ifaces[id.Name] {
						for _, name := range field.Names {
							ports[name.Name] = true
						}
					}
				}
				return true
			})
		}
		for name := range local {
			delete(vars, name) // одноимённая переменная функции подставила бы чужую операцию
		}
		for i, file := range parsed {
			byName := importsByName(file)
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				pkg, name := kitCall(call, byName)
				arg, ok := registrars[pkg][name]
				if !ok || len(call.Args) <= arg {
					return true
				}
				lit, open := operation(call.Args[1], vars, pkgByName, map[string]bool{})
				switch {
				case lit == nil:
					c.Errorf("%s: operation %s is neither a huma.Operation literal in the call nor a package var that no function redeclares: archtest reads httpkit.Public only there (rule domain-architecture)", files[i].path, types.ExprString(call.Args[1]))
					return true
				case open:
					return true
				}
				body := handlerBody(call.Args[arg], funcs)
				if body == nil {
					return true // хендлер не из пакета — отказ TestInputsRegisterInDelivery
				}
				id := operationID(lit)
				called := usecaseCalls(body, funcs, ports, map[string]bool{})
				if len(called) == 0 {
					c.Errorf("%s: operation %s under Security calls no usecase method: the handler reaches its domain through the usecase", files[i].path, id)
				}
				for _, name := range called {
					m, ok := root.methods[name]
					walk := policyWalk{root, map[*ast.FuncDecl]bool{}, map[*ast.FuncDecl]bool{}}
					if read := walk.readBeforePolicy(m); ok && read != "" {
						c.Errorf("%s: operation %s under Security calls %s.Usecase.%s, which calls %s before it asks policy: a read before the decision gives away whether the resource exists — policy first, visibility is a parameter of the read (rule domain-architecture)", files[i].path, id, domain, name, read)
					}
					switch {
					case !ok:
						c.Errorf("%s: operation %s under Security calls %s, which *%s.Usecase does not declare", files[i].path, id, name, domain)
					case !walk.asks(m):
						c.Errorf("%s: operation %s under Security calls %s.Usecase.%s, which neither asks policy (uc.d.Policy.Can…) nor only reads the caller's own record, as Me does (uc.d.Repo.GetByID(ctx, caller.UserID)): any signed-in caller would get it (rule domain-architecture)", files[i].path, id, domain, name)
					}
				}
				return true
			})
		}
	}
}

// ctxKeyOutsideWith: ключ личности в ctx пакета identity — только в With и From: помощник с context.WithValue и тем
// же ключом положил бы личность мимо With.
func ctxKeyOutsideWith(c *check, rel string, file *ast.File) {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && (fn.Name.Name == "With" || fn.Name.Name == "From") {
			continue
		}
		used := false
		ast.Inspect(decl, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok && ts.Name.Name == "ctxKey" {
				return false // объявление самого ключа; алиас и тип над ним — использование
			}
			id, ok := n.(*ast.Ident)
			used = used || ok && id.Name == "ctxKey"
			return !used
		})
		if used {
			c.Errorf("%s uses ctxKey outside With and From: only authgate puts the caller into ctx, after it checks the token; "+
				"service work is a usecase method without policy (PurgeLinks) or Trusted", rel)
		}
	}
}

// operation — литерал huma.Operation операции и открыта ли она (httpkit.Public). Литерал — прямо в вызове
// регистрации или в значении переменной пакета (registerOp = httpkit.Public(huma.Operation{…})), под вызовами с одним
// аргументом (<таблица>.Declare, httpkit.Public, authgate.Secured); nil — операции правило не видит: переменная
// функции, параметр, вызов с другими аргументами.
func operation(expr ast.Expr, vars map[string]ast.Expr, byName map[string]string, seen map[string]bool) (*ast.CompositeLit, bool) {
	switch x := expr.(type) {
	case *ast.CompositeLit:
		if isSelector(x.Type, byName, humaPath, "Operation") {
			return x, false
		}
	case *ast.CallExpr:
		if len(x.Args) == 1 {
			lit, open := operation(x.Args[0], vars, byName, seen)
			return lit, open || isSelector(x.Fun, byName, httpkitPath, "Public")
		}
	case *ast.Ident:
		if value, ok := vars[x.Name]; ok && !seen[x.Name] {
			seen[x.Name] = true
			return operation(value, vars, byName, seen)
		}
	}
	return nil, false
}

// operationID — OperationID литерала операции; "?" — его нет.
func operationID(lit *ast.CompositeLit) string {
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok && isIdent(kv.Key, "OperationID") {
			if value, ok := kv.Value.(*ast.BasicLit); ok {
				id, _ := strconv.Unquote(value.Value)
				return id
			}
		}
	}
	return "?"
}

// handlerBody — тело хендлера huma.Register: метод или функция пакета, литерал; nil — не найден.
func handlerBody(expr ast.Expr, funcs map[string]*ast.FuncDecl) *ast.BlockStmt {
	switch h := expr.(type) {
	case *ast.SelectorExpr:
		if fn := funcs[h.Sel.Name]; fn != nil {
			return fn.Body
		}
	case *ast.Ident:
		if fn := funcs[h.Name]; fn != nil {
			return fn.Body
		}
	case *ast.FuncLit:
		return h.Body
	}
	return nil
}

// usecaseCalls — методы usecase, которые тело зовёт через поле-порт (h.uc.List) — само или функциями пакета.
func usecaseCalls(body *ast.BlockStmt, funcs map[string]*ast.FuncDecl, ports, seen map[string]bool) []string {
	if body == nil {
		return nil
	}
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if field, ok := fun.X.(*ast.SelectorExpr); ok && ports[field.Sel.Name] {
				out = append(out, fun.Sel.Name)
				return true
			}
			name = fun.Sel.Name
		case *ast.Ident:
			name = fun.Name
		}
		if fn := funcs[name]; fn != nil && !seen[name] {
			seen[name] = true
			out = append(out, usecaseCalls(fn.Body, funcs, ports, seen)...)
		}
		return true
	})
	return out
}

// policyRoot — что правило policy знает о корне домена: методы Usecase и функции корня, импорты их файлов,
// интерфейсы корня — порты, порты policy (тип поля Policy и интерфейсы из одних Can…) и поля зависимостей
// (структур с fx.In) с ними.
type policyRoot struct {
	methods, funcs map[string]*ast.FuncDecl
	byName         map[*ast.FuncDecl]map[string]string
	ports          map[string]bool // интерфейсы корня
	policies       map[string]bool // интерфейсы корня — порты policy
	portFields     map[string]bool
	policyFields   map[string]bool
}

func loadPolicyRoot(c *check, domain string) policyRoot {
	c.T.Helper()
	root := policyRoot{
		methods: map[string]*ast.FuncDecl{}, funcs: map[string]*ast.FuncDecl{}, byName: map[*ast.FuncDecl]map[string]string{},
		ports: map[string]bool{}, policies: map[string]bool{}, portFields: map[string]bool{}, policyFields: map[string]bool{},
	}
	var deps []*ast.StructType
	for _, file := range rootFiles(c, domain) {
		byName := importsByName(file)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				root.byName[fn] = byName
				switch {
				case fn.Recv == nil:
					root.funcs[fn.Name.Name] = fn
				case receiverName(fn) == "Usecase":
					root.methods[fn.Name.Name] = fn
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				switch typ := ts.Type.(type) {
				case *ast.InterfaceType:
					root.ports[ts.Name.Name], root.policies[ts.Name.Name] = true, onlyCan(typ)
				case *ast.StructType:
					if embedsFxIn(typ, byName) {
						deps = append(deps, typ)
					}
				}
			}
			return true
		})
	}
	for _, st := range deps {
		for _, field := range st.Fields.List {
			id, ok := field.Type.(*ast.Ident)
			if !ok || !root.ports[id.Name] {
				continue
			}
			for _, name := range field.Names {
				if name.Name == "Policy" {
					root.policies[id.Name] = true
				}
			}
		}
	}
	for _, st := range deps {
		for _, field := range st.Fields.List {
			if id, ok := field.Type.(*ast.Ident); ok && root.ports[id.Name] {
				for _, name := range field.Names {
					root.portFields[name.Name], root.policyFields[name.Name] = true, root.policies[id.Name]
				}
			}
		}
	}
	return root
}

// embedsFxIn — структура встраивает fx.In: это зависимости, которые собирает граф.
func embedsFxIn(st *ast.StructType, byName map[string]string) bool {
	return slices.ContainsFunc(st.Fields.List, func(f *ast.Field) bool {
		return len(f.Names) == 0 && isSelector(f.Type, byName, "go.uber.org/fx", "In")
	})
}

// onlyCan — у интерфейса есть методы, и все они Can…: это порт policy.
func onlyCan(iface *ast.InterfaceType) bool {
	methods := iface.Methods.List
	return len(methods) > 0 && !slices.ContainsFunc(methods, func(m *ast.Field) bool {
		return len(m.Names) != 1 || !strings.HasPrefix(m.Names[0].Name, "Can")
	})
}

const identityPath = module + "/internal/platform/identity"

// policyWalk — обход методов и функций корня от метода за операцией; asked и yielded — ответы по функциям (на время
// обхода — false: цикл вызовов).
type policyWalk struct {
	policyRoot
	asked, yielded map[*ast.FuncDecl]bool
}

// asks — метод спрашивает policy (Can… порта policy: поля зависимостей или параметра) или передаёт порту
// зависимостей (uc.d.Repo.GetByID) ключ вызывающего (callerScope); сам или своим методом, функцией корня. Вызов
// лога, кита или функции пакета с UserID — не ключ: операция по-прежнему отдала бы чужое.
func (w policyWalk) asks(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	if done, ok := w.asked[fn]; ok {
		return done
	}
	w.asked[fn] = false
	params, sc := w.params(fn), w.scope(fn)
	asked := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if x, ok := n.(*ast.CallExpr); ok {
			sel, isSel := x.Fun.(*ast.SelectorExpr)
			switch {
			case isSel && strings.HasPrefix(sel.Sel.Name, "Can") && w.port(sel.X, params, true),
				isSel && w.port(sel.X, params, false) && slices.ContainsFunc(x.Args, sc.key),
				w.asks(sc.callee(x)):
				asked = true
			}
		}
		return !asked
	})
	w.asked[fn] = asked
	return asked
}

// asksCan — функция спрашивает порт policy (Can…) сама или своим методом, функцией корня.
func (w policyWalk) asksCan(fn *ast.FuncDecl, seen map[*ast.FuncDecl]bool) bool {
	if fn == nil || fn.Body == nil || seen[fn] {
		return false
	}
	seen[fn] = true
	params, sc := w.params(fn), w.scope(fn)
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			sel, isSel := call.Fun.(*ast.SelectorExpr)
			found = isSel && strings.HasPrefix(sel.Sel.Name, "Can") && w.port(sel.X, params, true) || w.asksCan(sc.callee(call), seen)
		}
		return !found
	})
	return found
}

// readBeforePolicy — вызов порта зависимостей, который метод делает раньше первого вопроса policy (самого или своим
// методом, функцией корня); "" — такого нет или метод policy не спрашивает (читает своё, как Me). Вызов с ключом
// вызывающего в аргументах — чтение своего: квоте по своим записям policy отвечает после него.
func (w policyWalk) readBeforePolicy(fn *ast.FuncDecl) string {
	if fn == nil || fn.Body == nil {
		return ""
	}
	params, sc := w.params(fn), w.scope(fn)
	read, asked := "", false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && !asked && read == "" {
			sel, isSel := call.Fun.(*ast.SelectorExpr)
			switch {
			case isSel && strings.HasPrefix(sel.Sel.Name, "Can") && w.port(sel.X, params, true),
				w.asksCan(sc.callee(call), map[*ast.FuncDecl]bool{}):
				asked = true
			case isSel && w.port(sel.X, params, false) && !w.port(sel.X, params, true) && !slices.ContainsFunc(call.Args, sc.key):
				read = types.ExprString(call.Fun)
			}
		}
		return !asked && read == ""
	})
	if read == "" || !w.asksCan(fn, map[*ast.FuncDecl]bool{}) {
		return ""
	}
	return read
}

// yields — каждый return функции отдаёт первым результатом ключ вызывающего (return caller.UserID, nil) или "" с
// ошибкой (return "", err): функция, которая на одной из веток отдаёт ID входа, ключ не даёт.
func (w policyWalk) yields(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	if done, ok := w.yielded[fn]; ok {
		return done
	}
	w.yielded[fn] = false
	sc, returns, yields := w.scope(fn), 0, true
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false // return литерала — не return функции
		case *ast.ReturnStmt:
			returns++
			yields = yields && len(x.Results) > 0 && (sc.key(x.Results[0]) || refused(x.Results))
		}
		return yields
	})
	w.yielded[fn] = returns > 0 && yields
	return w.yielded[fn]
}

// refused — return "", err: пустой ключ вместе с ошибкой.
func refused(results []ast.Expr) bool {
	lit, ok := results[0].(*ast.BasicLit)
	return ok && lit.Value == `""` && len(results) > 1 && !isIdent(results[len(results)-1], "nil")
}

// callerScope — переменные функции fn: вызывающий — переменная, каждая запись которой — identity.From(ctx); ключ —
// переменная, каждая запись которой — ключ (key). Одна запись не ключа снимает метку: target := caller.UserID;
// if id != "" { target = id } — не ключ.
type callerScope struct {
	w             policyWalk
	fn            *ast.FuncDecl
	vars          map[string][]ast.Expr // writes(fn)
	callers, keys map[string]bool
}

// scope — callerScope функции fn: ключи — наибольшее множество, у каждой переменной которого все записи — ключ.
func (w policyWalk) scope(fn *ast.FuncDecl) callerScope {
	sc := callerScope{w: w, fn: fn, vars: writes(fn), callers: map[string]bool{}, keys: map[string]bool{}}
	for name, values := range sc.vars {
		sc.callers[name] = !slices.ContainsFunc(values, func(v ast.Expr) bool {
			call, ok := v.(*ast.CallExpr)
			return !ok || !isSelector(call.Fun, w.byName[fn], identityPath, "From")
		})
		sc.keys[name] = true
	}
	for changed := true; changed; {
		changed = false
		for name, values := range sc.vars {
			if sc.keys[name] && slices.ContainsFunc(values, func(v ast.Expr) bool { return v == nil || !sc.key(v) }) {
				sc.keys[name], changed = false, true
			}
		}
	}
	return sc
}

// key — выражение — ключ вызывающего: <вызывающий>.UserID, переменная-ключ или вызов своего метода или функции
// корня, которая отдаёт ключ (uc.callerID(ctx)).
func (sc callerScope) key(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return sc.keys[e.Name]
	case *ast.SelectorExpr:
		id, ok := e.X.(*ast.Ident)
		return ok && e.Sel.Name == "UserID" && sc.callers[id.Name]
	case *ast.CallExpr:
		return sc.w.yields(sc.callee(e))
	}
	return false
}

// writes — записи переменных в функции (и её литералах): имя → значения. Значение кортежа (id, err :=
// uc.callerID(ctx)) — вызов у первой переменной; nil — значения не видно: параметр, вторая переменная кортежа, range,
// var без значения, x++, взятый адрес (&x). Запись поля или элемента (x.f = v) — запись x.
func writes(fn ast.Node) map[string][]ast.Expr {
	out := map[string][]ast.Expr{}
	write := func(lhs, value ast.Expr) {
		for lhs != nil {
			switch x := lhs.(type) {
			case *ast.Ident:
				out[x.Name] = append(out[x.Name], value)
				return
			case *ast.SelectorExpr:
				lhs = x.X
			case *ast.IndexExpr:
				lhs = x.X
			case *ast.StarExpr:
				lhs = x.X
			case *ast.ParenExpr:
				lhs = x.X
			default:
				return
			}
		}
	}
	tuple := func(names, values []ast.Expr) {
		for i, name := range names {
			switch {
			case len(names) == len(values):
				write(name, values[i])
			case i == 0 && len(values) == 1:
				write(name, values[0])
			default:
				write(name, nil)
			}
		}
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			for _, name := range x.Names {
				write(name, nil)
			}
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(x.Names))
			for i, name := range x.Names {
				names[i] = name
			}
			tuple(names, x.Values)
		case *ast.AssignStmt:
			tuple(x.Lhs, x.Rhs)
		case *ast.RangeStmt:
			write(x.Key, nil)
			write(x.Value, nil)
		case *ast.IncDecStmt:
			write(x.X, nil)
		case *ast.UnaryExpr:
			if x.Op == token.AND {
				write(x.X, nil)
			}
		}
		return true
	})
	return out
}

// callee — свой метод (на получателе fn) или функция корня, которую зовёт вызов; nil — чужой вызов или имя, которое
// функция объявила сама: литерал функции под именем функции корня — не она.
func (sc callerScope) callee(call *ast.CallExpr) *ast.FuncDecl {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if len(sc.vars[fun.Name]) == 0 {
			return sc.w.funcs[fun.Name]
		}
	case *ast.SelectorExpr:
		if recv := sc.fn.Recv; recv != nil && len(recv.List[0].Names) > 0 && isIdent(fun.X, recv.List[0].Names[0].Name) {
			return sc.w.methods[fun.Sel.Name]
		}
	}
	return nil
}

// port — выражение — порт корня (policy — порт policy): поле зависимостей (uc.d.Repo) или параметр функции.
func (w policyWalk) port(expr ast.Expr, params map[string]string, policy bool) bool {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		return policy && w.policyFields[x.Sel.Name] || !policy && w.portFields[x.Sel.Name]
	case *ast.Ident:
		return policy && w.policies[params[x.Name]] || !policy && w.ports[params[x.Name]]
	}
	return false
}

// params — параметры функции с типом корня: имя → тип.
func (w policyWalk) params(fn *ast.FuncDecl) map[string]string {
	out := map[string]string{}
	for _, field := range fn.Type.Params.List {
		if id, ok := field.Type.(*ast.Ident); ok {
			for _, name := range field.Names {
				out[name.Name] = id.Name
			}
		}
	}
	return out
}

// TestIdentityWithOnlyInAuthgate: личность вызывающего кладёт в ctx только authgate после проверки токена; identity.With
// в коде домена, входа или cmd/ — и помощник над ним или над ключом ctxKey в самом пакете identity, и вызов под
// dot-импортом — подставил бы чужую личность мимо cookie, и policy решала бы за неё. Тесты и internal/testkit
// (identitytest) — можно.
func TestIdentityWithOnlyInAuthgate(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func identityWithOnlyInAuthgate(c *check) {
	for _, f := range c.files {
		own := f.pkg == strings.TrimPrefix(identityPath, module+"/")
		if f.test || under(f.pkg, "internal/platform/authgate") || under(f.pkg, "internal/testkit") ||
			!own && !slices.Contains(f.imports, identityPath) {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		if own {
			ctxKeyOutsideWith(c, f.path, file)
		}
		bare := own || byName["."] == identityPath // With без селектора — identity.With
		named := map[*ast.Ident]bool{}             // имена объявлений, полей и селекторов: не вызов identity.With
		ast.Inspect(file, func(n ast.Node) bool {
			var with bool
			switch n := n.(type) {
			case *ast.FuncDecl:
				named[n.Name] = true
			case *ast.Field:
				for _, name := range n.Names {
					named[name] = true
				}
			case *ast.KeyValueExpr:
				if id, ok := n.Key.(*ast.Ident); ok {
					named[id] = true
				}
			case *ast.SelectorExpr:
				named[n.Sel] = true
				with = isSelector(n, byName, identityPath, "With")
			case *ast.Ident:
				with = bare && n.Name == "With" && !named[n]
			}
			if with {
				c.Errorf("%s calls identity.With: only authgate puts the caller into ctx, after it checks the token; "+
					"service work is a usecase method without policy (PurgeLinks) or Trusted", f.path)
			}
			return true
		})
	}
}
