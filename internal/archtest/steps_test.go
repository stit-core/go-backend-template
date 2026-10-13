package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// step запускает скрипт шага гейта в dir без окружения make и git вызывающего и отдаёт вывод.
func step(t *testing.T, dir, script string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Env = callerEnv("PATH=" + os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestCoverCheck: порог покрытия — факт (100), блок, который тесты не исполнят по построению, идёт
// исполненным только по записи списка непокрываемого cover.sh, а запись без такого блока — отказ. Модуль
// игрушечный: ветка G с паникой тестами не исполняется, профиль — настоящий go test.
func TestCoverCheck(t *testing.T) {
	t.Parallel()
	script := readFiles(t, repoRoot(t), "scripts/tools/cover.sh")["scripts/tools/cover.sh"]
	const list = "    cat <<'LIST'\nLIST\n"
	require.Equal(t, 1, strings.Count(script, list), "anchor of the uncoverable list in cover.sh")
	for _, tc := range []struct {
		name  string
		entry string // запись списка непокрываемого
		want  string
		ok    bool
	}{
		{name: "an uncovered block", want: "  internal/p/p.go:13."},
		{name: "the block is listed", entry: "internal/p/p.go:panic(\"unreachable", want: "непокрываемое списком — 1", ok: true},
		{name: "an entry without an uncovered block", entry: "internal/p/p.go:return 1", want: "запись непокрываемого без неисполненного блока"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, map[string]string{
				"go.mod":                 "module example.com/cov\n\ngo 1.26\n",
				"pkg/doc.txt":            "\n",
				"internal/p/p.go":        "package p\n\n// F — покрытая.\nfunc F(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n\n// G — ошибки тесты не дают.\nfunc G(err error) string {\n\tif err != nil {\n\t\tpanic(\"unreachable: G never gets an error\")\n\t}\n\treturn \"ok\"\n}\n",
				"internal/p/p_test.go":   "package p\n\nimport \"testing\"\n\nfunc TestP(t *testing.T) {\n\tif F(1) != 1 || F(0) != 0 || G(nil) != \"ok\" {\n\t\tt.Fatal(\"p\")\n\t}\n}\n",
				"scripts/tools/cover.sh": strings.Replace(script, list, "    cat <<'LIST'\n"+tc.entry+"\nLIST\n", 1),
			})
			git(t, repo, "init", "-q", "-b", "main")
			out, err := runIn(t, repo, "go", "test", "-coverprofile=coverage.out", "./...")
			require.NoError(t, err, out)

			out, err = step(t, repo, "scripts/tools/cover.sh", "coverage.out", "100")
			require.Contains(t, out, tc.want)
			if tc.ok {
				require.NoError(t, err, out)
				return
			}
			require.Error(t, err, out)
		})
	}
}

// TestMigrationsCheck: migrations-check пропускает только каталог по конвенции, а up с эффектом для выкатки
// — разрушительный, правку данных, долгий lock, расширение, код DO — только с маркером
// `-- infra-handoff:`. Индекс по таблице этого же файла и CONCURRENTLY, действия FK и триггер на UPDATE —
// не эффект; выпущенные миграции репо проходят как есть.
func TestMigrationsCheck(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "tools", "migrations-check.sh")
	require.FileExists(t, script) // скрипт читает bash: без этого кеш go test не видит его правок
	users := map[string]string{
		"0001_users.up.sql":   "CREATE TABLE users (id UUID PRIMARY KEY, name TEXT, role TEXT);\n",
		"0001_users.down.sql": "DROP TABLE users;\n",
	}
	second := func(up string) map[string]string {
		return union(users, map[string]string{"0002_change.up.sql": up, "0002_change.down.sql": "-- noop: данные не возвращаются\n"})
	}
	for _, tc := range []struct {
		name  string
		files map[string]string // nil — каталога нет
		dir   string            // каталог вместо files
		want  string
		ok    bool
	}{
		{name: "migrations of the repo", dir: filepath.Join(root, "migrations"), want: "OK: migrations (", ok: true},
		{name: "empty directory", files: map[string]string{}, want: "OK: migrations (пусто)", ok: true},
		{name: "no directory", want: "FAIL: migrations: нет каталога"},
		{name: "three-digit names", files: map[string]string{"001_init.up.sql": "DROP TABLE users;\n", "001_init.down.sql": ""}, want: "вне конвенции"},
		{name: "timestamp names", files: map[string]string{"20261010120000_init.up.sql": "DROP TABLE users;\n", "20261010120000_init.down.sql": ""}, want: "вне конвенции"},
		{name: "a subdirectory only", files: map[string]string{"v2/0001_init.up.sql": "DROP TABLE users;\n", "v2/0001_init.down.sql": ""}, want: "  v2"},
		{name: "a new table with an index", files: map[string]string{
			"0001_orders.up.sql":   "CREATE TABLE orders (\n    id UUID PRIMARY KEY,\n    user_id UUID NOT NULL\n);\nCREATE INDEX orders_user_id_idx ON orders(user_id);\nCREATE UNIQUE INDEX IF NOT EXISTS orders_id_uq ON ONLY orders (id);\n",
			"0001_orders.down.sql": "DROP TABLE orders;\n",
		}, want: "OK: migrations (1 пар", ok: true},
		{name: "an index concurrently", files: second("CREATE INDEX CONCURRENTLY users_name_idx ON users (name);\n"), want: "OK: migrations (2 пар", ok: true},
		{name: "FK actions and an update trigger", files: second("CREATE TABLE notes (\n    id UUID PRIMARY KEY,\n    user_id UUID REFERENCES users(id) ON DELETE CASCADE ON UPDATE SET NULL\n);\nCREATE TRIGGER notes_upd BEFORE UPDATE ON notes FOR EACH ROW EXECUTE FUNCTION set_updated_at();\n"), want: "OK: migrations (2 пар", ok: true},
		{name: "DROP COLUMN", files: second("ALTER TABLE users DROP COLUMN role;\n"), want: "alter table users drop column role"},
		{name: "DELETE", files: second("DELETE FROM users WHERE role = 'x';\n"), want: "delete from users"},
		{name: "UPDATE … SET", files: second("UPDATE ONLY users u SET role = 'admin';\n"), want: "update only users u set role ="},
		{name: "SET NOT NULL", files: second("ALTER TABLE users ALTER COLUMN name SET NOT NULL;\n"), want: "set not null"},
		{name: "an index on an existing table", files: second("CREATE INDEX users_name_idx ON users (name);\n"), want: "create index users_name_idx on users"},
		{name: "CREATE EXTENSION", files: second("CREATE EXTENSION IF NOT EXISTS citext;\n"), want: "create extension if not exists citext"},
		{name: "DO", files: second("DO $$ BEGIN EXECUTE 'DROP TABLE users'; END $$;\n"), want: "do $$ begin execute"},
		{name: "an effect with the marker", files: second("-- infra-handoff: роли пересчитываются — окно выкатки\nUPDATE users SET role = 'client';\n"), want: "OK: migrations (2 пар", ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.dir
			if dir == "" {
				dir = filepath.Join(t.TempDir(), "migrations")
				if tc.files != nil {
					require.NoError(t, os.MkdirAll(dir, 0o755))
					writeFiles(t, dir, tc.files)
				}
			}
			out, err := step(t, root, script, dir)
			require.Contains(t, out, tc.want)
			if tc.ok {
				require.NoError(t, err, out)
				return
			}
			require.Error(t, err, out)
			require.Contains(t, out, "FAIL: migrations")
		})
	}
}
