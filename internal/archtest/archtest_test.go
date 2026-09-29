// Package archtest машинно держит границы и слоевые инварианты доменов (rule
// domain-architecture) для любого каталога internal/domain/*: импорты между доменами и
// слоями, чистоту корня домена, huma-уникальность HTTP-DTO, владение subject-литералами и
// запрет time.Now(). На archtest //nolint не действует.
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

// TestHTTPDTONamesGloballyUnique ловит huma-коллизию на этапе теста, а не
// паникой при старте: имена экспортируемых типов delivery/http уникальны глобально.
func TestHTTPDTONamesGloballyUnique(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	seen := map[string]string{} // type name -> first location
	for _, d := range domains(t) {
		dir := filepath.Join(root, "internal", "domain", d, "delivery", "http")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoError(t, err)
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					name := ts.Name.Name
					// huma registers request/response types by name; Handler/Usecase/View stay local.
					if !ast.IsExported(name) ||
						(!strings.HasSuffix(name, "Input") && !strings.HasSuffix(name, "Output")) {
						continue
					}
					prev, dup := seen[name]
					require.False(t, dup,
						"HTTP DTO type %q declared in both %s and %s — huma registry panics on name collision", name, prev, path)
					seen[name] = path
				}
			}
		}
	}
}

// TestSubjectLiteralOwnership: литерал "<domain>.events." встречается только в
// самом домене-источнике или в delivery/nats консьюмеров; exported Subject* у
// источника запрещён (consumers обязаны держать свою константу).
func TestSubjectLiteralOwnership(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	names := domains(t)
	walkGoFiles(t, filepath.Join(root, "internal"), func(path string) {
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
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
				inOwner := strings.HasPrefix(rel, filepath.Join("internal", "domain", d)+string(os.PathSeparator))
				inConsumerNATS := strings.Contains(rel, filepath.Join("delivery", "nats"))
				require.True(t, inOwner || inConsumerNATS,
					"%s contains subject literal %q — only the owning domain or a delivery/nats consumer may declare it", rel, val)
			}
			return true
		})
	})
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

// TestNoTimeNowInDomains: детерминизм — clockkit.Clock вместо time.Now()
// во всём internal/domain (кроме тестов).
func TestNoTimeNowInDomains(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal", "domain"), func(path string) {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if ok && ident.Name == "time" && sel.Sel.Name == "Now" {
				t.Errorf("%s calls time.Now() — inject clockkit.Clock (rule domain-architecture)", path)
			}
			return true
		})
	})
}

func walkGoFiles(t *testing.T, root string, fn func(path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "mocks" || d.Name() == "db" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			fn(path)
		}
		return nil
	})
	require.NoError(t, err)
}
