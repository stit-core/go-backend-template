package archtest

import (
	"go/ast"
	"go/types"
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

// TestInputsRegisterInDelivery: операцию HTTP регистрирует huma.Register в delivery/http домена, с хендлером
// оттуда же, подписку на события — natskit в delivery/nats. Правила входа читают эти пакеты: операция или
// подписка в platform, module/ или cmd/ обошла бы их.
func TestInputsRegisterInDelivery(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if f.test || !slices.Contains(f.imports, humaPath) && !slices.Contains(f.imports, natskitPath) {
			continue
		}
		file := parseGo(t, f.path)
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); imp.Name != nil && imp.Name.Name == "." && (p == humaPath || p == natskitPath) {
				t.Errorf("%s dot-imports %s: archtest would not see the operations and subscriptions it registers", f.path, p)
			}
		}
		byName := importsByName(file)
		_, sub, inDomain := splitDomain(f.pkg)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch pkg, name := kitCall(call, byName); {
			case pkg == humaPath && slices.Contains(humaShortcuts, name):
				t.Errorf("%s registers an operation with huma.%s: an operation is registered by huma.Register with its error table", f.path, name)
			case pkg == humaPath && name == "Register" && (!inDomain || sub != "delivery/http"):
				t.Errorf("%s registers an HTTP operation: operations live in internal/domain/<d>/delivery/http, where the rules of error tables and policy read them", f.path)
			case pkg == humaPath && name == "Register" && len(call.Args) == 3 && importedName(call.Args[2], byName):
				t.Errorf("%s registers handler %s declared outside delivery/http: the rules of error tables and policy read the handlers of that package", f.path, types.ExprString(call.Args[2]))
			case pkg == natskitPath && strings.HasPrefix(name, "Subscribe") && (!inDomain || sub != "delivery/nats"):
				t.Errorf("%s subscribes to events: subscriptions live in internal/domain/<d>/delivery/nats, where the consumer's contract test checks them", f.path)
			}
			return true
		})
	}
}

// kitCall — пакет и имя вызова <пакет>.<имя>(…), и с параметрами типа (huma.Register[I, O]); "" — не такой вызов.
func kitCall(call *ast.CallExpr, byName map[string]string) (string, string) {
	fun := call.Fun
	switch x := fun.(type) {
	case *ast.IndexExpr:
		fun = x.X
	case *ast.IndexListExpr:
		fun = x.X
	}
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return byName[id.Name], sel.Sel.Name
}

// importedName — выражение <пакет>.<имя>: имя объявлено в другом пакете.
func importedName(expr ast.Expr, byName map[string]string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && byName[id.Name] != ""
}

// deliveryKits — пакеты golibinfra, которые видит вход: транспорт, лог и helpers без I/O. Хранилища, клиенты,
// хэшер, аудит и метрики домена — за портами usecase.
var deliveryKits = []string{"httpkit", "natskit", "grpckit", "logkit", "clockkit", "idkit", "ptrkit", "mapkit", "tzkit"}

// TestDeliveryReachesUsecaseOnly: вход домена зовёт usecase своего домена портом пакета. Из корня он берёт
// данные — сущности, DTO, ошибки, — а не порты, usecase, Trusted и их зависимости, и не держит хранилищ,
// клиентов и драйверов: иначе операция прошла бы мимо usecase и его policy.
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
					for _, field := range typ.Fields.List {
						if len(field.Names) == 0 && isSelector(field.Type, byName, "go.uber.org/fx", "In") {
							services[ts.Name.Name] = true
						}
					}
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
// домена — identity-closure над *<домен>.Usecase в его module/. Порт, привязанный к репозиторию, Trusted или
// хранилищу кита, отдал бы входу операции мимо usecase и policy.
func TestDeliveryPortsBoundToUsecase(t *testing.T) {
	t.Parallel()
	files := goFiles(t)
	ports := map[string]map[string]bool{} // путь импорта пакета delivery/<транспорт> → его интерфейсы
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
				if _, ok := ts.Type.(*ast.InterfaceType); ok {
					ports[pkg][ts.Name.Name] = true
				}
			}
			return true
		})
	}
	for _, f := range files {
		if f.test || !slices.ContainsFunc(f.imports, func(imp string) bool { return ports[imp] != nil }) {
			continue
		}
		file := parseGo(t, f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			fn, body := funcParts(n)
			if fn == nil || fn.Results == nil {
				return true
			}
			sel, ok := fn.Results.List[0].Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || !ports[byName[id.Name]][sel.Sel.Name] {
				return true
			}
			domain, _, _ := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/"))
			if f.pkg != "internal/domain/"+domain+"/module" || !usecaseClosure(fn, body, byName, module+"/internal/domain/"+domain) {
				t.Errorf("%s binds %s.%s otherwise than func(uc *%s.Usecase) %s.%s { return uc } in internal/domain/%s/module: an input reaches its domain through the usecase and its policy", f.path, id.Name, sel.Sel.Name, domain, id.Name, sel.Sel.Name, domain)
			}
			return true
		})
	}
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
