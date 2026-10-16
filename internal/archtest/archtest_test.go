// Package archtest машинно держит обвязку шаблона и канон доменов (rule domain-architecture) для любого
// каталога internal/domain/*; на archtest и archkit //nolint не действует. Правила кода (rules) читают дерево с
// корнем: репо или пример testdata/rules/*.txtar — у каждого есть нарушение и законная форма. Тесты по файлам:
//
//   - layout_test.go — TestLayout: раскладку держит archkit (golibinfra) — каталоги internal/, импорты между
//     доменами и слоями, чистоту корня и чистых слоёв, настенные часы, порты между доменами без
//     identity-closure, свои таблицы; правила едут версией кита, настройки сервиса (layout и киты входа
//     deliveryKits) — layout_settings_test.go;
//   - archtest_test.go — TestHTTPTypes (префикс, уникальность и envelope HTTP-типов), TestDomainErrorTables
//     (таблица ошибок домена и ответ хендлера — nil или она), TestDomainErrorsInErrorsGo (ошибки корня — только
//     в errors.go), TestSubjectLiteralOwnership и TestNoExportedSubjectInDomainRoot (владение subject'ами),
//     TestSubjectVersions (первая версия subject — без суффикса), TestDomainPackages (подпакеты домена, SQL — только
//     infra/postgres, natskit — только delivery/nats), TestDomainDeterminism (время, ID и случайность — киты, состояния
//     в переменных корня нет), TestRootTypes (типы корня без тегов транспорта, payload событий без PII),
//     TestTestsAssert (тесты и fuzz-цели без утверждений, мягкие ожидания моков), TestRuleExamples (примеры правил
//     кода: нарушение — отказ с текстом, законная форма — тишина);
//   - crossdomain_test.go — TestTrustedOnlyInModules и TestTrustedErrors (Trusted между доменами), TestNoFxAs
//     (привязка портов без fx.As), TestModulesEmbedNoForeignTypes (порт между доменами — типовой адаптер),
//     TestModulesOnlyWire (module/ только связывает: Module, адаптеры поверх Trusted, без хуков fx),
//     TestDomainDependencies (порт привязывает свой домен, граф доменов ацикличен);
//   - delivery_test.go — вход домена: TestDeliveryPackages (пакет delivery/<транспорт>),
//     TestInputsRegisterInDelivery (операции и подписки — только в нём), TestSubscriptionsHaveContractTest
//     (контракт-тест подписки), TestDeliveryReachesUsecaseOnly (из корня — данные, киты — deliveryKits),
//     TestDeliveryPortsBoundToUsecase (порт входа — над *Usecase), TestSecuredOperationsAskPolicy (операция под
//     Security — с policy до чтения), TestIdentityWithOnlyInAuthgate (личность в ctx кладёт только authgate);
//   - generated_test.go — TestMain (тест archtest не пропускается), TestGeneratedHeaderOnlyInGeneratorOutput
//     (заголовок сгенерированного — только у вывода генераторов), TestMainOnlyEnters (main бинаря — только вход),
//     TestBuildConstraints (ограничение сборки — только integration у тестов), TestLinterDirectives (директив
//     линтеров мимо //nolint нет);
//   - harness_test.go — TestDomainsAreWired (домены подключены в cmd/api), TestProseReferencesExist (ссылки прозы
//     на make-цели, пути, тесты, разделы и ссылки, факты гейта) и TestSectionAnchors (якоря разделов и ссылок по
//     правилу Gitea), TestRulesAndSkillsLoad (доставка правил и скиллов: шапка, paths:, name:, шаг 0),
//     TestNolintNamesEnabledLinters и TestNolintNames (//nolint — только включённых линтеров), TestStatusInfraTouch
//     (сигнал infra-touch make status);
//   - template_test.go — TestTemplatePaths, TestTemplateUpdate, TestTemplateUpdateFirstBase,
//     TestTemplateUpdateBeforeVersionTag, TestTemplateUpdatePaths, TestTemplateUpdateRemote (пути и версия
//     обвязки .template, её перенос в форк make template-update), TestTemplateAnnotation (разделы аннотации тега
//     выпуска — в формате переноса), TestNoConflictMarkers и TestConflictMarkers;
//   - onboarding_test.go — TestIdentityPaths, TestIdentityDependents, TestPruneNamesIdentityDependents (пути
//     доменов идентичности и процедура их prune), TestCanonicalTemplate (канонический репо шаблона),
//     TestStackValidator (валидатор stack-артефактов);
//   - guard_test.go, commitmsg_test.go, hooks_test.go — TestGuardBash, TestGuardWired (подключение guard-bash и
//     запреты deny .claude/settings.json), TestCommitMsgHook, TestHooksVisible и TestSetupWiresHooksFirst (хуки
//     guard-bash и commit-msg, подключение git-хуков: make setup, status, doctor);
//   - make_test.go — TestMakeCheckCannotBeForged, TestCheckCoversItsOwnRun, TestPrePushGatesPushedCommit,
//     TestGateStepsIgnoreShellEnvironment, TestHooksIgnoreCallerEnvironment (самозащита make check и pre-push от
//     флагов, переменных и окружения вызывающего, секреты и сообщения пушимых коммитов), TestPreCommitChecksIndex
//     (pre-commit судит индекс), TestReleaseGateIgnoresGNUmakefile,
//     TestReleaseStopsWithoutModuleAccess, TestReleasePreflight, TestReleaseBuildForwardsSSHAgent (релиз);
//   - steps_test.go — TestCoverCheck, TestLintUsesItsConfig, TestGenCheckComparesMocksWithTree,
//     TestFmtAppliesLintRules, TestMigrationsCheck, TestToolVersions, TestGenBuildsFirst (шаги гейта, версии
//     инструментов, make gen);
//   - newdomain_test.go — TestNewDomainRefusesOrRollsBack (make new-domain: отказ до первой правки, откат после);
//   - mutate_test.go — TestMutate (раннер make mutate: вердикты, --strict, файлы кода без мутаций, дерево
//     разработчика);
//   - repo_test.go — TestNoSecretFiles (секретов в индексе нет, перечень — строка pre-push, с ним сверен
//     .gitignore), TestImagesPinned (образы — по digest и тегу), TestAllowReadsOnly (allow .claude/settings.json —
//     только то, что не пишет, ask нет);
//   - release_test.go — TestReleaseImage (путь образа релиза).
package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

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
	return domainsAt(t, repoRoot(t))
}

// domainsAt — каталоги internal/domain дерева root.
func domainsAt(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "internal", "domain"))
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
	return goFilesAt(t, repoRoot(t))
}

// goFilesAt — .go-файлы дерева root вне скрытых каталогов.
func goFilesAt(t *testing.T, root string) []goFile {
	t.Helper()
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
	return parseGoAt(t, repoRoot(t), rel)
}

// parseGoAt — файл дерева root по пути от корня, с комментариями.
func parseGoAt(t *testing.T, root, rel string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, parser.ParseComments)
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

// rules — правила кода по имени их теста: тест гоняет правило по репо (runRule), TestRuleExamples — по примерам
// testdata/rules.
var rules = map[string]func(*check){
	"TestHTTPTypes":                     httpTypes,
	"TestDomainErrorTables":             domainErrorTables,
	"TestDomainErrorsInErrorsGo":        domainErrorsInErrorsGo,
	"TestSubjectLiteralOwnership":       subjectLiteralOwnership,
	"TestNoExportedSubjectInDomainRoot": noExportedSubjectInDomainRoot,
	"TestSubjectVersions":               subjectVersions,
	"TestTrustedOnlyInModules":          trustedOnlyInModules,
	"TestTrustedErrors":                 trustedErrors,
	"TestModulesEmbedNoForeignTypes":    modulesEmbedNoForeignTypes,
	"TestNoFxAs":                        noFxAs,
	"TestDeliveryPackages":              deliveryPackages,
	"TestInputsRegisterInDelivery":      inputsRegisterInDelivery,
	"TestSubscriptionsHaveContractTest": subscriptionsHaveContractTest,
	"TestDeliveryReachesUsecaseOnly":    deliveryReachesUsecaseOnly,
	"TestDeliveryPortsBoundToUsecase":   deliveryPortsBoundToUsecase,
	"TestSecuredOperationsAskPolicy":    securedOperationsAskPolicy,
	"TestModulesOnlyWire":               modulesOnlyWire,
	"TestDomainDependencies":            domainDependencies,
	"TestDomainPackages":                domainPackages,
	"TestDomainDeterminism":             domainDeterminism,
	"TestRootTypes":                     rootTypes,
	"TestIdentityWithOnlyInAuthgate":    identityWithOnlyInAuthgate,
}

// check — прогон правила по дереву: репо или пример testdata/rules. Нарушение правило отдаёт c.Errorf — в репо это
// отказ теста, в примере — запись, которую пример сверяет со своими want; сбой чтения — require на c.T.
type check struct {
	T        *testing.T
	root     string
	files    []goFile  // .go-файлы дерева
	found    *[]string // отказы примера; nil — дерево репо
	released []string  // subject'ы .v1, выпущенные до канона (releasedSubjects)
}

// runRule — правило теста t (rules[t.Name()]) по дереву репо.
func runRule(t *testing.T) {
	t.Helper()
	rule, ok := rules[t.Name()]
	require.True(t, ok, "%s is not in rules", t.Name())
	rule(&check{T: t, root: repoRoot(t), files: goFiles(t), released: releasedSubjects})
}

func (c *check) Errorf(format string, args ...any) {
	c.T.Helper()
	if c.found == nil {
		c.T.Errorf(format, args...)
		return
	}
	*c.found = append(*c.found, fmt.Sprintf(format, args...))
}

func (c *check) parse(rel string) *ast.File {
	c.T.Helper()
	return parseGoAt(c.T, c.root, rel)
}

func (c *check) domains() []string {
	c.T.Helper()
	return domainsAt(c.T, c.root)
}

// TestRuleExamples: у каждого правила rules — примеры testdata/rules/*.txtar: нарушение (want — подстрока отказа:
// каждая находит отказ, каждый отказ — свою want) и законная форма (без want — правило молчит). Правило, которое
// замолчало или шумит на законной форме, роняет пример; правило без примера нарушения или законной формы — отказ.
func TestRuleExamples(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob(filepath.Join("testdata", "rules", "*.txtar"))
	require.NoError(t, err)
	violation, legit := map[string]bool{}, map[string]bool{}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".txtar")
		ex := loadExample(t, name, true)
		checked := ex.rules
		if slices.Equal(checked, []string{"*"}) {
			checked = slices.Sorted(maps.Keys(rules))
		}
		require.NotEmpty(t, checked, "%s names no rule", p)
		require.True(t, len(ex.want) == 0 || len(checked) == 1, "%s: a violation example checks one rule", p)
		for _, rule := range checked {
			require.Contains(t, rules, rule, "%s: unknown rule", p)
			violation[rule] = violation[rule] || len(ex.want) > 0
			legit[rule] = legit[rule] || len(ex.want) == 0
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFiles(t, root, ex.files)
			for _, rule := range checked {
				var found []string
				rules[rule](&check{T: t, root: root, files: goFilesAt(t, root), found: &found, released: ex.released})
				for _, want := range ex.want {
					require.True(t, slices.ContainsFunc(found, func(f string) bool { return strings.Contains(f, want) }),
						"%s: no refusal with %q; refusals: %q", rule, want, found)
				}
				for _, f := range found {
					require.True(t, slices.ContainsFunc(ex.want, func(w string) bool { return strings.Contains(f, w) }),
						"%s refuses what the example does not want: %s", rule, f)
				}
			}
		})
	}
	for rule := range rules {
		require.True(t, violation[rule], "%s has no violation example in testdata/rules", rule)
		require.True(t, legit[rule], "%s has no legit example in testdata/rules", rule)
	}
}

// example — пример из testdata/rules/<name>.txtar. Комментарий архива: rule — правила через пробел (* — все), want —
// подстрока ожидаемого отказа (без want — законная форма), base — пример, чьё дерево берётся за основу, drop — файл
// основы, которого в дереве нет, released — строка releasedSubjects; строки с # — пояснения. Файлы архива дописывают и заменяют файлы основы; {module} в
// файлах и want — module path сервиса: de-template меняет его в коде, а примеры остаются верны.
type example struct {
	rules, want, released []string
	files                 map[string]string
}

func loadExample(t *testing.T, name string, withBase bool) example {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "rules", name+".txtar"))
	require.NoError(t, err)
	comment, files := parseTxtar(t, name, string(data))
	ex := example{files: map[string]string{}}
	var drop []string
	for line := range strings.Lines(comment) {
		line = strings.TrimSpace(line)
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		switch key {
		case "rule":
			ex.rules = append(ex.rules, strings.Fields(value)...)
		case "want":
			ex.want = append(ex.want, strings.ReplaceAll(value, "{module}", module))
		case "base":
			require.True(t, withBase, "%s: the base %s has a base itself", name, value)
			maps.Copy(ex.files, loadExample(t, value, false).files)
		case "drop":
			drop = append(drop, value)
		case "released":
			ex.released = append(ex.released, value)
		default:
			require.True(t, line == "" || strings.HasPrefix(line, "#"), "%s: unknown line %q", name, line)
		}
	}
	for name, content := range files {
		ex.files[name] = strings.ReplaceAll(content, "{module}", module)
	}
	for _, f := range drop {
		require.Contains(t, ex.files, f, "%s drops a file the tree does not have", name)
		delete(ex.files, f)
	}
	return ex
}

// parseTxtar — комментарий и файлы архива: строка «-- путь --» открывает файл (формат golang.org/x/tools/txtar).
func parseTxtar(t *testing.T, name, data string) (string, map[string]string) {
	t.Helper()
	var comment string
	files := map[string]string{}
	file, body := "", strings.Builder{}
	flush := func() {
		if file == "" {
			comment = body.String()
		} else {
			require.NotContains(t, files, file, "%s: file %s twice", name, file)
			files[file] = body.String()
		}
		body.Reset()
	}
	for line := range strings.Lines(data) {
		if marker, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "-- "); ok && strings.HasSuffix(marker, " --") {
			flush()
			file = strings.TrimSuffix(marker, " --")
			continue
		}
		body.WriteString(line)
	}
	flush()
	return comment, files
}

// TestHTTPTypes: huma регистрирует схемы по имени Go-типа на весь API, и одноимённые типы двух
// доменов роняют старт паникой — поэтому у типа delivery/http префикс домена и уникальное имя;
// тело ответа — envelope {data}. Handler, интерфейсы (порты входа) и типы-функции в схемы не попадают.
func TestHTTPTypes(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func httpTypes(c *check) {
	seen := map[string]string{} // имя типа → файл
	for _, f := range c.files {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "delivery/http" || f.test {
			continue
		}
		prefix := exportedName(domain)
		for _, decl := range c.parse(f.path).Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				name := ts.Name.Name
				switch ts.Type.(type) {
				case *ast.InterfaceType, *ast.FuncType:
					continue
				}
				if !ast.IsExported(name) || name == "Handler" {
					continue
				}
				// Регистр — не важен: инициализм revive пишет заглавными (api_key → APIKey).
				if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
					c.Errorf("%s declares %s: an HTTP type starts with the domain prefix %s", f.path, name, prefix)
				}
				if prev, dup := seen[name]; dup {
					c.Errorf("type %s is declared in both %s and %s: huma names schemas by Go type", name, prev, f.path)
				}
				seen[name] = f.path
				if body := structField(ts.Type, "Body"); strings.HasSuffix(name, "Output") && body != nil && structField(body.Type, "Data") == nil {
					c.Errorf("%s declares %s with a Body without Data: a response body is the envelope {data}", f.path, name)
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
	runRule(t)
}

func domainErrorTables(c *check) {
	files := map[string][]goFile{} // домен → не-тестовые файлы его delivery/http
	for _, f := range c.files {
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
			file := c.parse(f.path)
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
			c.Errorf("%s registers operations without the domain error table %s = httpkit.NewErrorMap(…)", path, table)
			continue
		}
		covered := map[string]bool{}
		for _, arg := range tableArgs {
			if id, ok := arg.(*ast.Ident); ok {
				covered[rules[id.Name]] = true
			}
			covered[ruleErr(arg, tableByName, root)] = true
		}
		for _, sentinel := range domainErrors(c, domain) {
			if !covered[sentinel] {
				c.Errorf("%s.%s has no rule in %s.%s: the domain error would answer 500", domain, sentinel, path, table)
			}
		}
		for name := range errorMaps {
			if name != table && !declared[name] {
				c.Errorf("%s: operation table %s is not declared in huma.Register(api, %s.Declare(op), …): its codes are missing from OpenAPI", path, name, name)
			}
		}
		for i, file := range parsed {
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if m := calledOn(x, "Map"); errorMaps[m] && m != table {
						c.Errorf("%s maps errors with %s: a handler maps through the domain table %s, an operation table only declares", pkgFiles[i].path, m, table)
					}
				case *ast.FuncDecl:
					if handlers[x.Name.Name] {
						handlerReturns(c, pkgFiles[i].path, x.Name.Name, x.Body, table)
					}
				}
				return true
			})
		}
		for lit, where := range literals {
			handlerReturns(c, where, "func literal", lit.Body, table)
		}
	}
}

// handlerReturns: хендлер отдаёт ошибкой nil или <таблица домена>.Map(…) — другая ошибка ушла бы клиенту 500
// или мимо кодов таблицы. Литерал внутри хендлера — своя функция: его return — не ответ хендлера.
func handlerReturns(c *check, path, handler string, body *ast.BlockStmt, table string) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if len(x.Results) == 0 {
				return true
			}
			if last := x.Results[len(x.Results)-1]; !isIdent(last, "nil") && calledOn(last, "Map") != table {
				c.Errorf("%s: handler %s returns %s: a handler returns nil or %s.Map(err) — any other error answers 500 or bypasses the codes of the table", path, handler, types.ExprString(last), table)
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
func domainErrors(c *check, domain string) []string {
	c.T.Helper()
	var names []string
	for _, decl := range c.parse("internal/domain/" + domain + "/errors.go").Decls {
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
	runRule(t)
}

func domainErrorsInErrorsGo(c *check) {
	for _, f := range c.files {
		if _, sub, ok := splitDomain(f.pkg); !ok || sub != "" || f.test {
			continue
		}
		file := c.parse(f.path)
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
							c.Errorf("%s declares %s: errors.go holds exported sentinels Err… — an unexported one has no rule in the HTTP table", f.path, id.Name)
						case named:
							c.Errorf("%s declares %s: a sentinel lives in errors.go, where the HTTP error table is checked against it", f.path, id.Name)
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
					c.Errorf("%s creates an error with errors.New outside a sentinel var Err… of errors.go: a domain error is such a sentinel, with a rule in the HTTP error table", f.path)
				case isSelector(x.Fun, byName, "fmt", "Errorf") && !wrapsError(x):
					c.Errorf("%s calls fmt.Errorf without %%w: the error loses its sentinel or cause — wrap it, a new domain error is a sentinel of errors.go", f.path)
				}
			case *ast.FuncDecl:
				recv := receiverName(x)
				switch {
				case recv != "" && x.Name.Name == "Error" && !inErrorsGo:
					c.Errorf("%s declares error type %s: domain error types live in errors.go", f.path, recv)
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
				c.Errorf("%s: error type %s does not unwrap to a sentinel of errors.go: the HTTP table maps sentinels, the type would answer 500", f.path, typ)
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
	runRule(t)
}

func subjectLiteralOwnership(c *check) {
	names := c.domains()
	for _, f := range c.files {
		if !under(f.pkg, "internal") || f.test {
			continue
		}
		ast.Inspect(c.parse(f.path), func(n ast.Node) bool {
			val, ok := stringLit(n)
			if !ok {
				return true
			}
			for _, d := range names {
				if strings.HasPrefix(val, d+".events.") && !under(f.pkg, "internal/domain/"+d) && !strings.Contains(f.pkg, "delivery/nats") {
					c.Errorf("%s contains subject literal %q — only the owning domain or a delivery/nats consumer may declare it", f.path, val)
				}
			}
			return true
		})
	}
}

// stringLit — значение строкового литерала; false — не он.
func stringLit(n ast.Node) (string, bool) {
	lit, ok := n.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	val, err := strconv.Unquote(lit.Value)
	return val, err == nil
}

// TestNoExportedSubjectInDomainRoot: subject — неэкспортируемая константа источника, у потребителя — своя
// (TestSubjectLiteralOwnership): экспорт отдал бы его потребителю мимо контракт-теста.
func TestNoExportedSubjectInDomainRoot(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func noExportedSubjectInDomainRoot(c *check) {
	for _, f := range c.files {
		if _, sub, ok := splitDomain(f.pkg); !ok || sub != "" || f.test {
			continue
		}
		for _, decl := range c.parse(f.path).Decls {
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
					if strings.HasPrefix(n.Name, "Subject") {
						c.Errorf("%s exports %q — subjects stay unexported in the source domain", f.path, n.Name)
					}
				}
			}
		}
	}
}

// firstVersion — subject с суффиксом первой версии: <домен>.events.<событие>.v1.
var firstVersion = regexp.MustCompile(`^([a-z0-9_]+)\.events\..+\.v1$`)

// TestSubjectVersions: первая версия события домена этого репо — subject без суффикса, новая — следующий номер .vN
// (event.md § Versioning): .v1 рядом с subject без суффикса дал бы два имени одной версии. Subject другого сервиса
// называет его контракт, .v1, выпущенный до канона, — строка releasedSubjects настроек сервиса.
func TestSubjectVersions(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func subjectVersions(c *check) {
	names := c.domains()
	named := map[string]bool{}
	for _, f := range c.files {
		if !under(f.pkg, "internal") || under(f.pkg, "internal/archtest") { // настройки и примеры — не subject кода
			continue
		}
		ast.Inspect(c.parse(f.path), func(n ast.Node) bool {
			val, ok := stringLit(n)
			if ok {
				named[val] = true
			}
			if m := firstVersion.FindStringSubmatch(val); ok && m != nil && slices.Contains(names, m[1]) && !slices.Contains(c.released, val) {
				c.Errorf("%s names subject %q: the first version of an event has no suffix, a new one is the next .vN; a .v1 released before the canon is a line of releasedSubjects", f.path, val)
			}
			return true
		})
	}
	for _, s := range c.released {
		if !named[s] {
			c.Errorf("releasedSubjects lists %q, and no file of the service names it: remove the line", s)
		}
	}
}

// TestDomainPackages: домен — корень, delivery/<транспорт>, infra/<адаптер> (sqlc — infra/postgres/db, моки —
// infra/<адаптер>/mocks), module и mocks: правила слоёв читают эти пакеты, подпакет с другим именем (store, usertest)
// ушёл бы из-под них. infra не знает входа; SQL — только infra/postgres: вне его из pgkit — лишь Transactor, без
// database/sql и pgx; natskit — только delivery/nats: публикация идёт через outbox.
func TestDomainPackages(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func domainPackages(c *check) {
	for _, f := range c.files {
		_, sub, ok := splitDomain(f.pkg)
		if !ok || f.test {
			continue
		}
		parts := strings.Split(sub, "/")
		switch {
		case sub == "", sub == "module", sub == "mocks", parts[0] == "delivery", sub == "infra/postgres/db":
		case parts[0] == "infra" && (len(parts) == 2 || len(parts) == 3 && parts[2] == "mocks"):
		default:
			c.Errorf("%s: a domain is its root, delivery/<transport>, infra/<adapter>, module and mocks (an adapter's mocks — infra/<adapter>/mocks) — rules of layers read only these packages", f.pkg)
		}
		repository := under(sub, "infra/postgres")
		for _, imp := range f.imports {
			impDomain, impSub, inDomain := splitDomain(strings.TrimPrefix(imp, module+"/"))
			switch {
			case parts[0] == "infra" && inDomain && under(impSub, "delivery"):
				c.Errorf("%s imports %s: infra serves the usecase's ports and knows no input of %s", f.path, imp, impDomain)
			case !repository && (under(imp, "database/sql") || under(imp, "github.com/jackc/pgx")):
				c.Errorf("%s imports %s: SQL lives in infra/postgres, the repository behind the usecase's port", f.path, imp)
			case parts[0] != "delivery" && imp == natskitPath:
				c.Errorf("%s imports natskit: a domain publishes through its outbox and subscribes in delivery/nats", f.path)
			}
		}
		if repository || !slices.Contains(f.imports, pgkitPath) {
			continue
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && !isSelector(sel, byName, pgkitPath, "Transactor") && isSelector(sel, byName, pgkitPath, sel.Sel.Name) {
				c.Errorf("%s uses pgkit.%s: outside infra/postgres a domain takes only pgkit.Transactor — SQL lives in the repository", f.path, sel.Sel.Name)
			}
			return true
		})
	}
}

const pgkitPath = golibinfra + "/pgkit"

// determinismKits — пакеты, вместо которых домен берёт кит: ID — idkit, токены — authnkit, время — clockkit.Clock
// из зависимостей.
var determinismKits = map[string]string{
	"crypto/rand":              "authnkit or idkit",
	"math/rand":                "a kit",
	"math/rand/v2":             "a kit",
	"github.com/google/uuid":   "idkit",
	"github.com/gofrs/uuid":    "idkit",
	"github.com/oklog/ulid/v2": "idkit",
}

// TestDomainDeterminism: ID, случайность и время домен берёт у китов и из зависимостей — тест подменяет их, а
// clockkit.Real{} и crypto/rand мимо инжекции — нет. Состояния между запросами в переменных корня нет: переменная
// корня — sentinel Err… (errors.go), TrustedErrors, исход outcome.New, шаблон regexp.MustCompile, значение
// структуры T{…} типа корня или pkg/ или проверка var _, и ни одна функция корня её не пишет.
func TestDomainDeterminism(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func domainDeterminism(c *check) {
	vars := map[string]map[string]bool{} // домен → переменные корня
	type rootFunc struct {
		domain, path string
		fn           *ast.FuncDecl
	}
	var funcs []rootFunc
	for _, f := range c.files {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || f.test {
			continue
		}
		for _, imp := range f.imports {
			if kit, ok := determinismKits[imp]; ok {
				c.Errorf("%s imports %s: a domain takes IDs, tokens and randomness from %s, so tests can pin them", f.path, imp, kit)
			}
		}
		file := c.parse(f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok && isSelector(e, byName, golibinfra+"/clockkit", "Real") {
				c.Errorf("%s uses clockkit.Real: a domain reads time from the injected clockkit.Clock (Clock in its dependencies)", f.path)
			}
			return true
		})
		if sub != "" {
			continue
		}
		if vars[domain] == nil {
			vars[domain] = map[string]bool{}
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				funcs = append(funcs, rootFunc{domain, f.path, d})
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						if name.Name == "_" {
							continue
						}
						vars[domain][name.Name] = true
						fixed := i < len(vs.Values) && fixedValue(vs.Values[i], byName)
						if !fixed && name.Name != "TrustedErrors" && path.Base(f.path) != "errors.go" {
							c.Errorf("%s declares package var %s: state between requests lives in the database or a kit; a fixed value is a const or a function", f.path, name.Name)
						}
					}
				}
			}
		}
	}
	for _, rf := range funcs {
		for _, name := range packageWrites(rf.fn) {
			if vars[rf.domain][name] {
				c.Errorf("%s: %s writes package var %s: state between requests lives in the database or a kit", rf.path, rf.fn.Name.Name, name)
			}
		}
	}
}

// fixedValue — значение переменной корня, которое не состояние: исход outcome.New("…"), шаблон regexp.MustCompile("…"),
// значение структуры T{…} типа корня или pkg/ (не sync и sync/atomic: их методы пишут значение).
func fixedValue(expr ast.Expr, byName map[string]string) bool {
	switch x := expr.(type) {
	case *ast.CallExpr:
		pkg, name := kitCall(x, byName)
		return pkg == module+"/pkg/outcome" && name == "New" || pkg == "regexp" && name == "MustCompile"
	case *ast.CompositeLit:
		switch t := x.Type.(type) {
		case *ast.Ident:
			return true
		case *ast.SelectorExpr:
			id, ok := t.X.(*ast.Ident)
			return ok && strings.HasPrefix(byName[id.Name], module+"/pkg/")
		}
	}
	return false
}

// packageWrites — имена, которые функция пишет (=, op=, ++, --, &x, поле или элемент x) и не объявляет сама: запись в
// переменную пакета.
func packageWrites(fn *ast.FuncDecl) []string {
	local := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			for _, name := range x.Names {
				local[name.Name] = true
			}
		case *ast.ValueSpec:
			for _, name := range x.Names {
				local[name.Name] = true
			}
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, lhs := range x.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						local[id.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if id, ok := e.(*ast.Ident); ok {
						local[id.Name] = true
					}
				}
			}
		}
		return true
	})
	var out []string
	for name := range writes(fn) {
		if !local[name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// piiWords — слово имени поля payload (любое, целиком: Zip — не IP), которому место в данных поставщика, а не в
// событии; piiFused — слитные формы. Факт (bool, число, время) и ID последним словом — не данные (EmailVerified, TokenID).
var (
	piiWords = []string{"Email", "Phone", "Password", "Token", "Secret", "Name", "Address", "Addr", "IP"}
	piiFused = []string{"username", "surname", "nickname", "fullname", "firstname", "lastname", "email"}
)

// fxTagKeys — теги полей зависимостей fx (структура с fx.In или fx.Out); tagKey — ключ тега поля.
var (
	fxTagKeys = []string{"name", "group", "optional"}
	tagKey    = regexp.MustCompile(`(\w+):"`)
)

// TestRootTypes: типы корня — без тегов транспорта: форму провода задают DTO delivery, у payload событий тегов нет
// (провод — Go-имена); теги fx у зависимостей — способ fx. Payload — тип, который корень публикует (литерал в
// ….Publish или в переменной для него), типы events*.go и типы корня в их полях: ID и факты, а адрес, имя, телефон, IP
// и секреты потребитель берёт по ID портом.
func TestRootTypes(t *testing.T) {
	t.Parallel()
	runRule(t)
}

func rootTypes(c *check) {
	type rootFile struct {
		path, domain string
		file         *ast.File
	}
	var root []rootFile
	payloads := map[string]map[string]bool{}           // домен → типы payload
	structs := map[string]map[string]*ast.StructType{} // домен → структуры корня
	for _, f := range c.files {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "" || f.test {
			continue
		}
		file := c.parse(f.path)
		root = append(root, rootFile{f.path, domain, file})
		if payloads[domain] == nil {
			payloads[domain], structs[domain] = map[string]bool{}, map[string]*ast.StructType{}
		}
		events := strings.HasPrefix(path.Base(f.path), "events")
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TypeSpec:
				if st, ok := n.Type.(*ast.StructType); ok {
					structs[domain][n.Name.Name] = st
					if events {
						payloads[domain][n.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				for _, name := range published(n) {
					payloads[domain][name] = true
				}
			}
			return true
		})
	}
	for domain, types := range payloads { // тип корня в поле payload — тоже payload
		for grown := true; grown; {
			grown = false
			for name := range types {
				for _, field := range structFields(structs[domain][name]) {
					if inner := baseIdent(field.Type); structs[domain][inner] != nil && !types[inner] {
						types[inner], grown = true, true
					}
				}
			}
		}
	}
	for _, rf := range root {
		fxName := ""
		for name, p := range importsByName(rf.file) {
			if p == "go.uber.org/fx" {
				fxName = name
			}
		}
		ast.Inspect(rf.file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			fxDeps := embedsFx(st, fxName)
			for _, field := range st.Fields.List {
				if field.Tag != nil && (!fxDeps || !fxTag(field.Tag.Value)) {
					c.Errorf("%s: %s has a field tag %s: the wire form lives in DTOs of delivery, an event payload has none", rf.path, ts.Name.Name, field.Tag.Value)
				}
				for _, name := range field.Names {
					if payloads[rf.domain][ts.Name.Name] && personalData(name.Name, field.Type) {
						c.Errorf("%s: event %s carries %s: a payload holds IDs and facts — the consumer reads personal data and secrets by ID through a port", rf.path, ts.Name.Name, name.Name)
					}
				}
			}
			return true
		})
	}
}

// published — типы, которые функция передаёт в ….Publish: литералом в вызове или переменной, которой функция
// присвоила литерал (ev := T{…}; Publish(…, ev)).
func published(fn *ast.FuncDecl) []string {
	lits := map[string]string{}
	var out []string
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && i < len(n.Rhs) && literalType(n.Rhs[i]) != "" {
					lits[id.Name] = literalType(n.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			for i, id := range n.Names {
				if i < len(n.Values) && literalType(n.Values[i]) != "" {
					lits[id.Name] = literalType(n.Values[i])
				}
			}
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Publish" {
				for _, arg := range n.Args {
					name := literalType(arg)
					if id, ok := arg.(*ast.Ident); ok {
						name = lits[id.Name]
					}
					if name != "" {
						out = append(out, name)
					}
				}
			}
		}
		return true
	})
	return out
}

// structFields — поля структуры; nil — не структура корня.
func structFields(st *ast.StructType) []*ast.Field {
	if st == nil {
		return nil
	}
	return st.Fields.List
}

// baseIdent — имя типа пакета под указателем, срезом, массивом и значением map; "" — не тип пакета.
func baseIdent(t ast.Expr) string {
	switch t := t.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return baseIdent(t.X)
	case *ast.ArrayType:
		return baseIdent(t.Elt)
	case *ast.MapType:
		return baseIdent(t.Value)
	}
	return ""
}

// literalType — имя типа литерала T{…} или &T{…}; "" — не литерал типа пакета.
func literalType(expr ast.Expr) string {
	if u, ok := expr.(*ast.UnaryExpr); ok && u.Op == token.AND {
		expr = u.X
	}
	if lit, ok := expr.(*ast.CompositeLit); ok {
		if id, ok := lit.Type.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// embedsFx — структура зависимостей fx: встраивает fx.In или fx.Out пакета, импортированного как fxName.
func embedsFx(st *ast.StructType, fxName string) bool {
	for _, field := range st.Fields.List {
		if sel, ok := field.Type.(*ast.SelectorExpr); ok && len(field.Names) == 0 && isIdent(sel.X, fxName) &&
			(sel.Sel.Name == "In" || sel.Sel.Name == "Out") {
			return true
		}
	}
	return false
}

// fxTag — тег поля, все ключи которого — теги fx.
func fxTag(lit string) bool {
	tag, err := strconv.Unquote(lit)
	if err != nil {
		return false
	}
	keys := tagKey.FindAllStringSubmatch(tag, -1)
	for _, k := range keys {
		if !slices.Contains(fxTagKeys, k[1]) {
			return false
		}
	}
	return len(keys) > 0
}

// personalData — слово имени (CamelCase и _) из piiWords, и во множественном числе, или слитная форма (EMail,
// Username); факт по типу поля и ID последним словом — нет.
func personalData(name string, typ ast.Expr) bool {
	var words []string
	for _, part := range strings.Split(name, "_") {
		if part != "" {
			words = append(words, camelWords(part)...)
		}
	}
	if last := words[len(words)-1]; strings.EqualFold(last, "ID") || strings.EqualFold(last, "IDs") || factType(typ) {
		return false
	}
	for i, w := range words {
		if slices.Contains(piiFused, strings.ToLower(w)) || i > 0 && strings.EqualFold(words[i-1]+w, "email") ||
			slices.ContainsFunc(piiWords, func(p string) bool {
				return strings.EqualFold(w, p) || strings.EqualFold(w, p+"s") || strings.EqualFold(w, p+"es")
			}) {
			return true
		}
	}
	return false
}

// factType — тип факта, а не данных: bool, числа, время (и указатель на них).
func factType(t ast.Expr) bool {
	switch t := t.(type) {
	case *ast.StarExpr:
		return factType(t.X)
	case *ast.Ident:
		return slices.Contains([]string{
			"bool", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16",
			"uint32", "uint64", "float32", "float64",
		}, t.Name)
	case *ast.SelectorExpr:
		return isIdent(t.X, "time") && (t.Sel.Name == "Time" || t.Sel.Name == "Duration")
	}
	return false
}

// camelWords — слова имени CamelCase: ClientIP → Client IP, IPAddress → IP Address, UserIPs → User IPs.
func camelWords(name string) []string {
	r := []rune(name)
	var words []string
	start := 0
	for i := 1; i < len(r); i++ {
		lowerUpper := !unicode.IsUpper(r[i-1]) && unicode.IsUpper(r[i])
		acronymEnd := unicode.IsUpper(r[i-1]) && unicode.IsUpper(r[i]) && i+1 < len(r) && unicode.IsLower(r[i+1]) &&
			(i+2 != len(r) || r[i+1] != 's')
		if lowerUpper || acronymEnd {
			words = append(words, string(r[start:i]))
			start = i
		}
	}
	return append(words, string(r[start:]))
}

// TestTestsAssert: покрытие считает исполненный код, поэтому тест без единого утверждения поднимает его,
// ничего не проверив, — в любом пакете модуля: покрытие набирают и тесты pkg/ и cmd/, и fuzz-цели (сиды f.Add
// идут в обычном go test). Тест домена и платформы утверждает require, assert или проверкой тестового кита
// (rule tests-pipeline); остальным засчитываются ещё t.Error*, t.Fatal*, t.Fail* и t, отданный проверке
// тестового кита — его пакета (archkit.Check, testkit.MigrationLadder) или значения из него
// (envAllowlist.Check), а не любому вызову (fmt.Println(t)). Ожидание мока тоже утверждение, и мягкое — ничего не
// обещает: .Maybe() (вызова может не быть) и цепочка EXPECT() без .Once(), .Twice() или .Times(n) (вызовов
// сколько угодно) — отказ; цепочку можно продолжить и вторым выражением через переменную.
func TestTestsAssert(t *testing.T) {
	t.Parallel()
	for src, want := range map[string][]string{
		"m.EXPECT().Get(id).Return(u, nil).Once()":                       nil,
		"m.EXPECT().Get(id).Return(u, nil)":                              {"EXPECT().Get without .Once(), .Twice() or .Times(n)"},
		"m.EXPECT().Get(id).Return(u, nil).Maybe()":                      {".Maybe()"},
		"call := m.EXPECT().Get(id)\ncall.Return(u, nil).Times(2)":       nil,
		"call := m.EXPECT().Get(id).Return(u, nil)\ncall.Return(u, nil)": {"EXPECT().Get without .Once(), .Twice() or .Times(n)"},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "", "package p\n\nfunc f() {\n"+src+"\n}\n", 0)
		require.NoError(t, err)
		require.Equal(t, want, softExpectations(file.Decls[0].(*ast.FuncDecl).Body), src)
	}
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
				if !ok || fn.Body == nil {
					continue
				}
				test := strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" || strings.HasPrefix(fn.Name.Name, "Fuzz")
				if fn.Recv == nil && test && !asserts(fn.Body, f.byName, helpers, kits, bare) {
					t.Errorf("%s: %s asserts nothing — no require, assert, Assert*/Require* of a test kit or testify/mock, or helper of the package that asserts (rule tests-pipeline): such a test raises coverage without checking anything", f.path, fn.Name.Name)
				}
				for _, soft := range softExpectations(fn.Body) {
					t.Errorf("%s: %s: %s — a soft expectation of a mock promises nothing: a call that never happens or happens again passes; exact .Once(), .Twice() or .Times(n) (rule tests-pipeline)", f.path, fn.Name.Name, soft)
				}
			}
		}
	}
}

// softExpectations — мягкие ожидания моков в теле: .Maybe() и цепочка EXPECT() без .Once(), .Twice() или .Times(n) —
// одним выражением или продолженная через переменную (call := m.EXPECT().X(); call.Return(…).Once()); значение —
// что нашлось.
func softExpectations(body ast.Node) []string {
	var soft, pending []string // pending — переменные с цепочкой EXPECT() без числа вызовов
	expectOf, timed := map[string]string{}, map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		var expr ast.Expr
		variable := ""
		switch x := n.(type) {
		case *ast.ExprStmt:
			expr = x.X
		case *ast.AssignStmt:
			if len(x.Rhs) != 1 {
				return true
			}
			expr = x.Rhs[0]
			if id, ok := x.Lhs[0].(*ast.Ident); ok && len(x.Lhs) == 1 {
				variable = id.Name
			}
		default:
			return true
		}
		var chain []string // методы цепочки снаружи внутрь: Once, Return, GetByID, EXPECT
		for {
			call, ok := expr.(*ast.CallExpr)
			if !ok {
				break
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			chain = append(chain, sel.Sel.Name)
			expr = sel.X
		}
		times := func(chain []string) bool {
			return slices.ContainsFunc(chain, func(m string) bool { return m == "Once" || m == "Twice" || m == "Times" })
		}
		expect := slices.Index(chain, "EXPECT")
		switch {
		case slices.Contains(chain, "Maybe"):
			soft = append(soft, ".Maybe()")
		case expect > 0 && !times(chain[:expect]) && variable != "":
			pending = append(pending, variable)
			expectOf[variable] = chain[expect-1]
		case expect > 0 && !times(chain[:expect]):
			soft = append(soft, "EXPECT()."+chain[expect-1]+" without .Once(), .Twice() or .Times(n)")
		case expect < 0 && times(chain):
			if id, ok := expr.(*ast.Ident); ok {
				timed[id.Name] = true
			}
		}
		return true
	})
	for _, variable := range pending {
		if !timed[variable] {
			soft = append(soft, "EXPECT()."+expectOf[variable]+" without .Once(), .Twice() or .Times(n)")
		}
	}
	return soft
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
