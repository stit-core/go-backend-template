package archtest

import (
	"go/ast"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	seen := map[string]bool{}
	for _, f := range goFiles(t) {
		_, sub, ok := splitDomain(f.pkg)
		if !ok || !under(sub, "delivery") || seen[f.pkg] {
			continue
		}
		seen[f.pkg] = true
		if parts := strings.Split(sub, "/"); len(parts) != 2 && (len(parts) != 3 || parts[2] != "mocks") {
			t.Errorf("%s: a domain input is one package delivery/<transport> with its mocks — the rules of HTTP types, error tables, policy and event contracts read that package", f.pkg)
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
	files := goFiles(t)
	declared := map[string]map[string]bool{} // пакет delivery/http → «m:<метод>», «f:<функция>»
	for _, f := range files {
		if _, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/http" && !f.test {
			if declared[f.pkg] == nil {
				declared[f.pkg] = map[string]bool{}
			}
			for _, decl := range parseGo(t, f.path).Decls {
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
		file := parseGo(t, f.path)
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); imp.Name != nil && imp.Name.Name == "." && (p == humaPath || p == ssePath || p == natskitPath) {
				t.Errorf("%s dot-imports %s: archtest would not see the operations and subscriptions it registers", f.path, p)
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
					t.Errorf("%s registers an operation with huma.%s: an operation is registered by huma.Register with its error table", f.path, name)
				case registrar && !inHTTP:
					t.Errorf("%s registers an HTTP operation (%s): operations live in internal/domain/<d>/delivery/http, where the rules of error tables and policy read them", f.path, types.ExprString(x))
				case registrar && !called[x]:
					t.Errorf("%s uses %s as a value: an operation is registered by a direct call, which the rules of error tables and policy read", f.path, types.ExprString(x))
				case pkg == natskitPath && strings.HasPrefix(name, "Subscribe") && !inNATS:
					t.Errorf("%s subscribes to events (natskit.%s): subscriptions live in internal/domain/<d>/delivery/nats, where the consumer's contract test checks them", f.path, name)
				}
			case *ast.CallExpr:
				pkg, name := kitCall(x, byName)
				if arg, ok := registrars[pkg][name]; ok && inHTTP && len(x.Args) > arg && !localHandler(x.Args[arg], byName, declared[f.pkg]) {
					t.Errorf("%s registers handler %s, which is not a method, function or literal of delivery/http: the rules of error tables and policy read only those", f.path, types.ExprString(x.Args[arg]))
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
// держит присутствие теста и вызова Subscriptions; что сверка верна, держит сам тест.
func TestSubscriptionsHaveContractTest(t *testing.T) {
	t.Parallel()
	subscribers := map[string]bool{} // пакеты delivery/nats с подпиской
	for _, f := range goFiles(t) {
		if _, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/nats" && !f.test && slices.Contains(f.imports, natskitPath) {
			subscribers[f.pkg] = true
		}
	}
	for pkg := range subscribers {
		contract := pkg + "/contract_test.go"
		if _, err := os.Stat(filepath.Join(repoRoot(t), contract)); err != nil {
			t.Errorf("%s subscribes to events without %s: the contract test delivers every subscription an event of the provider's usecase (rule events-and-messaging)", pkg, contract)
			continue
		}
		checked := false
		ast.Inspect(parseGo(t, contract), func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			checked = checked || ok && sel.Sel.Name == "Subscriptions"
			return !checked
		})
		if !checked {
			t.Errorf("%s does not check the subscriptions of Register against the provider events it delivers (bus.Subscriptions()): a subscription without an event would pass", contract)
		}
	}
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
	services := map[string]map[string]bool{} // домен → имена корня, за которыми зависимости
	for _, f := range goFiles(t) {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || f.test || !under(sub, "delivery") {
			continue
		}
		for _, imp := range f.imports {
			kit, isKit := strings.CutPrefix(imp, golibinfra+"/")
			if isKit && !slices.Contains(deliveryKits, kit) || under(imp, "database/sql") || under(imp, "github.com/jackc/pgx") {
				t.Errorf("%s imports %s: delivery reaches the domain through its usecase port — stores, clients and drivers sit behind the usecase's ports", f.path, imp)
			}
		}
		if services[domain] == nil {
			services[domain] = rootServices(t, domain)
		}
		root := module + "/internal/domain/" + domain
		file := parseGo(t, f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && byName[id.Name] == root && services[domain][sel.Sel.Name] {
				t.Errorf("%s uses %s.%s: delivery takes data types from its domain root and calls the domain through its Usecase port — ports, the usecase, Trusted and their dependencies are wired in module/", f.path, id.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// rootServices — имена корня домена, за которыми зависимости, а не данные: интерфейсы (порты), структуры с
// fx.In, типы с полем такого типа (Usecase, Trusted) и функции с ними в сигнатуре (NewUsecase, NewPolicy).
func rootServices(t *testing.T, domain string) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "domain", domain, "*.go"))
	require.NoError(t, err)
	var (
		specs []*ast.TypeSpec
		funcs []*ast.FuncDecl
	)
	services := map[string]bool{}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		file := parseGo(t, "internal/domain/"+domain+"/"+filepath.Base(p))
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
				switch typ := ts.Type.(type) {
				case *ast.InterfaceType:
					services[ts.Name.Name] = true
				case *ast.StructType:
					services[ts.Name.Name] = embedsFxIn(typ, byName)
				}
			}
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
// алиас, параметр, экспортируемый тип-функция пакета входа — отказ: порт над репозиторием, Trusted или хранилищем
// кита отдал бы входу операции мимо usecase и policy.
func TestDeliveryPortsBoundToUsecase(t *testing.T) {
	t.Parallel()
	files := goFiles(t)
	ports := map[string]map[string]bool{} // путь импорта пакета delivery/<транспорт> → его порты
	for _, f := range files {
		if _, sub, ok := splitDomain(f.pkg); !ok || f.test || !under(sub, "delivery") {
			continue
		}
		pkg := module + "/" + f.pkg
		if ports[pkg] == nil {
			ports[pkg] = map[string]bool{}
		}
		ast.Inspect(parseGo(t, f.path), func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				switch ts.Type.(type) {
				case *ast.InterfaceType:
					ports[pkg][ts.Name.Name] = true
				case *ast.FuncType:
					ports[pkg][ts.Name.Name] = ast.IsExported(ts.Name.Name)
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
		file := parseGo(t, f.path)
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
			if sel, ok := n.(*ast.SelectorExpr); ok && !bound[sel] {
				if domain, ok := portDomain(sel, byName, ports); ok {
					port := types.ExprString(sel)
					t.Errorf("%s uses %s otherwise than as the result of func(uc *%s.Usecase) %s { return uc } in internal/domain/%s/module: an input port gets the usecase of its domain and its policy", f.path, port, domain, port, domain)
				}
			}
			return true
		})
	}
}

// portDomain — домен порта входа, если выражение — <пакет delivery/<транспорт>>.<порт>.
func portDomain(expr ast.Expr, byName map[string]string, ports map[string]map[string]bool) (string, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || !ports[byName[id.Name]][sel.Sel.Name] {
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
// (…Policy.Can…) — сам или своим методом; без policy — только чтение самого вызывающего: его UserID из
// identity.From — аргумент вызова порта зависимостей (uc.d.Repo.GetByID — Me), а не лога, кита или функции пакета.
// Иначе операция отдала бы ресурс любому вошедшему.
func TestSecuredOperationsAskPolicy(t *testing.T) {
	t.Parallel()
	byDomain := map[string][]goFile{} // домен → не-тестовые файлы delivery/http
	for _, f := range goFiles(t) {
		if domain, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/http" && !f.test {
			byDomain[domain] = append(byDomain[domain], f)
		}
	}
	for domain, files := range byDomain {
		root := loadPolicyRoot(t, domain)
		parsed := make([]*ast.File, len(files))
		ifaces := map[string]bool{}
		funcs := map[string]*ast.FuncDecl{} // методы и функции пакета
		for i, f := range files {
			parsed[i] = parseGo(t, f.path)
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
		ports := map[string]bool{}        // поля пакета с типом-портом: через них хендлер зовёт usecase
		values := map[string][]ast.Expr{} // значения переменных пакета: операцию объявляют и переменной
		pkgByName := map[string]string{}  // импорты пакета
		for _, file := range parsed {
			maps.Copy(pkgByName, importsByName(file))
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.ValueSpec:
					for j, name := range x.Names {
						if j < len(x.Values) {
							values[name.Name] = append(values[name.Name], x.Values[j])
						}
					}
				case *ast.AssignStmt:
					for j, lhs := range x.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && len(x.Lhs) == len(x.Rhs) {
							values[id.Name] = append(values[id.Name], x.Rhs[j])
						}
					}
				}
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
				op := operationExprs(call.Args[1], values)
				if public(op, pkgByName) {
					return true
				}
				body := handlerBody(call.Args[arg], funcs)
				if body == nil {
					return true // хендлер не из пакета — отказ TestInputsRegisterInDelivery
				}
				id := operationID(op)
				called := usecaseCalls(body, funcs, ports, map[string]bool{})
				if len(called) == 0 {
					t.Errorf("%s: operation %s under Security calls no usecase method: the handler reaches its domain through the usecase", files[i].path, id)
				}
				for _, name := range called {
					m, ok := root.methods[name]
					switch {
					case !ok:
						t.Errorf("%s: operation %s under Security calls %s, which *%s.Usecase does not declare", files[i].path, id, name, domain)
					case !(policyWalk{root, map[*ast.FuncDecl]bool{}, map[*ast.FuncDecl]bool{}}).asks(m):
						t.Errorf("%s: operation %s under Security calls %s.Usecase.%s, which neither asks policy (uc.d.Policy.Can…) nor passes the caller's UserID to a port of its dependencies (uc.d.Repo…): any signed-in caller would get it (rule domain-architecture)", files[i].path, id, domain, name)
					}
				}
				return true
			})
		}
	}
}

// operationExprs — выражение операции в huma.Register и значения переменных пакета в нём: операцию объявляют и
// переменной (registerOp = httpkit.Public(huma.Operation{…})).
func operationExprs(op ast.Expr, values map[string][]ast.Expr) []ast.Expr {
	out, seen := []ast.Expr{op}, map[string]bool{}
	for i := 0; i < len(out); i++ {
		ast.Inspect(out[i], func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && !seen[id.Name] {
				seen[id.Name] = true
				out = append(out, values[id.Name]...)
			}
			return true
		})
	}
	return out
}

// public — операция открыта пометкой httpkit.Public.
func public(op []ast.Expr, byName map[string]string) bool {
	return slices.ContainsFunc(op, func(e ast.Expr) bool {
		found := false
		ast.Inspect(e, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isSelector(call.Fun, byName, httpkitPath, "Public") {
				found = true
			}
			return !found
		})
		return found
	})
}

// operationID — OperationID литерала операции; "?" — его нет.
func operationID(op []ast.Expr) string {
	id := "?"
	for _, e := range op {
		ast.Inspect(e, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok && isIdent(kv.Key, "OperationID") {
				if lit, ok := kv.Value.(*ast.BasicLit); ok {
					id, _ = strconv.Unquote(lit.Value)
				}
			}
			return true
		})
	}
	return id
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

func loadPolicyRoot(t *testing.T, domain string) policyRoot {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "domain", domain, "*.go"))
	require.NoError(t, err)
	root := policyRoot{
		methods: map[string]*ast.FuncDecl{}, funcs: map[string]*ast.FuncDecl{}, byName: map[*ast.FuncDecl]map[string]string{},
		ports: map[string]bool{}, policies: map[string]bool{}, portFields: map[string]bool{}, policyFields: map[string]bool{},
	}
	var deps []*ast.StructType
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		file := parseGo(t, "internal/domain/"+domain+"/"+filepath.Base(p))
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
// зависимостей (uc.d.Repo.GetByID) ключ вызывающего — UserID из identity.From или переменную с ним, в том числе от
// своего метода или функции корня (uc.callerID); сам или своим методом, функцией корня. Вызов лога, кита или
// функции пакета с UserID — не ключ: операция по-прежнему отдала бы чужое.
func (w policyWalk) asks(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	if done, ok := w.asked[fn]; ok {
		return done
	}
	w.asked[fn] = false
	params, sc := w.params(fn), callerScope{callers: map[string]bool{}, keys: map[string]bool{}}
	asked := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			w.bind(fn, x, sc)
		case *ast.CallExpr:
			sel, isSel := x.Fun.(*ast.SelectorExpr)
			switch {
			case isSel && strings.HasPrefix(sel.Sel.Name, "Can") && w.port(sel.X, params, true),
				isSel && w.port(sel.X, params, false) && slices.ContainsFunc(x.Args, sc.key),
				w.asks(w.callee(fn, x)):
				asked = true
			}
		}
		return !asked
	})
	w.asked[fn] = asked
	return asked
}

// yields — функция отдаёт первым результатом ключ вызывающего: return caller.UserID, nil.
func (w policyWalk) yields(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	if done, ok := w.yielded[fn]; ok {
		return done
	}
	w.yielded[fn] = false
	sc := callerScope{callers: map[string]bool{}, keys: map[string]bool{}}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			w.bind(fn, x, sc)
		case *ast.ReturnStmt:
			found = found || len(x.Results) > 0 && sc.key(x.Results[0])
		}
		return !found
	})
	w.yielded[fn] = found
	return found
}

// callerScope — переменные функции: вызывающий из identity.From и ключи — его UserID.
type callerScope struct{ callers, keys map[string]bool }

func (sc callerScope) key(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return sc.keys[e.Name]
	case *ast.SelectorExpr:
		id, ok := e.X.(*ast.Ident)
		return ok && e.Sel.Name == "UserID" && sc.callers[id.Name]
	}
	return false
}

// bind — x := identity.From(ctx) даёт вызывающего, x := <ключ> и x := <своя функция, которая отдаёт ключ> — ключ.
func (w policyWalk) bind(fn *ast.FuncDecl, x *ast.AssignStmt, sc callerScope) {
	id, ok := x.Lhs[0].(*ast.Ident)
	if !ok || id.Name == "_" || len(x.Rhs) != 1 {
		return
	}
	call, isCall := x.Rhs[0].(*ast.CallExpr)
	switch {
	case isCall && isSelector(call.Fun, w.byName[fn], identityPath, "From"):
		sc.callers[id.Name] = true
	case isCall && w.yields(w.callee(fn, call)), sc.key(x.Rhs[0]):
		sc.keys[id.Name] = true
	}
}

// callee — свой метод (на получателе fn) или функция корня, которую зовёт вызов; nil — чужой вызов.
func (w policyWalk) callee(fn *ast.FuncDecl, call *ast.CallExpr) *ast.FuncDecl {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return w.funcs[fun.Name]
	case *ast.SelectorExpr:
		if fn.Recv != nil && len(fn.Recv.List[0].Names) > 0 && isIdent(fun.X, fn.Recv.List[0].Names[0].Name) {
			return w.methods[fun.Sel.Name]
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
