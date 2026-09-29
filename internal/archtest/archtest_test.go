// Package archtest машинно держит границы и слоевые инварианты доменов (rule
// domain-architecture) для любого каталога internal/domain/*: импорты между доменами и
// слоями, чистоту корня домена, адаптеры и таблицы между доменами, huma-уникальность
// HTTP-типов, владение subject-литералами, запрет time.Now() и тестов без утверждений. На archtest
// //nolint не действует.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
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

// importsByName — пакеты файла по имени, под которым файл их видит: алиас, "." или элемент пути.
func importsByName(f *ast.File) map[string]string {
	byName := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		byName[name] = p
	}
	return byName
}

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

// TestHTTPDTONamesGloballyUnique: huma регистрирует схемы по имени Go-типа на весь API, и
// одноимённые типы двух доменов роняют старт паникой. Handler и Usecase — канон пакета, в
// схемы не попадают.
func TestHTTPDTONamesGloballyUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]string{} // имя типа → файл
	for _, f := range goFiles(t) {
		if _, sub, ok := splitDomain(f.pkg); !ok || sub != "delivery/http" || f.test {
			continue
		}
		for _, decl := range parseGo(t, f.path).Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				name := spec.(*ast.TypeSpec).Name.Name
				if !ast.IsExported(name) || name == "Handler" || name == "Usecase" {
					continue
				}
				if prev, dup := seen[name]; dup {
					t.Errorf("type %s is declared in both %s and %s: huma names schemas by Go type, prefix it with the domain", name, prev, f.path)
				}
				seen[name] = f.path
			}
		}
	}
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

// wallClock — функции time, которые читают или ждут настенные часы.
var wallClock = []string{"Now", "Since", "Until", "After", "AfterFunc", "Tick", "NewTimer", "NewTicker", "Sleep"}

// TestNoWallClockInDomains: детерминизм — clockkit.Clock вместо настенных часов во всём
// internal/domain (кроме тестов). Имя пакета берётся из импортов файла: алиас не прячет вызов.
func TestNoWallClockInDomains(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if _, _, ok := splitDomain(f.pkg); !ok || f.test {
			continue
		}
		file := parseGo(t, f.path)
		byName := importsByName(file)
		if byName["."] == "time" {
			t.Errorf("%s dot-imports time: the wall clock would hide behind it", f.path)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !slices.Contains(wallClock, sel.Sel.Name) {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && byName[id.Name] == "time" {
				t.Errorf("%s calls time.%s: the domain reads time through an injected clockkit.Clock (rule domain-architecture)", f.path, sel.Sel.Name)
			}
			return true
		})
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
