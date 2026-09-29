package archtest

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// closureAllowlist — identity-closure между доменами в текущем коде.
var closureAllowlist = allowlist{
	"internal/domain/auth/module/module.go: *user.Usecase → auth.PasswordVerifier":                        "ADP-02",
	"internal/domain/auth/module/module.go: *user.Usecase → auth.PasswordSetter":                          "ADP-02",
	"internal/domain/verification/module/module.go: *user.Usecase → verification.PasswordSetter":          "ADP-02",
	"internal/domain/verification/module/module.go: *user.Usecase → verification.EmailVerificationSetter": "ADP-02",
}

// TestNoCrossDomainIdentityClosure: `func(uc *user.Usecase) auth.Port { return uc }` отдаёт
// потребителю чужие ошибки и типы мимо маппинга (ADP-02) — порт между доменами реализует
// типовой адаптер в module/adapters.go.
func TestNoCrossDomainIdentityClosure(t *testing.T) {
	t.Parallel()
	var found []string
	for _, f := range goFiles(t) {
		domain, sub, ok := splitDomain(f.pkg)
		if !ok || sub != "module" || f.test {
			continue
		}
		file := parseGo(t, f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			param, result, ok := identityFunc(n)
			if !ok {
				return true
			}
			from, _, ok := splitDomain(strings.TrimPrefix(byName[typePackage(param)], module+"/"))
			if ok && from != domain {
				found = append(found, fmt.Sprintf("%s: %s → %s", f.path, types.ExprString(param), types.ExprString(result)))
			}
			return true
		})
	}
	closureAllowlist.check(t, found, "a port between domains is implemented by a typed adapter in module/adapters.go that maps errors and types")
}

// identityFunc распознаёт функцию вида func(x T) R { return x } и возвращает типы T и R.
func identityFunc(n ast.Node) (ast.Expr, ast.Expr, bool) {
	var fn *ast.FuncType
	var body *ast.BlockStmt
	switch f := n.(type) {
	case *ast.FuncLit:
		fn, body = f.Type, f.Body
	case *ast.FuncDecl:
		fn, body = f.Type, f.Body
	default:
		return nil, nil, false
	}
	if body == nil || len(body.List) != 1 || fn.Results == nil || len(fn.Results.List) != 1 ||
		len(fn.Params.List) != 1 || len(fn.Params.List[0].Names) != 1 {
		return nil, nil, false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, nil, false
	}
	id, ok := ret.Results[0].(*ast.Ident)
	if !ok || id.Name != fn.Params.List[0].Names[0].Name {
		return nil, nil, false
	}
	return fn.Params.List[0].Type, fn.Results.List[0].Type, true
}

// typePackage — имя пакета в типе вида pkg.T или *pkg.T; "" — тип без пакета.
func typePackage(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

var (
	createTable = regexp.MustCompile(`(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?(?:public\.)?(\w+)`)
	tableRef    = regexp.MustCompile(`(?i)\b(?:from|join|into|update)\s+(?:only\s+)?(?:public\.)?(\w+)`)
	sqlNoise    = regexp.MustCompile(`--[^\n]*|/\*[\s\S]*?\*/|'(?:[^']|'')*'`)
)

// TestQueriesUseOwnTables: таблица приложения (CREATE TABLE в migrations/) принадлежит одному
// домену; запрос соседа к ней — связь в обход порта (ADP-03).
func TestQueriesUseOwnTables(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	tables := sqlNames(t, filepath.Join(root, "migrations", "*.up.sql"), createTable)
	require.NotEmpty(t, tables)
	queriedBy := map[string][]string{} // таблица → домены
	for _, d := range domains(t) {
		used := sqlNames(t, filepath.Join(root, "internal", "domain", d, "infra", "postgres", "queries", "*.sql"), tableRef)
		for _, table := range tables {
			if slices.Contains(used, table) {
				queriedBy[table] = append(queriedBy[table], d)
			}
		}
	}
	for table, ds := range queriedBy {
		if len(ds) > 1 {
			t.Errorf("table %s is queried by domains %s: a table belongs to one domain, the others go through a port", table, strings.Join(ds, ", "))
		}
	}
}

// sqlNames — имена, которые re находит в SQL-файлах glob вне комментариев и строк.
func sqlNames(t *testing.T, glob string, re *regexp.Regexp) []string {
	t.Helper()
	files, err := filepath.Glob(glob)
	require.NoError(t, err)
	var names []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range re.FindAllStringSubmatch(sqlNoise.ReplaceAllString(string(data), " "), -1) {
			names = append(names, strings.ToLower(m[1]))
		}
	}
	return names
}
