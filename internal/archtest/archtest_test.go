// Package archtest машинно проверяет слоевые инварианты доменов
// (rule domain-architecture): чистоту entity/policy/dto/errors/events,
// транспортную изоляцию usecase, huma-уникальность HTTP-DTO, владение
// subject-литералами и запрет time.Now() в доменном коде.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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

// ioStdlib — stdlib с сайд-эффектами: чистым слоям запрещён даже он
// (net/mail и net/url — парсеры, им можно).
var ioStdlib = []string{"net", "net/http", "net/smtp", "os", "io", "bufio", "database/sql", "syscall", "os/exec"}

// pureLayerAllowed — что можно импортить из entity/policy/dto/errors/events:
// side-effect-free stdlib + платформенные value-примитивы. Всё остальное — запрещено.
func pureLayerAllowed(imp string) bool {
	switch {
	case !strings.Contains(imp, "."):
		return !slices.Contains(ioStdlib, imp) // side-effect-free stdlib only
	case imp == module+"/internal/platform/identity",
		imp == module+"/internal/platform/shared",
		imp == module+"/pkg/policy":
		return true
	}
	return false
}

func TestPureLayersHaveNoIO(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	pure := []string{"entity.go", "policy.go", "dto.go", "errors.go", "events.go"}
	for _, d := range domains(t) {
		for _, name := range pure {
			path := filepath.Join(root, "internal", "domain", d, name)
			if _, err := os.Stat(path); err != nil {
				continue // optional file (rule domain-architecture: existence table)
			}
			for _, imp := range parseImports(t, path) {
				require.True(t, pureLayerAllowed(imp),
					"%s/%s imports %q — pure layers allow stdlib + platform/{identity,shared} + pkg/policy only", d, name, imp)
			}
		}
	}
}

// usecaseForbidden — транспорт и низкоуровневая инфраструктура, которых
// usecase не должен видеть (ctx-геттеры — через platform/identity).
var usecaseForbidden = []string{
	"git.stit.tech/stit-core/golibinfra/httpkit",
	"git.stit.tech/stit-core/golibinfra/natskit",
	"github.com/danielgtaylor/huma",
	"github.com/jackc/pgx",
	"net/http",
}

func TestUsecaseIsTransportAgnostic(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, d := range domains(t) {
		matches, err := filepath.Glob(filepath.Join(root, "internal", "domain", d, "usecase*.go"))
		require.NoError(t, err)
		for _, path := range matches {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, imp := range parseImports(t, path) {
				for _, bad := range usecaseForbidden {
					require.False(t, strings.HasPrefix(imp, bad),
						"%s imports %q — usecase must not know transport/driver packages", path, imp)
				}
				require.False(t, strings.Contains(imp, "/internal/domain/"+d+"/"),
					"%s imports own subpackage %q — usecase depends on ports, not delivery/infra", path, imp)
			}
		}
	}
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
// во всём internal/domain и internal/jobs (кроме тестов).
func TestNoTimeNowInDomains(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, dir := range []string{filepath.Join(root, "internal", "domain"), filepath.Join(root, "internal", "jobs")} {
		walkGoFiles(t, dir, func(path string) {
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
