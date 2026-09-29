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

	"git.stit.tech/stit-core/go-backend-template/internal/testkit/allowlist"
)

// closureAllowlist — identity-closure между доменами в текущем коде.
var closureAllowlist = allowlist.List{
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
	closureAllowlist.Check(t, found, "a port between domains is implemented by a typed adapter in module/adapters.go that maps errors and types")
}

// identityFunc распознаёт функцию вида func(x T) R { return x } или func(x T) (R, error)
// { return x, nil } (конструктор fx с ошибкой) и возвращает типы T и R.
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
	if body == nil || len(body.List) != 1 || fn.Results == nil || len(fn.Params.List) != 1 || len(fn.Params.List[0].Names) != 1 {
		return nil, nil, false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	results := fn.Results.List
	switch {
	case !ok || len(ret.Results) != len(results) || len(results) > 2:
		return nil, nil, false
	case len(results) == 2 && (!isIdent(results[1].Type, "error") || !isIdent(ret.Results[1], "nil")):
		return nil, nil, false
	case !isIdent(ret.Results[0], fn.Params.List[0].Names[0].Name):
		return nil, nil, false
	}
	return fn.Params.List[0].Type, results[0].Type, true
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
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
	createTable = regexp.MustCompile(`(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?(?:"?public"?\.)?"?(\w+)`)
	// tableKeyword — за ним идёт таблица; tableItem — таблица (схема, кавычки, алиас) и запятая,
	// если список FROM продолжается.
	tableKeyword = regexp.MustCompile(`(?i)\b(?:from|join|into|update|using)\s+(?:only\s+)?`)
	tableItem    = regexp.MustCompile(`^(?:"?\w+"?\.)?"?(\w+)"?(?:\s+(?:as\s+)?\w+)?(\s*,\s*)?`)
	sqlNoise     = regexp.MustCompile(`--[^\n]*|/\*[\s\S]*?\*/|'(?:[^']|'')*'`)
)

// TestQueriesUseOwnTables: таблица приложения (CREATE TABLE в migrations/) принадлежит одному
// домену; запрос соседа к ней — связь в обход порта.
func TestQueriesUseOwnTables(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	var tables []string
	for _, m := range createTable.FindAllStringSubmatch(sqlText(t, filepath.Join(root, "migrations", "*.up.sql")), -1) {
		tables = append(tables, strings.ToLower(m[1]))
	}
	require.NotEmpty(t, tables)
	queriedBy := map[string][]string{} // таблица → домены
	for _, d := range domains(t) {
		used := queriedTables(sqlText(t, filepath.Join(root, "internal", "domain", d, "infra", "postgres", "queries", "*.sql")))
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

// queriedTables — имена в позиции таблицы: после FROM, JOIN, INTO, UPDATE, USING и через запятую
// в списке FROM. Колонка с именем чужой таблицы (orders.notes) — не обращение к ней.
func queriedTables(sql string) []string {
	var names []string
	for _, kw := range tableKeyword.FindAllStringIndex(sql, -1) {
		rest := sql[kw[1]:]
		for m := tableItem.FindStringSubmatch(rest); m != nil; m = tableItem.FindStringSubmatch(rest) {
			names = append(names, strings.ToLower(m[1]))
			if m[2] == "" {
				break
			}
			rest = rest[len(m[0]):]
		}
	}
	return names
}

// sqlText — SQL файлов glob без комментариев и строк.
func sqlText(t *testing.T, glob string) string {
	t.Helper()
	files, err := filepath.Glob(glob)
	require.NoError(t, err)
	var sql strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		sql.WriteString(sqlNoise.ReplaceAllString(string(data), " ") + "\n")
	}
	return sql.String()
}
