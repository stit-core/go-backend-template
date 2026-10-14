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
// исполненным только по записи списка непокрываемого cover.sh — ровно одной своей: запись без текста, без
// такого блока или подходящая к двум блокам — отказ. Модуль игрушечный: ветка G с паникой тестами не
// исполняется (с h.go — и такая же ветка H), профиль — настоящий go test.
func TestCoverCheck(t *testing.T) {
	t.Parallel()
	script := readFiles(t, repoRoot(t), "scripts/tools/cover.sh")["scripts/tools/cover.sh"]
	const list = "    cat <<'LIST'\nLIST\n"
	require.Equal(t, 1, strings.Count(script, list), "anchor of the uncoverable list in cover.sh")
	// h.go — H и H2 с одинаковой веткой паники: запись по её тексту подошла бы к обеим.
	h := map[string]string{
		"internal/p/h.go": "package p\n\n// H — как G.\nfunc H(err error) string {\n\tif err != nil {\n\t\tpanic(\"unreachable: H never gets an error\")\n\t}\n\treturn \"ok\"\n}\n\n" +
			"// H2 — ещё одна такая ветка.\nfunc H2(err error) {\n\tif err != nil {\n\t\tpanic(\"unreachable: H2 never gets an error\")\n\t}\n}\n",
		"internal/p/h_test.go": "package p\n\nimport \"testing\"\n\nfunc TestH(t *testing.T) {\n\tH2(nil)\n\tif H(nil) != \"ok\" {\n\t\tt.Fatal(\"h\")\n\t}\n}\n",
	}
	for _, tc := range []struct {
		name  string
		entry string            // запись списка непокрываемого
		files map[string]string // файлы модуля поверх p.go
		want  string
		ok    bool
	}{
		{name: "an uncovered block", want: "  internal/p/p.go:13."},
		{name: "the block is listed", entry: "internal/p/p.go:panic(\"unreachable", want: "непокрываемое списком — 1", ok: true},
		{name: "an entry without an uncovered block", entry: "internal/p/p.go:return 1", want: "запись непокрываемого без неисполненного блока"},
		{name: "an entry without a text", entry: "internal/p/p.go:", want: "запись непокрываемого без текста блока"},
		{name: "an entry over two blocks", entry: "internal/p/h.go:panic(\"unreachable", files: h, want: "подходит к нескольким неисполненным блокам"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, union(map[string]string{
				"go.mod":                 "module example.com/cov\n\ngo 1.26\n",
				"pkg/doc.txt":            "\n",
				"internal/p/p.go":        "package p\n\n// F — покрытая.\nfunc F(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n\n// G — ошибки тесты не дают.\nfunc G(err error) string {\n\tif err != nil {\n\t\tpanic(\"unreachable: G never gets an error\")\n\t}\n\treturn \"ok\"\n}\n",
				"internal/p/p_test.go":   "package p\n\nimport \"testing\"\n\nfunc TestP(t *testing.T) {\n\tif F(1) != 1 || F(0) != 0 || G(nil) != \"ok\" {\n\t\tt.Fatal(\"p\")\n\t}\n}\n",
				"scripts/tools/cover.sh": strings.Replace(script, list, "    cat <<'LIST'\n"+tc.entry+"\nLIST\n", 1),
			}, tc.files))
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

// TestLintUsesItsConfig: lint гейта читает только .golangci.yml — слабый .golangci.toml рядом его не
// заслоняет, — а своя директива gosec ничего не глушит. Модуль игрушечный, конфиг и скрипт — репо.
func TestLintUsesItsConfig(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	repo := t.TempDir()
	writeFiles(t, repo, union(readFiles(t, root, ".golangci.yml", "scripts/tools/lint.sh"), map[string]string{
		"go.mod":         "module example.com/lint\n\ngo 1.26\n",
		".golangci.toml": "version = \"2\"\n\n[linters]\ndefault = \"none\"\nenable = [\"govet\"]\n",
		"p/p.go": "package p\n\nimport (\n\t\"crypto/md5\" // #nosec G501\n\t\"os\"\n)\n\n// Close drops an error.\n" +
			"func Close(f *os.File) {\n\tf.Close()\n}\n\n// Sum hashes weakly.\nfunc Sum(s string) [16]byte {\n\treturn md5.Sum([]byte(s)) // #nosec G401\n}\n",
	}))
	git(t, repo, "init", "-q", "-b", "main")
	out, err := step(t, repo, "scripts/tools/lint.sh")
	require.Error(t, err, out)
	require.Contains(t, out, "(errcheck)", ".golangci.toml shadows .golangci.yml")
	require.Contains(t, out, "(gosec)", "the gosec directive silences gosec")
}

// TestGenCheckComparesMocksWithTree: gen-check сверяет моки с деревом до прогона mockery, а не с индексом:
// свежие моки после make gen — не дрейф, ручная правка мока в дереве — дрейф, новый мок вне индекса —
// отказ, а MOCKERY_* окружения до mockery не доходят. Инструменты — заглушки: mockery пишет моки v2 (с
// MOCKERY_CONFIG в окружении — никуда, как вывод мимо mocks/) и падает без --config .mockery.yml — свой
// конфиг mockery ищет сам, и .mockery.yaml рядом заслонил бы .mockery.yml; sqlc и go отвечают успехом.
func TestGenCheckComparesMocksWithTree(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	bin := t.TempDir()
	writeFiles(t, bin, map[string]string{
		"mockery": "#!/bin/sh\n[ \"$*\" = \"--config .mockery.yml\" ] || { echo \"mockery $*: not the config of the repo\" >&2; exit 3; }\n" +
			"[ -z \"$MOCKERY_CONFIG\" ] || exit 0\nprintf 'package mocks // v2\\n' >p/mocks/m.go\nprintf 'package mocks // n\\n' >p/mocks/n.go\n",
		"sqlc": "#!/bin/sh\nexit 0\n",
		"go":   "#!/bin/sh\nexit 0\n",
	})
	for _, tool := range []string{"mockery", "sqlc", "go"} {
		require.NoError(t, os.Chmod(filepath.Join(bin, tool), 0o700))
	}
	for _, tc := range []struct {
		name    string
		tree    string // p/mocks/m.go в дереве; в индексе — v1
		newMock bool   // n.go — вне индекса
		env     []string
		want    string
		ok      bool
	}{
		{name: "fresh mocks out of the index", tree: "package mocks // v2\n", want: "OK: sqlc, моки", ok: true},
		{name: "a hand-edited mock in the tree", tree: "package mocks // v2 hand-edited\n", want: "  p/mocks/m.go"},
		{name: "MOCKERY_CONFIG in env", tree: "package mocks // v1\n", env: []string{"MOCKERY_CONFIG=/elsewhere.yml"}, want: "FAIL: моки разошлись"},
		{name: "a new mock out of the index", tree: "package mocks // v2\n", newMock: true, want: "FAIL: новые моки не в индексе"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, union(readFiles(t, root, "Makefile"), map[string]string{
				"p/mocks/m.go": "package mocks // v1\n",
				"p/mocks/n.go": "package mocks // n\n",
			}))
			git(t, repo, "init", "-q", "-b", "main")
			commitAll(t, repo)
			if tc.newMock {
				git(t, repo, "rm", "-q", "--cached", "p/mocks/n.go")
			}
			writeFiles(t, repo, map[string]string{"p/mocks/m.go": tc.tree})
			cmd := exec.CommandContext(t.Context(), "make", "--no-print-directory", "gen-check")
			cmd.Dir = repo
			cmd.Env = callerEnv(append([]string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, tc.env...)...)
			data, err := cmd.CombinedOutput()
			out := string(data)
			require.Contains(t, out, tc.want)
			if tc.ok {
				require.NoError(t, err, out)
				return
			}
			require.Error(t, err, out)
		})
	}
}

// TestFmtAppliesLintRules: lint требует правила gofumpt с extra-rules (.golangci.yml), поэтому fmt-check
// ловит их нарушение, а make fmt его чинит — иначе отказ lint не снять ни make fmt, ни ничем, кроме //nolint.
func TestFmtAppliesLintRules(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	require.Contains(t, readFiles(t, root, ".golangci.yml")[".golangci.yml"], "extra-rules: true")
	repo := t.TempDir()
	writeFiles(t, repo, union(readFiles(t, root, "Makefile"), map[string]string{
		"p/p.go": "package p\n\n// Join joins.\nfunc Join(a string, b string) string { return a + b }\n",
	}))
	runMake := func(target string) (string, error) {
		cmd := exec.CommandContext(t.Context(), "make", "--no-print-directory", target)
		cmd.Dir = repo
		cmd.Env = callerEnv("PATH=" + os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := runMake("fmt-check")
	require.Error(t, err, "fmt-check must refuse what lint refuses:\n%s", out)
	require.Contains(t, out, "FAIL: fmt")
	out, err = runMake("fmt")
	require.NoError(t, err, out)
	require.Contains(t, readFiles(t, repo, "p/p.go")["p/p.go"], "func Join(a, b string)", "make fmt must apply the rules lint requires")
	out, err = runMake("fmt-check")
	require.NoError(t, err, out)
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
