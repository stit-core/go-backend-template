package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewDomainRefusesOrRollsBack: make new-domain с ключевым словом Go, объявлением пакета домена и таблицей,
// которую уже создаёт миграция, отказывает до первой правки, а слово кода среза, на котором срез собирается
// (status, code…), — нет; сбой после первой правки (здесь — make gen без Makefile; у имени, которое код среза уже
// объявляет, — сборка) откатывает все правки скрипта, и блоки мутаций среза тоже: дерево — как до запуска. До make gen
// срез сверяет якоря своих блоков мутаций (их пишет сам скрипт): блок, разошедшийся со скелетом, — отказ с правкой
// скрипта. Песочница — git-репо со скриптом, раннером мутаций, go.mod, cmd/api, конфигами генераторов, спецификацией
// мутаций, миграциями репо и своей миграцией с занятой таблицей (у форка после prune таблиц доменов идентичности нет);
// инструменты — заглушки, python3 — настоящий. Имена, которые форк вправе взять своему домену, тест берёт свободными
// у сервиса (freeDomain).
func TestNewDomainRefusesOrRollsBack(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := readFiles(t, root, "scripts/new-domain.sh", "go.mod", "cmd/api/main.go", "cmd/api/http.go", "sqlc.yaml", ".mockery.yml", "openapi.json",
		"scripts/tools/mutants.txt", "scripts/tools/mutate.py")
	migrations, err := os.ReadDir(filepath.Join(root, "migrations"))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	for _, m := range migrations {
		files = union(files, readFiles(t, root, "migrations/"+m.Name()))
	}
	// Имена — до своей миграции: её таблица занимает имя occupied.
	word, occupied, free := freeDomain(files, "status", "code", "field", "api", "caller"), freeDomain(files, "probe_item"), freeDomain(files, "probe")
	files["migrations/9000_occupied.up.sql"] = "CREATE TABLE " + occupied + "s (id UUID PRIMARY KEY);\n"

	bin := stubTools(t, "sqlc", "mockery")
	gofumpt, err := exec.LookPath("true")
	require.NoError(t, err)
	require.NoError(t, os.Symlink(gofumpt, filepath.Join(bin, "gofumpt")))
	// Блок мутаций скелета, чей «было» скелет не пишет: так блоки устаревают, когда меняется код среза.
	stale := strings.Replace(files["scripts/new-domain.sh"], "- if d := uc.d.Policy.CanCreate${ATITLE}(ctx, caller); !d.IsAllowed {",
		"- if d := uc.d.Policy.MayCreate(ctx, caller); !d.IsAllowed {", 1)
	require.NotEqual(t, files["scripts/new-domain.sh"], stale, "the block ${NAME}-01 of new-domain.sh")
	const anchored = "OK: якоря" // сверка якорей блоков среза прошла
	for _, tc := range []struct {
		name   string
		domain string
		agg    string // AGG=; пусто — агрегат по умолчанию
		script string // new-domain.sh песочницы; пусто — репо
		want   []string
	}{
		{name: "a keyword", domain: "func", want: []string{"домен 'func' — ключевое слово"}},
		{name: "a declaration of the domain package", domain: "usecase", want: []string{"объявление пакета домена"}},
		{name: "an aggregate declaration", domain: free, agg: "usecase", want: []string{"агрегат 'usecase' — объявление пакета домена", "AGG="}},
		{name: "an aggregate not in snake_case", domain: free, agg: "Order", want: []string{"имя агрегата — snake_case"}},
		{name: "an aggregate table taken", domain: free, agg: occupied, want: []string{"таблицу " + occupied + "s уже создаёт миграция — возьми другой агрегат"}},
		{name: "an aggregate the name does not give", domain: "matches", want: []string{"агрегат контекста matches по имени не выводится", "AGG="}},
		{name: "a word of the slice that builds", domain: word, want: []string{anchored, "правки откатаны"}},
		{name: "an occupied table", domain: occupied, want: []string{"таблицу " + occupied + "s уже создаёт миграция"}},
		{name: "a failure after the first edit", domain: free, want: []string{anchored, "правки откатаны"}},
		{name: "a stale block of the skeleton", domain: free, script: stale, want: []string{"блоки мутаций среза", "scripts/new-domain.sh", "правки откатаны"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, files)
			if tc.script != "" {
				writeFiles(t, repo, map[string]string{"scripts/new-domain.sh": tc.script})
			}
			git(t, repo, "init", "-q", "-b", "main")
			commitAll(t, repo)

			args := []string{"scripts/new-domain.sh", tc.domain}
			if tc.agg != "" {
				args = append(args, tc.agg)
			}
			cmd := exec.CommandContext(t.Context(), "bash", args...)
			cmd.Dir = repo
			cmd.Env = callerEnv("PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "new-domain %s must fail here:\n%s", tc.domain, out)
			for _, want := range tc.want {
				require.Contains(t, string(out), want)
			}
			require.Empty(t, git(t, repo, "status", "--porcelain", "--untracked-files=all"), "new-domain %s leaves the tree changed:\n%s", tc.domain, out)
		})
	}
}

// freeDomain — первое из имён, которого у сервиса нет (ни алиаса в cmd/api, ни таблицы в миграциях), а заняты все —
// первое с номером: срез с ним доходит до таблицы и первой правки, какие бы домены ни завёл форк. Таблица — с
// любым окончанием (status → statuses), номер окончания не меняет (probe_item2 → probe_item2s).
func freeDomain(files map[string]string, names ...string) string {
	var all strings.Builder
	for _, text := range files {
		all.WriteString(strings.ToLower(text) + "\n")
	}
	taken := func(name string) bool {
		flat := strings.ReplaceAll(name, "_", "")
		for _, use := range []string{flat + "mod", flat + "http", "table " + name, "exists " + name} {
			if strings.Contains(all.String(), use) {
				return true
			}
		}
		return false
	}
	for _, name := range names {
		if !taken(name) {
			return name
		}
	}
	for n := 2; ; n++ {
		if name := names[0] + strconv.Itoa(n); !taken(name) {
			return name
		}
	}
}

// TestNewDomainNames: имена среза по контексту и агрегату (--names — без правок): агрегат по умолчанию — контекст в
// единственном числе по однозначному окончанию, таблица и путь — множественное агрегата без второго -s, HTTP-типы — с
// префиксом домена, инициализм во множественном — с «s» строчной.
func TestNewDomainNames(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, tc := range []struct {
		args                    []string
		typ, table, http, field string
	}{
		{args: []string{"order"}, typ: "Order", table: "orders", http: "Order", field: "Orders"},
		{args: []string{"objects"}, typ: "Object", table: "objects", http: "ObjectsObject", field: "Objects"},
		{args: []string{"categories"}, typ: "Category", table: "categories", http: "CategoriesCategory", field: "Categories"},
		{args: []string{"addresses"}, typ: "Address", table: "addresses", http: "AddressesAddress", field: "Addresses"},
		{args: []string{"boxes"}, typ: "Box", table: "boxes", http: "BoxesBox", field: "Boxes"},
		{args: []string{"analyses"}, typ: "Analysis", table: "analyses", http: "AnalysesAnalysis", field: "Analyses"},
		{args: []string{"analysis"}, typ: "Analysis", table: "analyses", http: "Analysis", field: "Analyses"},
		{args: []string{"status"}, typ: "Status", table: "statuses", http: "Status", field: "Statuses"},
		{args: []string{"quiz"}, typ: "Quiz", table: "quizzes", http: "Quiz", field: "Quizzes"},
		{args: []string{"cases"}, typ: "Case", table: "cases", http: "CasesCase", field: "Cases"},
		{args: []string{"urls"}, typ: "URL", table: "urls", http: "URLsURL", field: "URLs"},
		{args: []string{"order_items"}, typ: "OrderItem", table: "order_items", http: "OrderItemsOrderItem", field: "OrderItems"},
		{args: []string{"people", "employee"}, typ: "Employee", table: "employees", http: "PeopleEmployee", field: "Employees"},
		{args: []string{"ordering", "order"}, typ: "Order", table: "orders", http: "OrderingOrder", field: "Orders"},
		{args: []string{"media", "series"}, typ: "Series", table: "series", http: "MediaSeries", field: "Series"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			out, err := runIn(t, root, "bash", append([]string{"scripts/new-domain.sh", "--names"}, tc.args...)...)
			require.NoError(t, err, out)
			names := map[string]string{}
			for line := range strings.Lines(out) {
				if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
					names[key] = value
				}
			}
			table := strings.ReplaceAll(tc.table, "_", "-")
			require.Equal(t, map[string]string{
				"domain": tc.args[0], "agg": names["agg"], "type": tc.typ, "table": tc.table, "route": "/api/" + table,
				"http": tc.http, "port": tc.typ + "Repository", "field": tc.field,
			}, names)
		})
	}
}
