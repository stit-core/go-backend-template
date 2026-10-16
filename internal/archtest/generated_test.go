package archtest

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMain: тест archtest не пропускается — t.Skip сделал бы его проверку зелёной без следа в выводе гейта; снять
// проверку — правка гейта (CLAUDE.md § Границы). Пропуски ищутся до тестов, а не тестом: тест с t.Skip первой
// строкой пропустил бы и себя.
func TestMain(m *testing.M) {
	if skips := skipCalls(); len(skips) > 0 {
		fmt.Fprintln(os.Stderr, "archtest: тест гейта не пропускается — Skip, SkipNow и Skipf в пакете:")
		for _, at := range skips {
			fmt.Fprintln(os.Stderr, "  "+at)
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// skipCalls — Skip, SkipNow и Skipf (и вызов, и значение метода) в файлах пакета: «файл:строка:колонка».
func skipCalls() []string {
	files, err := filepath.Glob("*.go")
	if err != nil {
		return []string{err.Error()}
	}
	fset := token.NewFileSet()
	var found []string
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			return []string{err.Error()}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && slices.Contains([]string{"Skip", "SkipNow", "Skipf"}, sel.Sel.Name) {
				found = append(found, fset.Position(sel.Sel.Pos()).String())
			}
			return true
		})
	}
	return found
}

// generatedHeader — заголовок сгенерированного файла: по нему файл выпадает из покрытия
// (scripts/tools/cover.sh) и из lint.
var generatedHeader = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

// sqlcFiles — файлы, которые sqlc пишет в каталог вывода домена db: общие и по файлу запросов из соседнего
// queries/ (users.sql → users.sql.go).
func sqlcFiles(t *testing.T, db string) map[string]bool {
	t.Helper()
	files := map[string]bool{"db.go": true, "models.go": true, "querier.go": true, "copyfrom.go": true, "batch.go": true}
	queries, err := filepath.Glob(filepath.Join(repoRoot(t), path.Dir(db), "queries", "*.sql"))
	require.NoError(t, err)
	for _, q := range queries {
		files[filepath.Base(q)+".go"] = true
	}
	return files
}

// TestGeneratedHeaderOnlyInGeneratorOutput: заголовок сгенерированного выводит файл из покрытия и lint, поэтому
// он стоит только у вывода генераторов гейта — sqlc (каталог infra/postgres/db домена, файлы, которые sqlc
// пишет) и mockery (mocks), — а в каталоге вывода sqlc нет других файлов: рукописный файл под заголовком обошёл
// бы гейт, а без него — make gen. Сгенерированный клиент pkg/ — генератор в make gen и его каталог в этом тесте:
// правка гейта.
func TestGeneratedHeaderOnlyInGeneratorOutput(t *testing.T) {
	t.Parallel()
	sqlc := map[string]map[string]bool{} // каталог вывода sqlc → его файлы
	for _, f := range goFiles(t) {
		src, err := os.ReadFile(filepath.Join(repoRoot(t), f.path))
		require.NoError(t, err)
		gen := generatedHeader.Match(src)
		switch {
		case strings.HasSuffix(f.pkg, "/infra/postgres/db"):
			if sqlc[f.pkg] == nil {
				sqlc[f.pkg] = sqlcFiles(t, f.pkg)
			}
			if !gen || !sqlc[f.pkg][path.Base(f.path)] {
				t.Errorf("%s: sqlc writes no such file — infra/postgres/db holds only the output of make gen (db.go, models.go, <file of queries/>.sql.go)", f.path)
			}
		case gen && path.Base(f.pkg) != "mocks":
			t.Errorf("%s: the generated-code header takes the file out of coverage and lint — only the output of the generators of make gen carries it: sqlc (infra/postgres/db), mockery (mocks)", f.path)
		}
	}
}

// TestMainOnlyEnters: main бинаря cmd/ — вход: присваивания, вызовы и выход по ошибке или подкоманде (if без else,
// чьё тело кончается os.Exit или return). Цикл, switch, select, go, else, вложенный if и литерал функции (вызванный на
// месте, отложенный или переданный — его тело cover-check считает телом main) — логика: её место — функции пакета, их
// судит cover-check, а main он не видит (main исполняют перезапущенный тестовый бинарь и образ). Литерал функции на
// уровне пакета cmd/ (var run = func…, fx.Provide(func…)) — то же: его тело вне любой функции профиля.
func TestMainOnlyEnters(t *testing.T) {
	t.Parallel()
	inMain := func(body string) string { return "func main() {\n" + body + "\n}\n" }
	for src, logic := range map[string]bool{
		inMain("log, err := newLog()\nif err != nil {\n\tprintln(err)\n\tos.Exit(1)\n}\nos.Exit(run(log))"): false,
		inMain("if len(os.Args) > 1 {\n\treturn\n}\nfx.New().Run()"):                                        false,
		inMain("for _, a := range os.Args {\n\tuse(a)\n}"):                                                  true,
		inMain("if ok {\n\tos.Exit(0)\n} else {\n\tos.Exit(1)\n}"):                                          true,
		inMain("if ok {\n\tuse(1)\n}"):                                                                      true,
		inMain("if ok {\n\tif more {\n\t\tos.Exit(2)\n\t}\n\tos.Exit(1)\n}"):                                true,
		inMain("switch mode {\ncase 1:\n\tuse(1)\n}"):                                                       true,
		inMain("func() {\n\tfor _, a := range os.Args {\n\t\tuse(a)\n\t}\n}()"):                             true,
		inMain("os.Exit(run(func() int {\n\treturn 1\n}))"):                                                 true,
		"var service = fx.Options(fx.Provide(newMeter))\n\n" + inMain("fx.New(service).Run()"):              false,
		"var run = func() int {\n\treturn 1\n}\n\n" + inMain("os.Exit(run())"):                              true,
		"var service = fx.Options(fx.Invoke(func() {}))\n\n" + inMain("fx.New(service).Run()"):              true,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "", "package main\n\n"+src, 0)
		require.NoError(t, err)
		require.Equal(t, logic, len(cmdLogic(file)) > 0, src)
	}
	for _, f := range goFiles(t) {
		if f.test || !strings.HasPrefix(f.pkg, "cmd/") {
			continue
		}
		for _, logic := range cmdLogic(parseGo(t, f.path)) {
			t.Errorf("%s: %s: it goes to a named function of the package — cover-check judges functions, but not main "+
				"and not a literal outside them", f.path, logic)
		}
	}
}

// cmdLogic — логика файла cmd/, которую cover-check не судит: в теле main и литералы функций на уровне пакета.
func cmdLogic(file *ast.File) []string {
	var logic []string
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == "main" && d.Body != nil {
				for _, l := range mainLogic(d.Body) {
					logic = append(logic, "main has logic ("+l+")")
				}
			}
		case *ast.GenDecl:
			for range funcLits(d) {
				logic = append(logic, "a function literal at the level of the package")
			}
		}
	}
	return logic
}

// mainLogic — логика в теле main: инструкции логики и литералы функций.
func mainLogic(body *ast.BlockStmt) []string {
	logic := stmtLogic(body.List, true)
	for range funcLits(body) {
		logic = append(logic, "function literal")
	}
	return logic
}

// funcLits — литералы функций в n; вложенный в литерал — его часть.
func funcLits(n ast.Node) int {
	count := 0
	ast.Inspect(n, func(n ast.Node) bool {
		_, lit := n.(*ast.FuncLit)
		if lit {
			count++
		}
		return !lit
	})
	return count
}

// stmtLogic — инструкции логики (top — верхний уровень main): всё, кроме присваивания, объявления, вызова, return и
// defer, и if, который только выходит.
func stmtLogic(stmts []ast.Stmt, top bool) []string {
	var logic []string
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.AssignStmt, *ast.DeclStmt, *ast.ExprStmt, *ast.ReturnStmt, *ast.DeferStmt:
		case *ast.IfStmt:
			if !top || s.Else != nil || !exits(s.Body) {
				logic = append(logic, "if that does not only exit")
				continue
			}
			logic = append(logic, stmtLogic(s.Body.List, false)...)
		default:
			logic = append(logic, fmt.Sprintf("%T", stmt))
		}
	}
	return logic
}

// exits — тело кончается выходом: return или os.Exit(…).
func exits(body *ast.BlockStmt) bool {
	if len(body.List) == 0 {
		return false
	}
	switch last := body.List[len(body.List)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.ExprStmt:
		call, ok := last.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "os" && sel.Sel.Name == "Exit"
	}
	return false
}

// TestBuildConstraints: гейт проверяет код в сборке с тегом integration, образ собирает без тегов. Файл,
// который ограничение сборки исключает из сборки гейта, выпал бы из vet, lint, тестов и покрытия, а в
// бинарь попал бы; ограничение у не-теста разводит то, что проверено, и то, что собрано. Поэтому
// ограничение — только //go:build integration и только у *_test.go, а суффикс имени (_windows.go,
// _arm64.go) не исключает файл из сборки гейта на этой машине. Каталоги testdata и «_»-файлы go не видит.
func TestBuildConstraints(t *testing.T) {
	t.Parallel()
	gate := build.Default
	gate.BuildTags = []string{"integration"}
	for _, f := range goFiles(t) {
		if slices.ContainsFunc(strings.Split(f.path, "/"), func(part string) bool {
			return part == "testdata" || strings.HasPrefix(part, "_")
		}) {
			continue
		}
		file := filepath.Join(repoRoot(t), f.path)
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, line := range buildLines(src) {
			if expr, err := constraint.Parse(line); err != nil || !f.test || expr.String() != "integration" {
				t.Errorf("%s: %q — a build constraint is //go:build integration on a *_test.go only: "+
					"another one takes the file out of vet, lint, tests and coverage of the gate or out of the image", f.path, line)
			}
		}
		ok, err := gate.MatchFile(filepath.Dir(file), filepath.Base(file))
		require.NoError(t, err, f.path)
		if !ok {
			t.Errorf("%s: the build of the gate (tag integration) skips the file: its name (_windows.go, _arm64.go) or "+
				"constraint takes it out of vet, lint, tests and coverage", f.path)
		}
	}
}

// buildLines — строки ограничений сборки (//go:build и // +build) в заголовке файла до package.
func buildLines(src []byte) []string {
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			break
		}
		if constraint.IsGoBuild(line) || constraint.IsPlusBuild(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

// linterDirective — директива, которой линтер глушит себя мимо //nolint: revive:disable… и своя директива
// gosec (её gosec не слушает — nosec: true в .golangci.yml, — а читатель примет за действующую).
var linterDirective = regexp.MustCompile(`^(//|/\*)\s*revive:|#nosec`)

// TestLinterDirectives: ослабление lint — только //nolint с именем включённого линтера и причиной
// (nolintlint): директива самого линтера глушит его без имени и причины.
func TestLinterDirectives(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(repoRoot(t), f.path), nil, parser.ParseComments)
		require.NoError(t, err)
		for _, group := range file.Comments {
			for _, c := range group.List {
				if linterDirective.MatchString(c.Text) {
					t.Errorf("%s:%d: %q — a linter directive past //nolint: weaken lint only by //nolint:<linter> // <reason>",
						f.path, fset.Position(c.Pos()).Line, c.Text)
				}
			}
		}
	}
}
