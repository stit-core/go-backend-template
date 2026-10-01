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

// TestNoCrossDomainIdentityClosure: `func(uc *user.Usecase) auth.Port { return uc }` отдаёт
// потребителю чужие ошибки и типы мимо маппинга — порт между доменами реализует
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
	require.Empty(t, found, "a port between domains is implemented by a typed adapter in module/adapters.go that maps errors and types")
}

// TestTrustedOnlyInModules: Trusted домена — операции без policy для других доменов; их берут только
// адаптеры module/ потребителей, иначе вход (HTTP, NATS) обошёл бы авторизацию. Свой module/
// поставщика только провайдит NewTrusted: интерфейс над *Trusted в нём открыл бы доверенные методы
// своему же входу.
func TestTrustedOnlyInModules(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if f.test {
			continue
		}
		domain, sub, inDomain := splitDomain(f.pkg)
		file := parseGo(t, f.path)
		byName := importsByName(file)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Trusted" {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			provider, provSub, ok := splitDomain(strings.TrimPrefix(byName[id.Name], module+"/"))
			if ok && provSub == "" && (!inDomain || sub != "module" || provider == domain) {
				t.Errorf("%s uses %s.Trusted: operations without policy are for adapters in module/ of consumer domains", f.path, id.Name)
			}
			return true
		})
	}
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
	// sqlToken — слово (с кавычками и схемой) или скобка, запятая, точка с запятой: остальное для
	// поиска таблиц не нужно.
	sqlToken = regexp.MustCompile(`"?\w+"?(?:\."?\w+"?)?|[(),;]`)
	// fromEnds — предложения, которыми кончается список FROM.
	fromEnds = []string{"where", "group", "having", "order", "limit", "offset", "returning", "union", "intersect", "except", "window", "for"}
	sqlNoise = regexp.MustCompile(`--[^\n]*|/\*[\s\S]*?\*/|'(?:[^']|'')*'`)
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

// queriedTables — имена в позиции таблицы: после FROM запроса, JOIN, INTO, UPDATE, USING и через
// запятую в списке FROM, в том числе после JOIN … ON …. Колонка с именем таблицы (orders.notes)
// и FROM внутри выражения (TRIM(… FROM notes), IS DISTINCT FROM notes) — не обращение к таблице.
func queriedTables(sql string) []string {
	type scope struct{ query, from bool } // уровень скобок: идёт запрос, открыт список FROM
	scopes := []scope{{}}
	var names []string
	table, prev := false, ""
	for _, tok := range sqlToken.FindAllString(sql, -1) {
		cur := &scopes[len(scopes)-1]
		word := strings.ToLower(tok)
		switch {
		case tok == "(":
			scopes, table = append(scopes, scope{}), false
		case tok == ")":
			scopes, table = scopes[:max(len(scopes)-1, 1)], false
		case tok == ";":
			scopes, table = []scope{{}}, false
		case tok == ",":
			table = cur.from
		case table && word != "only" && word != "lateral":
			names, table = append(names, strings.Trim(word[strings.LastIndex(word, ".")+1:], `"`)), false
		case word == "select" || word == "delete":
			cur.query = true
		case word == "from" && cur.query && prev != "distinct":
			cur.from, table = true, true
		case word == "join" || word == "into" || word == "update" || word == "using":
			cur.query, table = true, true
		case slices.Contains(fromEnds, word):
			cur.from = false
		}
		prev = word
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
