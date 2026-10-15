// Package archtest машинно держит обвязку шаблона и канон доменов (rule domain-architecture) для любого
// каталога internal/domain/*; на archtest и archkit //nolint не действует. Тесты по файлам:
//
//   - layout_test.go — TestLayout: раскладку держит archkit (golibinfra) — каталоги internal/, импорты между
//     доменами и слоями, чистоту корня и чистых слоёв, настенные часы, порты между доменами без
//     identity-closure, свои таблицы; правила едут версией кита, настройки сервиса (layout и киты входа
//     deliveryKits) — layout_settings_test.go;
//   - archtest_test.go — TestHTTPTypes (префикс, уникальность и envelope HTTP-типов), TestDomainErrorTables
//     (таблица ошибок домена и ответ хендлера — nil или она), TestDomainErrorsInErrorsGo (ошибки корня — только
//     в errors.go), TestSubjectLiteralOwnership и TestNoExportedSubjectInDomainRoot (владение subject'ами),
//     TestTestsAssert (тесты без утверждений);
//   - crossdomain_test.go — TestTrustedOnlyInModules и TestTrustedErrors (Trusted между доменами), TestNoFxAs
//     (привязка портов без fx.As), TestModulesEmbedNoForeignTypes (порт между доменами — типовой адаптер);
//   - delivery_test.go — вход домена: TestDeliveryPackages (пакет delivery/<транспорт>),
//     TestInputsRegisterInDelivery (операции и подписки — только в нём), TestSubscriptionsHaveContractTest
//     (контракт-тест подписки), TestDeliveryReachesUsecaseOnly (из корня — данные, киты — deliveryKits),
//     TestDeliveryPortsBoundToUsecase (порт входа — над *Usecase), TestSecuredOperationsAskPolicy (операция под
//     Security — с policy);
//   - generated_test.go — TestGeneratedHeaderOnlyInGeneratorOutput (заголовок сгенерированного — только у вывода
//     генераторов), TestBuildConstraints (ограничение сборки — только integration у тестов),
//     TestLinterDirectives (директив линтеров мимо //nolint нет);
//   - harness_test.go — TestDomainsAreWired (домены подключены в cmd/api), TestProseReferencesExist (ссылки прозы
//     на make-цели, пути, тесты, разделы и ссылки, факты гейта) и TestSectionAnchors (якоря разделов и ссылок по
//     правилу Gitea), TestRulesAndSkillsLoad (доставка правил и скиллов: шапка, paths:, name:, шаг 0),
//     TestNolintNamesEnabledLinters и TestNolintNames (//nolint — только включённых линтеров), TestStatusInfraTouch
//     (сигнал infra-touch make status);
//   - template_test.go — TestTemplatePaths, TestTemplateUpdate, TestTemplateUpdateFirstBase,
//     TestTemplateUpdateBeforeVersionTag, TestTemplateUpdatePaths, TestTemplateUpdateRemote (пути и версия
//     обвязки .template, её перенос в форк make template-update), TestNoConflictMarkers и TestConflictMarkers;
//   - onboarding_test.go — TestIdentityPaths, TestIdentityDependents, TestPruneNamesIdentityDependents (пути
//     доменов идентичности и процедура их prune), TestCanonicalTemplate (канонический репо шаблона),
//     TestStackValidator (валидатор stack-артефактов);
//   - guard_test.go, commitmsg_test.go, hooks_test.go — TestGuardBash, TestGuardWired (подключение guard-bash и
//     запреты deny .claude/settings.json), TestCommitMsgHook, TestHooksVisible и TestSetupWiresHooksFirst (хуки
//     guard-bash и commit-msg, подключение git-хуков: make setup, status, doctor);
//   - make_test.go — TestMakeCheckCannotBeForged, TestCheckCoversItsOwnRun, TestPrePushGatesPushedCommit,
//     TestGateStepsIgnoreShellEnvironment, TestHooksIgnoreCallerEnvironment (самозащита make check и pre-push от
//     флагов, переменных и окружения вызывающего), TestReleaseGateIgnoresGNUmakefile,
//     TestReleaseStopsWithoutModuleAccess, TestReleasePreflight, TestReleaseBuildForwardsSSHAgent (релиз);
//   - steps_test.go — TestCoverCheck, TestLintUsesItsConfig, TestGenCheckComparesMocksWithTree,
//     TestFmtAppliesLintRules, TestMigrationsCheck (шаги гейта);
//   - newdomain_test.go — TestNewDomainRefusesOrRollsBack (make new-domain: отказ до первой правки, откат после);
//   - release_test.go — TestReleaseImage (путь образа релиза).
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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
				// Регистр — не важен: инициализм revive пишет заглавными (api_key → APIKey).
				if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
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
// (delivery/http) переводит каждую ошибку его errors.go, а хендлеры huma.Register отдают ошибкой только nil
// или <домен>Errors.Map(…): ошибка домена (она — только из errors.go, TestDomainErrorsInErrorsGo) не уходит
// клиенту 500 ни на одной операции. Таблица операции лишь объявляет свои коды в OpenAPI — Declare при
// регистрации.
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
		handlers := map[string]bool{}  // методы и функции пакета — хендлеры huma.Register
		declared := map[string]bool{}  // таблицы с Declare в операции huma.Register
		var (
			tableArgs   []ast.Expr
			tableByName map[string]string // импорты файла таблицы
		)
		registers := false
		literals := map[*ast.FuncLit]string{} // хендлеры-литералы huma.Register → файл
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
							if strings.EqualFold(name.Name, table) { // инициализм revive: user_id — userIDErrors
								table, tableArgs, tableByName = name.Name, call.Args, byName
							}
						}
					}
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if pkg, name := kitCall(call, byName); pkg != humaPath || name != "Register" || len(call.Args) < 3 {
					return true
				}
				registers = true
				switch h := call.Args[2].(type) {
				case *ast.SelectorExpr:
					handlers[h.Sel.Name] = true
				case *ast.Ident:
					handlers[h.Name] = true
				case *ast.FuncLit:
					literals[h] = f.path
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
					if handlers[x.Name.Name] {
						handlerReturns(t, pkgFiles[i].path, x.Name.Name, x.Body, table)
					}
				}
				return true
			})
		}
		for lit, where := range literals {
			handlerReturns(t, where, "func literal", lit.Body, table)
		}
	}
}

// handlerReturns: хендлер отдаёт ошибкой nil или <таблица домена>.Map(…) — другая ошибка ушла бы клиенту 500
// или мимо кодов таблицы. Литерал внутри хендлера — своя функция: его return — не ответ хендлера.
func handlerReturns(t *testing.T, path, handler string, body *ast.BlockStmt, table string) {
	t.Helper()
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if len(x.Results) == 0 {
				return true
			}
			if last := x.Results[len(x.Results)-1]; !isIdent(last, "nil") && calledOn(last, "Map") != table {
				t.Errorf("%s: handler %s returns %s: a handler returns nil or %s.Map(err) — any other error answers 500 or bypasses the codes of the table", path, handler, types.ExprString(last), table)
			}
		}
		return true
	})
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

var sentinelName = regexp.MustCompile(`^Err[A-Z]`)

// TestDomainErrorsInErrorsGo: ошибка домена — sentinel Err… или тип из errors.go: таблицу домена сверяют с
// ним (TestDomainErrorTables), и ошибка, созданная в другом файле корня, ушла бы клиенту 500. fmt.Errorf без
// %w теряет sentinel или причину; тип ошибки разворачивается (Unwrap) в sentinel errors.go — правило таблицы
// для sentinel переводит и тип.
func TestDomainErrorsInErrorsGo(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if _, sub, ok := splitDomain(f.pkg); !ok || sub != "" || f.test {
			continue
		}
		file := parseGo(t, f.path)
		byName := importsByName(file)
		inErrorsGo := path.Base(f.path) == "errors.go"
		sentinels := map[string]bool{}
		values := map[ast.Expr]bool{} // значения sentinel'ов errors.go: там errors.New и место
		for _, decl := range file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					for j, id := range vs.Names {
						switch named := sentinelName.MatchString(id.Name); {
						case id.Name == "_": // проверка интерфейса: var _ error = (*LockedError)(nil)
						case inErrorsGo && named:
							sentinels[id.Name] = true
							if j < len(vs.Values) {
								values[vs.Values[j]] = true
							}
						case inErrorsGo:
							t.Errorf("%s declares %s: errors.go holds exported sentinels Err… — an unexported one has no rule in the HTTP table", f.path, id.Name)
						case named:
							t.Errorf("%s declares %s: a sentinel lives in errors.go, where the HTTP error table is checked against it", f.path, id.Name)
						}
					}
				}
			}
		}
		var errorTypes []string
		unwraps := map[string]bool{} // типы, чей Unwrap отдаёт sentinel errors.go
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				switch {
				case isSelector(x.Fun, byName, "errors", "New") && !values[x]:
					t.Errorf("%s creates an error with errors.New outside a sentinel var Err… of errors.go: a domain error is such a sentinel, with a rule in the HTTP error table", f.path)
				case isSelector(x.Fun, byName, "fmt", "Errorf") && !wrapsError(x):
					t.Errorf("%s calls fmt.Errorf without %%w: the error loses its sentinel or cause — wrap it, a new domain error is a sentinel of errors.go", f.path)
				}
			case *ast.FuncDecl:
				recv := receiverName(x)
				switch {
				case recv != "" && x.Name.Name == "Error" && !inErrorsGo:
					t.Errorf("%s declares error type %s: domain error types live in errors.go", f.path, recv)
				case recv != "" && x.Name.Name == "Error":
					errorTypes = append(errorTypes, recv)
				case recv != "" && x.Name.Name == "Unwrap" && returnsOneOf(x, sentinels):
					unwraps[recv] = true
				}
			}
			return true
		})
		for _, typ := range errorTypes {
			if !unwraps[typ] {
				t.Errorf("%s: error type %s does not unwrap to a sentinel of errors.go: the HTTP table maps sentinels, the type would answer 500", f.path, typ)
			}
		}
	}
}

// wrapsError — fmt.Errorf с литералом формата, в котором есть %w.
func wrapsError(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	format, err := strconv.Unquote(lit.Value)
	return err == nil && strings.Contains(format, "%w")
}

// receiverName — имя типа получателя метода; "" — функция.
func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// returnsOneOf — тело функции — один return имени из names.
func returnsOneOf(fn *ast.FuncDecl, names map[string]bool) bool {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return false
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	id, ok := ret.Results[0].(*ast.Ident)
	return ok && names[id.Name]
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

// TestTestsAssert: покрытие считает исполненный код, поэтому тест без единого утверждения поднимает его,
// ничего не проверив, — в любом пакете модуля: покрытие набирают и тесты pkg/ и cmd/. Тест домена и
// платформы утверждает require, assert или проверкой тестового кита (rule tests-pipeline); остальным
// засчитываются ещё t.Error*, t.Fatal*, t.Fail* и t, отданный проверке тестового кита — его пакета
// (archkit.Check, testkit.MigrationLadder) или значения из него (envAllowlist.Check), а не любому вызову
// (fmt.Println(t)).
func TestTestsAssert(t *testing.T) {
	t.Parallel()
	type testFile struct {
		path   string
		file   *ast.File
		byName map[string]string
	}
	byPkg := map[string][]testFile{}
	for _, f := range goFiles(t) {
		if f.test {
			file := parseGo(t, f.path)
			byPkg[f.pkg] = append(byPkg[f.pkg], testFile{f.path, file, importsByName(file)})
		}
	}
	for pkg, files := range byPkg {
		bare := !under(pkg, "internal/domain") && !under(pkg, "internal/platform")
		// Значения пакета из тестового кита (var envAllowlist = allowlist.List{}): вызов их метода с t — проверка кита.
		kits := map[string]bool{}
		for _, f := range files {
			for _, decl := range f.file.Decls {
				if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
					for _, spec := range gd.Specs {
						vs := spec.(*ast.ValueSpec)
						for i, name := range vs.Names {
							if i < len(vs.Values) && testKit(f.byName[rootPkg(vs.Values[i])]) {
								kits[name.Name] = true
							}
						}
					}
				}
			}
		}
		// Утверждающий хелпер пакета — функция без результатов или общая таблица (var) с
		// утверждением, в том числе через другой хелпер: фикстура с require (newUC) возвращает
		// значение, тест на ней одной — пустышка.
		helpers := map[string]bool{}
		for grew := true; grew; {
			grew = false
			for _, f := range files {
				for _, decl := range f.file.Decls {
					for name, node := range helperDecls(decl) {
						if !helpers[name] && asserts(node, f.byName, helpers, kits, bare) {
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
					!asserts(fn.Body, f.byName, helpers, kits, bare) {
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
// или testify/mock (AssertExpectationsForObjects), утверждающий хелпер; bare — ещё и t.Error*, t.Fatal*,
// t.Fail* и вызов-оператор кита, которому отдан t: функции его пакета (archkit.Check(t, …)) или метода
// значения из него (kits: envAllowlist.Check(t, …)).
func asserts(node ast.Node, byName map[string]string, helpers, kits map[string]bool, bare bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ExprStmt:
			call, ok := x.X.(*ast.CallExpr)
			if !ok || !bare {
				break
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			if isIdent(sel.X, "t") {
				switch sel.Sel.Name {
				case "Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow":
					found = true
				}
			} else if id, ok := sel.X.(*ast.Ident); ok && len(call.Args) > 0 && isIdent(call.Args[0], "t") &&
				(testKit(byName[id.Name]) || kits[id.Name]) {
				found = true
			}
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

// testKit — пакет тестового кита: golibinfra или internal/testkit сервиса.
func testKit(path string) bool {
	return strings.HasPrefix(path, "git.stit.tech/stit-core/golibinfra/") || strings.Contains(path, "/internal/testkit/")
}

// rootPkg — имя пакета, из которого выражение заводит значение (allowlist.List{}, &kit.T{}, kit.New()), или "".
func rootPkg(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.CompositeLit:
		return rootPkg(x.Type)
	case *ast.UnaryExpr:
		return rootPkg(x.X)
	case *ast.CallExpr:
		return rootPkg(x.Fun)
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}
