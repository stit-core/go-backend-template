// Package archtest машинно держит обвязку и канон доменов (rule domain-architecture) для любого
// каталога internal/domain/*. Раскладку — каталоги internal/, импорты между доменами и слоями, чистоту
// корня домена и чистых слоёв, настенные часы, порты между доменами без identity-closure, свои таблицы —
// держит archkit (golibinfra) вызовом TestLayout: правила едут версией кита, настройки сервиса — в
// layout_settings_test.go. Здесь — правила шаблона: Trusted и таблицы ошибок между доменами, привязка
// портов без fx.As, префикс, уникальность и envelope HTTP-типов, таблица ошибок домена в HTTP-контракте,
// владение subject-литералами, тесты без утверждений, заголовок сгенерированного только в выводе
// генераторов, ограничение сборки — только integration у тестов; а ещё подключение доменов в cmd/api,
// ссылки прозы на make-цели и пути, //nolint только включённых линтеров и без директив линтеров мимо
// него, пути и версия обвязки шаблона (.template), её перенос в форк (make
// template-update), маркеры конфликтов, пути доменов идентичности и процедура их prune, канонический
// репо шаблона, валидатор stack-артефактов, хуки guard-bash, commit-msg, pre-commit и pre-push, подключение
// git-хуков (make setup, status, doctor), путь образа релиза и самозащита make check от флагов, переменных и
// окружения вызывающего. На archtest и archkit //nolint не
// действует.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const module = "git.stit.tech/stit-core/go-backend-template"

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func domains(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot(t), "internal", "domain"))
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	require.NotEmpty(t, out)
	return out
}

func parseImports(t *testing.T, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	require.NoError(t, err)
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)
		out = append(out, p)
	}
	return out
}

// goFile — Go-файл репо: путь и каталог пакета от корня репо (через "/") и его импорты.
type goFile struct {
	path    string
	pkg     string
	test    bool
	imports []string
}

// goFiles — все .go-файлы репо вне скрытых каталогов (.git, .cache, worktree'ы в .claude).
func goFiles(t *testing.T) []goFile {
	t.Helper()
	root := repoRoot(t)
	var files []goFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p != root && strings.HasPrefix(d.Name(), "."):
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(p, ".go"):
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files = append(files, goFile{
			path:    rel,
			pkg:     path.Dir(rel),
			test:    strings.HasSuffix(rel, "_test.go"),
			imports: parseImports(t, p),
		})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, files)
	return files
}

func parseGo(t *testing.T, rel string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repoRoot(t), rel), nil, 0)
	require.NoError(t, err)
	return f
}

// importsByName — пакеты файла по имени, под которым файл их видит: алиас, "." или элемент пути
// (у модуля v2+ — элемент перед версией: github.com/danielgtaylor/huma/v2 — huma).
func importsByName(f *ast.File) map[string]string {
	byName := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		switch {
		case imp.Name != nil:
			name = imp.Name.Name
		case majorVersion.MatchString(name):
			name = path.Base(path.Dir(p))
		}
		byName[name] = p
	}
	return byName
}

var majorVersion = regexp.MustCompile(`^v([2-9]|[1-9][0-9]+)$`)

// splitDomain разбирает путь пакета от корня репо на домен и подпакет:
// internal/domain/user/infra/postgres → "user", "infra/postgres", true.
func splitDomain(pkg string) (string, string, bool) {
	rest, ok := strings.CutPrefix(pkg, "internal/domain/")
	domain, sub, _ := strings.Cut(rest, "/")
	return domain, sub, ok
}

// under — путь p совпадает с dir или лежит внутри него.
func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// TestHTTPTypes: huma регистрирует схемы по имени Go-типа на весь API, и одноимённые типы двух
// доменов роняют старт паникой — поэтому у типа delivery/http префикс домена и уникальное имя;
// тело ответа — envelope {data}. Handler и Usecase — канон пакета, в схемы не попадают.
func TestHTTPTypes(t *testing.T) {
	t.Parallel()
	seen := map[string]string{} // имя типа → файл
	for _, f := range goFiles(t) {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "delivery/http" || f.test {
			continue
		}
		prefix := exportedName(domain)
		for _, decl := range parseGo(t, f.path).Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				name := ts.Name.Name
				if !ast.IsExported(name) || name == "Handler" || name == "Usecase" {
					continue
				}
				if !strings.HasPrefix(name, prefix) {
					t.Errorf("%s declares %s: an HTTP type starts with the domain prefix %s", f.path, name, prefix)
				}
				if prev, dup := seen[name]; dup {
					t.Errorf("type %s is declared in both %s and %s: huma names schemas by Go type", name, prev, f.path)
				}
				seen[name] = f.path
				if body := structField(ts.Type, "Body"); strings.HasSuffix(name, "Output") && body != nil && structField(body.Type, "Data") == nil {
					t.Errorf("%s declares %s with a Body without Data: a response body is the envelope {data}", f.path, name)
				}
			}
		}
	}
}

// exportedName — имя домена в CamelCase: order_item → OrderItem.
func exportedName(domain string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(domain, "_") {
		if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

// structField — поле name структурного типа expr или nil.
func structField(expr ast.Expr, name string) *ast.Field {
	st, ok := expr.(*ast.StructType)
	if !ok {
		return nil
	}
	for _, field := range st.Fields.List {
		for _, n := range field.Names {
			if n.Name == name {
				return field
			}
		}
	}
	return nil
}

// TestDomainErrorTables: в домене, который регистрирует HTTP-операции, таблица домена <домен>Errors
// (delivery/http) переводит каждую ошибку его errors.go, а хендлеры huma.Register отдают ошибки только
// через неё: ни одна ошибка домена не уходит клиенту 500 ни на одной операции. Таблица операции лишь
// объявляет свои коды в OpenAPI — Declare при регистрации.
func TestDomainErrorTables(t *testing.T) {
	t.Parallel()
	files := map[string][]goFile{} // домен → не-тестовые файлы его delivery/http
	for _, f := range goFiles(t) {
		if domain, sub, ok := splitDomain(f.pkg); ok && sub == "delivery/http" && !f.test {
			files[domain] = append(files[domain], f)
		}
	}
	for domain, pkgFiles := range files {
		table := strings.ToLower(exportedName(domain)[:1]) + exportedName(domain)[1:] + "Errors"
		root := module + "/internal/domain/" + domain
		rules := map[string]string{}   // переменная-правило → ошибка домена
		errorMaps := map[string]bool{} // переменные — таблицы ошибок
		handlers := map[string]bool{}  // методы — хендлеры huma.Register
		declared := map[string]bool{}  // таблицы с Declare в операции huma.Register
		var (
			tableArgs   []ast.Expr
			tableByName map[string]string // импорты файла таблицы
		)
		registers := false
		parsed := make([]*ast.File, len(pkgFiles))
		for i, f := range pkgFiles {
			file := parseGo(t, f.path)
			parsed[i] = file
			byName := importsByName(file)
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					for j, name := range vs.Names {
						if j >= len(vs.Values) {
							continue
						}
						if rule := ruleErr(vs.Values[j], byName, root); rule != "" {
							rules[name.Name] = rule
						}
						if call, ok := vs.Values[j].(*ast.CallExpr); ok && isSelector(call.Fun, byName, httpkitPath, "NewErrorMap") {
							errorMaps[name.Name] = true
							if name.Name == table {
								tableArgs, tableByName = call.Args, byName
							}
						}
					}
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isSelector(call.Fun, byName, "github.com/danielgtaylor/huma/v2", "Register") || len(call.Args) < 3 {
					return true
				}
				registers = true
				if h, ok := call.Args[2].(*ast.SelectorExpr); ok {
					handlers[h.Sel.Name] = true
				}
				ast.Inspect(call.Args[1], func(n ast.Node) bool {
					if name := calledOn(n, "Declare"); name != "" {
						declared[name] = true
					}
					return true
				})
				return true
			})
		}
		if !registers {
			continue
		}
		path := "internal/domain/" + domain + "/delivery/http"
		if tableArgs == nil {
			t.Errorf("%s registers operations without the domain error table %s = httpkit.NewErrorMap(…)", path, table)
			continue
		}
		covered := map[string]bool{}
		for _, arg := range tableArgs {
			if id, ok := arg.(*ast.Ident); ok {
				covered[rules[id.Name]] = true
			}
			covered[ruleErr(arg, tableByName, root)] = true
		}
		for _, sentinel := range domainErrors(t, domain) {
			if !covered[sentinel] {
				t.Errorf("%s.%s has no rule in %s.%s: the domain error would answer 500", domain, sentinel, path, table)
			}
		}
		for name := range errorMaps {
			if name != table && !declared[name] {
				t.Errorf("%s: operation table %s is not declared in huma.Register(api, %s.Declare(op), …): its codes are missing from OpenAPI", path, name, name)
			}
		}
		for i, file := range parsed {
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if m := calledOn(x, "Map"); errorMaps[m] && m != table {
						t.Errorf("%s maps errors with %s: a handler maps through the domain table %s, an operation table only declares", pkgFiles[i].path, m, table)
					}
				case *ast.FuncDecl:
					if x.Recv != nil && handlers[x.Name.Name] {
						ast.Inspect(x.Body, func(n ast.Node) bool {
							if ret, ok := n.(*ast.ReturnStmt); ok && len(ret.Results) > 0 && isIdent(ret.Results[len(ret.Results)-1], "err") {
								t.Errorf("%s: handler %s returns err unmapped: a handler returns %s.Map(err)", pkgFiles[i].path, x.Name.Name, table)
							}
							return true
						})
					}
				}
				return true
			})
		}
	}
}

const httpkitPath = "git.stit.tech/stit-core/golibinfra/httpkit"

// calledOn — имя переменной x в вызове x.method(…); "" — не такой вызов.
func calledOn(n ast.Node, method string) string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != method {
		return ""
	}
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// ruleErr — ошибка домена root в литерале httpkit.ErrorRule{Err: <домен>.ErrX, …}; "" — не такой литерал.
func ruleErr(expr ast.Expr, byName map[string]string, root string) string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || !isSelector(lit.Type, byName, httpkitPath, "ErrorRule") {
		return ""
	}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok && isIdent(kv.Key, "Err") {
			if sel, ok := kv.Value.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && byName[id.Name] == root {
					return sel.Sel.Name
				}
			}
		}
	}
	return ""
}

// isSelector — expr — это <пакет pkg>.name.
func isSelector(expr ast.Expr, byName map[string]string, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && byName[id.Name] == pkg
}

// domainErrors — ошибки (переменные Err*) из errors.go домена.
func domainErrors(t *testing.T, domain string) []string {
	t.Helper()
	var names []string
	for _, decl := range parseGo(t, "internal/domain/"+domain+"/errors.go").Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			for _, id := range spec.(*ast.ValueSpec).Names {
				if strings.HasPrefix(id.Name, "Err") {
					names = append(names, id.Name)
				}
			}
		}
	}
	return names
}

// TestSubjectLiteralOwnership: литерал "<domain>.events." встречается только в
// самом домене-источнике или в delivery/nats консьюмеров; exported Subject* у
// источника запрещён (consumers обязаны держать свою константу).
func TestSubjectLiteralOwnership(t *testing.T) {
	t.Parallel()
	names := domains(t)
	for _, f := range goFiles(t) {
		if !under(f.pkg, "internal") || f.test {
			continue
		}
		ast.Inspect(parseGo(t, f.path), func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, d := range names {
				if !strings.HasPrefix(val, d+".events.") {
					continue
				}
				inOwner := under(f.pkg, "internal/domain/"+d)
				inConsumerNATS := strings.Contains(f.pkg, "delivery/nats")
				require.True(t, inOwner || inConsumerNATS,
					"%s contains subject literal %q — only the owning domain or a delivery/nats consumer may declare it", f.path, val)
			}
			return true
		})
	}
}

func TestNoExportedSubjectInDomainRoot(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, d := range domains(t) {
		matches, err := filepath.Glob(filepath.Join(root, "internal", "domain", d, "*.go"))
		require.NoError(t, err)
		for _, path := range matches {
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoError(t, err)
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, n := range vs.Names {
						require.False(t, strings.HasPrefix(n.Name, "Subject"),
							"%s exports %q — subjects stay unexported in the source domain", path, n.Name)
					}
				}
			}
		}
	}
}

// TestTestsAssert: покрытие считает исполненный код, поэтому тест домена или платформы без
// единого утверждения поднимает его, ничего не проверив.
func TestTestsAssert(t *testing.T) {
	t.Parallel()
	type testFile struct {
		path   string
		file   *ast.File
		byName map[string]string
	}
	byPkg := map[string][]testFile{}
	for _, f := range goFiles(t) {
		if f.test && (under(f.pkg, "internal/domain") || under(f.pkg, "internal/platform")) {
			file := parseGo(t, f.path)
			byPkg[f.pkg] = append(byPkg[f.pkg], testFile{f.path, file, importsByName(file)})
		}
	}
	for _, files := range byPkg {
		// Утверждающий хелпер пакета — функция без результатов или общая таблица (var) с
		// утверждением, в том числе через другой хелпер: фикстура с require (newUC) возвращает
		// значение, тест на ней одной — пустышка.
		helpers := map[string]bool{}
		for grew := true; grew; {
			grew = false
			for _, f := range files {
				for _, decl := range f.file.Decls {
					for name, node := range helperDecls(decl) {
						if !helpers[name] && asserts(node, f.byName, helpers) {
							helpers[name], grew = true, true
						}
					}
				}
			}
		}
		for _, f := range files {
			for _, decl := range f.file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" &&
					!asserts(fn.Body, f.byName, helpers) {
					t.Errorf("%s: %s asserts nothing — no require, assert, Assert*/Require* of a test kit or testify/mock, or helper of the package that asserts (rule tests-pipeline): such a test raises coverage without checking anything", f.path, fn.Name.Name)
				}
			}
		}
	}
}

// helperDecls — кандидаты в утверждающие хелперы декларации: тело функции без получателя и
// результатов, значение каждой переменной пакета — своё, а не всего блока var.
func helperDecls(decl ast.Decl) map[string]ast.Node {
	nodes := map[string]ast.Node{}
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil && d.Type.Results == nil {
			nodes[d.Name.Name] = d.Body
		}
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Values) == len(vs.Names) {
				for i, name := range vs.Names {
					nodes[name.Name] = vs.Values[i]
				}
			}
		}
	}
	return nodes
}

// asserts — в узле есть testify require/assert, Assert*/Require* тестового кита (пакета из testkit)
// или testify/mock (AssertExpectationsForObjects), утверждающий хелпер.
func asserts(node ast.Node, byName map[string]string, helpers map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok {
				pkg := byName[id.Name]
				kitAssert := strings.HasPrefix(x.Sel.Name, "Assert") || strings.HasPrefix(x.Sel.Name, "Require")
				found = found || pkg == "github.com/stretchr/testify/require" || pkg == "github.com/stretchr/testify/assert" ||
					(strings.Contains(pkg, "/testkit/") || pkg == "github.com/stretchr/testify/mock") && kitAssert
			}
		case *ast.Ident:
			found = found || helpers[x.Name]
		}
		return !found
	})
	return found
}
