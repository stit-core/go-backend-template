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
	return stepEnv(t, dir, nil, script, args...)
}

// stepEnv — step с env поверх окружения.
func stepEnv(t *testing.T, dir string, env []string, script string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Env = callerEnv(append([]string{"PATH=" + os.Getenv("PATH")}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// makeIn запускает цель Makefile в dir без окружения make и git вызывающего; env — поверх, в том числе PATH.
func makeIn(t *testing.T, dir string, env []string, target string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "make", "--no-print-directory", target)
	cmd.Dir = dir
	cmd.Env = callerEnv(append([]string{"PATH=" + os.Getenv("PATH")}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pin — закреплённая версия инструмента (scripts/tools/versions.sh): её печатают заглушки инструментов.
func pin(t *testing.T, tool string) string {
	t.Helper()
	out, err := step(t, repoRoot(t), "scripts/tools/versions.sh", "--pin", tool)
	require.NoError(t, err, out)
	return strings.TrimSpace(out)
}

// TestCoverCheck: порог покрытия — факт (100), блок, который тесты не исполнят по построению, идёт
// исполненным только по записи списка непокрываемого cover.sh — ровно одной своей: запись без текста, без
// такого блока или подходящая к двум блокам — отказ, а функцию без тестов запись не прячет. Функция без
// тестов — отказ и в cmd/, кроме main: доля cmd/ в порог не идёт. Модуль игрушечный: ветка G с паникой тестами
// не исполняется (с h.go — и такая же ветка H), профиль — настоящий go test с -coverpkg, как у test-int.
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
		{name: "an entry over a whole function", entry: "internal/p/p.go:panic(\"unreachable\ninternal/p/k.go:unreachable: K", files: map[string]string{
			"internal/p/k.go": "package p\n\n// K — её тесты не зовут.\nfunc K() string { return \"unreachable: K\" }\n",
		}, want: "  example.com/cov/internal/p/k.go:4:"},
		{name: "a function of cmd/ without tests", entry: "internal/p/p.go:panic(\"unreachable", files: map[string]string{
			"cmd/app/main.go": "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(code()) }\n\n// code — код выхода.\nfunc code() int { return 0 }\n",
		}, want: "  example.com/cov/cmd/app/main.go:8:"},
		{name: "main of cmd/ and its tested logic", entry: "internal/p/p.go:panic(\"unreachable", files: map[string]string{
			"cmd/app/main.go":      "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(code(os.Args[1:])) }\n\n// code — код выхода; ветку ошибки тест не берёт: доля cmd/ в порог не идёт.\nfunc code(args []string) int {\n\tif len(args) > 0 {\n\t\treturn 2\n\t}\n\treturn 0\n}\n",
			"cmd/app/main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCode(t *testing.T) {\n\tif code(nil) != 0 {\n\t\tt.Fatal(\"code\")\n\t}\n}\n",
		}, want: "непокрываемое списком — 1", ok: true},
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
			out, err := runIn(t, repo, "go", "test", "-coverpkg=./internal/...,./pkg/...,./cmd/...", "-coverprofile=coverage.out", "./...")
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
// заслоняет, — своя директива gosec ничего не глушит, а пакеты грузит go list: драйвер go/packages из окружения
// (GOPACKAGESDRIVER) до линтера не доходит. Модуль игрушечный, Makefile, конфиг и скрипты — репо.
func TestLintUsesItsConfig(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	repo := t.TempDir()
	writeFiles(t, repo, union(readFiles(t, root, "Makefile", ".golangci.yml", "scripts/tools/lint.sh", "scripts/tools/versions.sh"), map[string]string{
		"go.mod":         "module example.com/lint\n\ngo 1.26\n",
		".golangci.toml": "version = \"2\"\n\n[linters]\ndefault = \"none\"\nenable = [\"govet\"]\n",
		"p/p.go": "package p\n\nimport (\n\t\"crypto/md5\" // #nosec G501\n\t\"os\"\n)\n\n// Close drops an error.\n" +
			"func Close(f *os.File) {\n\tf.Close()\n}\n\n// Sum hashes weakly.\nfunc Sum(s string) [16]byte {\n\treturn md5.Sum([]byte(s)) // #nosec G401\n}\n",
	}))
	git(t, repo, "init", "-q", "-b", "main")
	driver := filepath.Join(t.TempDir(), "driver")
	writeExecutable(t, driver, "#!/bin/sh\ntouch \"$0.called\"\necho '{\"Roots\":[],\"Packages\":[]}'\n")
	out, err := makeIn(t, repo, []string{"GOPACKAGESDRIVER=" + driver}, "lint")
	require.Error(t, err, out)
	require.Contains(t, out, "(errcheck)", ".golangci.toml shadows .golangci.yml")
	require.Contains(t, out, "(gosec)", "the gosec directive silences gosec")
	require.NoFileExists(t, driver+".called", "GOPACKAGESDRIVER of the caller loads the packages of the lint step")
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
	for tool, program := range map[string]string{
		"mockery": "#!/bin/sh\n[ \"$*\" = version ] && { echo " + pin(t, "mockery") + "; exit 0; }\n" +
			"[ \"$*\" = \"--config .mockery.yml\" ] || { echo \"mockery $*: not the config of the repo\" >&2; exit 3; }\n" +
			"[ -z \"$MOCKERY_CONFIG\" ] || exit 0\nprintf 'package mocks // v2\\n' >p/mocks/m.go\nprintf 'package mocks // n\\n' >p/mocks/n.go\n",
		"sqlc": "#!/bin/sh\n[ \"$*\" = version ] && echo " + pin(t, "sqlc") + "\nexit 0\n",
		"go":   "#!/bin/sh\nexit 0\n",
	} {
		writeExecutable(t, filepath.Join(bin, tool), program)
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
			writeFiles(t, repo, union(readFiles(t, root, "Makefile", "scripts/tools/versions.sh"), map[string]string{
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
// Код репо — его файлы, отслеживаемые и новые: worktree раннера в игнорируемом .cache/ (после обрыва — с
// правкой мутации) fmt-check не судит, а make fmt не трогает.
func TestFmtAppliesLintRules(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	require.Contains(t, readFiles(t, root, ".golangci.yml")[".golangci.yml"], "extra-rules: true")
	repo := t.TempDir()
	const unformatted = "package p\n\n// Join joins.\nfunc Join(a string, b string) string { return a + b }\n"
	writeFiles(t, repo, union(readFiles(t, root, "Makefile", "scripts/tools/fmt.sh", "scripts/tools/versions.sh"), map[string]string{
		".gitignore":              "/.cache/\n",
		"p/p.go":                  unformatted,
		".cache/mutate/wt/p/p.go": unformatted,
	}))
	git(t, repo, "init", "-q", "-b", "main")
	out, err := makeIn(t, repo, nil, "fmt-check")
	require.Error(t, err, "fmt-check must refuse what lint refuses:\n%s", out)
	require.Contains(t, out, "FAIL: fmt")
	require.NotContains(t, out, ".cache", "fmt-check judges the worktree of the mutation runner")
	out, err = makeIn(t, repo, nil, "fmt")
	require.NoError(t, err, out)
	require.Contains(t, readFiles(t, repo, "p/p.go")["p/p.go"], "func Join(a, b string)", "make fmt must apply the rules lint requires")
	require.Equal(t, unformatted, readFiles(t, repo, ".cache/mutate/wt/p/p.go")[".cache/mutate/wt/p/p.go"], "make fmt rewrites the worktree of the mutation runner")
	out, err = makeIn(t, repo, nil, "fmt-check")
	require.NoError(t, err, out)
}

// TestMigrationsCheck: migrations-check пропускает только каталог по конвенции (и .gitkeep каталога без
// миграций), а up с эффектом для выкатки — разрушительный, правку данных (и INSERT в существующую таблицу),
// долгий lock, расширение, код (DO, CALL, SELECT верхнего уровня) — только с маркером `-- infra-handoff:`, при
// любой локали вызывающего. Индекс по таблице этого же файла и CONCURRENTLY, INSERT в неё, действия FK, триггер на
// UPDATE и UPDATE в теле объявленной функции — не эффект; выпущенные миграции шаблона проходят как есть — по
// содержимому, и под другим номером, а своя миграция форка с их именем проверяется целиком.
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
	turkish := []string{"LC_ALL=tr_TR.UTF-8"} // [a-z] без «i», а tr оставляет «I»
	for _, tc := range []struct {
		name  string
		files map[string]string // nil — каталога нет
		dir   string            // каталог вместо files
		env   []string
		want  string
		ok    bool
	}{
		{name: "migrations of the repo", dir: filepath.Join(root, "migrations"), want: "OK: migrations (", ok: true},
		{name: "migrations of the repo in the Turkish locale", dir: filepath.Join(root, "migrations"), env: turkish, want: "OK: migrations (", ok: true},
		{name: "an index on an existing table in the Turkish locale", files: second("CREATE INDEX users_name_idx ON users (name);\n"), env: turkish, want: "create index users_name_idx on users"},
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
		{name: "DO", files: second("DO $$ BEGIN EXECUTE 'DROP TABLE users'; END $$;\n"), want: "do $$…$$"},
		{name: "CALL", files: second("CALL refresh_users();\n"), want: "call refresh_users()"},
		{name: "SELECT of a function declared in the file", files: second("CREATE FUNCTION fill() RETURNS void AS $body$\nBEGIN\n    UPDATE users SET role = 'client';\nEND\n$body$ LANGUAGE plpgsql;\nSELECT fill();\n"), want: "select fill()"},
		{name: "UPDATE in the body of a declared function", files: second("CREATE FUNCTION touch() RETURNS trigger AS $$\nBEGIN\n    UPDATE users SET name = NEW.name;\n    RETURN NEW;\nEND;\n$$ LANGUAGE plpgsql;\n"), want: "OK: migrations (2 пар", ok: true},
		{name: "an effect with the marker", files: second("-- infra-handoff: роли пересчитываются — окно выкатки\nUPDATE users SET role = 'client';\n"), want: "OK: migrations (2 пар", ok: true},
		{name: ".gitkeep of a directory without migrations", files: map[string]string{".gitkeep": ""}, want: "OK: migrations (пусто)", ok: true},
		{name: ".gitkeep beside migrations", files: union(users, map[string]string{".gitkeep": ""}), want: "OK: migrations (1 пар", ok: true},
		{name: "INSERT into an existing table", files: second("INSERT INTO users (id, name, role) VALUES (gen_random_uuid(), 'root', 'admin');\n"), want: "insert into users (id, name, role) values"},
		{name: "INSERT into a table of the file", files: second("CREATE TABLE roles (name TEXT PRIMARY KEY);\nINSERT INTO roles (name) VALUES ('admin'), ('client');\n"), want: "OK: migrations (2 пар", ok: true},
		{name: "a released migration under another number", files: union(users, map[string]string{
			"0002_pending_idx.up.sql":   readFiles(t, root, "migrations/0003_email_verifications_idempotency.up.sql")["migrations/0003_email_verifications_idempotency.up.sql"],
			"0002_pending_idx.down.sql": "DROP INDEX email_verifications_pending_user_idx;\n",
		}), want: "OK: migrations (2 пар", ok: true},
		{name: "a migration of the fork named as a released one", files: map[string]string{
			"0001_users.up.sql":   "CREATE EXTENSION IF NOT EXISTS citext;\nCREATE TABLE users (id UUID PRIMARY KEY, email CITEXT);\n",
			"0001_users.down.sql": "DROP TABLE users;\n",
		}, want: "create extension if not exists citext"},
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
			out, err := stepEnv(t, root, tc.env, script, dir)
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

// TestToolVersions: инструмент гейта — закреплённой версии (scripts/tools/versions.sh): шаг с инструментом другой
// версии отказывает — у каждой машины был бы свой гейт, — а make setup ставит только инструмент не той версии,
// и с закреплёнными сеть ему не нужна. Инструменты — заглушки, печатающие версию; go и curl записывают вызов.
func TestToolVersions(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	versionOf := map[string]func(v string) string{ // программа заглушки, которая печатает версию v
		"gofumpt": func(v string) string { return "echo '" + v + " (go1.26.0)'" },
		"golangci-lint": func(v string) string {
			return "echo 'golangci-lint has version " + strings.TrimPrefix(v, "v") + " built with go1.26'"
		},
		"sqlc":        func(v string) string { return "echo " + v },
		"mockery":     func(v string) string { return "echo " + v },
		"govulncheck": func(v string) string { return "echo 'Scanner: govulncheck@" + v + "'" },
	}
	const stale = "v0.0.1"
	tools := func(t *testing.T, other string) string { // каталог для PATH: закреплённые версии, у other — stale
		t.Helper()
		bin := t.TempDir()
		for tool, program := range versionOf {
			v := pin(t, tool)
			if tool == other {
				v = stale
			}
			writeExecutable(t, filepath.Join(bin, tool), "#!/bin/sh\n"+program(v)+"\n")
		}
		return bin
	}
	for target, tool := range map[string]string{"fmt-check": "gofumpt", "lint": "golangci-lint", "gen-check": "mockery"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, readFiles(t, root, "Makefile", "scripts/tools/fmt.sh", "scripts/tools/lint.sh", "scripts/tools/versions.sh"))
			git(t, repo, "init", "-q", "-b", "main")
			out, err := makeIn(t, repo, []string{"PATH=" + tools(t, tool) + string(os.PathListSeparator) + os.Getenv("PATH")}, target)
			require.Error(t, err, "make %s with %s %s must refuse:\n%s", target, tool, stale, out)
			require.Contains(t, out, "FAIL: "+tool+" "+stale+", закреплена "+pin(t, tool))
		})
	}
	t.Run("setup", func(t *testing.T) {
		t.Parallel()
		repo, gopath := t.TempDir(), t.TempDir()
		writeFiles(t, repo, union(readFiles(t, root, "scripts/install-tools.sh", "scripts/tools/versions.sh"), map[string]string{
			"go.mod": "module example.org/acme/backend\n",
		}))
		bin := tools(t, "sqlc")
		writeExecutable(t, filepath.Join(bin, "go"), "#!/bin/sh\ncase \"$1 $2\" in\n\"env GOPATH\") echo "+gopath+" ;;\n"+
			"\"env GOPRIVATE\") echo 'example.org/*' ;;\n*) echo \"go $*\" >>\"$0.calls\" ;;\nesac\n")
		writeExecutable(t, filepath.Join(bin, "curl"), "#!/bin/sh\necho \"curl $*\" >>\"$0.calls\"\nexit 1\n")
		out, err := stepEnv(t, repo, []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, "scripts/install-tools.sh")
		require.NoError(t, err, out)
		require.Equal(t, "go install github.com/sqlc-dev/sqlc/cmd/sqlc@"+pin(t, "sqlc")+"\n", readFiles(t, bin, "go.calls")["go.calls"],
			"make setup installs the tool of another version and only it:\n%s", out)
		require.NoFileExists(t, filepath.Join(bin, "curl.calls"), "golangci-lint of the pinned version is installed again")
	})
}

// TestGenBuildsFirst: mockery грузит пакеты интерфейсов и над кодом, который не собирается, падает шумом загрузки
// пакетов — make gen собирает код до него и называет причину. Мок, устаревший после переименования типа из
// сигнатуры интерфейса, — не причина: его mockery и перепишет. sqlc и mockery — заглушки, go — настоящий.
func TestGenBuildsFirst(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	const port = "package p\n\n// Port — интерфейс из .mockery.yml.\ntype Port interface{ Get() Item }\n\n// Item — тип из сигнатуры Port.\ntype Item struct{}\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		fail  string // ошибка сборки в отказе; пусто — make gen проходит
	}{
		{name: "code that does not build", files: map[string]string{"p/p.go": port + "\nvar _ = renamedByTheEdit\n"}, fail: "undefined: renamedByTheEdit"},
		{name: "a stale mock", files: map[string]string{"p/p.go": port, "p/mocks/port.go": "package mocks\n\nimport \"example.com/gen/p\"\n\nvar _ p.Element\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bin, repo := t.TempDir(), t.TempDir()
			writeExecutable(t, filepath.Join(bin, "sqlc"), "#!/bin/sh\n[ \"$*\" = version ] && echo "+pin(t, "sqlc")+"\nexit 0\n")
			writeExecutable(t, filepath.Join(bin, "mockery"), "#!/bin/sh\n[ \"$*\" = version ] && { echo "+pin(t, "mockery")+"; exit 0; }\ntouch \"$0.called\"\n")
			writeFiles(t, repo, union(readFiles(t, root, "Makefile", "scripts/tools/versions.sh"), tc.files, map[string]string{
				"go.mod":          "module example.com/gen\n\ngo 1.26\n",
				"cmd/api/main.go": "package main\n\nfunc main() {}\n",
				"cmd/api/api_test.go": "package main\n\nimport (\n\t\"flag\"\n\t\"testing\"\n)\n\nvar _ = flag.Bool(\"update\", false, \"\")\n\n" +
					"func TestOpenAPISpec(*testing.T) {}\n",
			}))
			out, err := makeIn(t, repo, []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, "gen")
			if tc.fail == "" {
				require.NoError(t, err, "make gen refuses over a mock that mockery regenerates:\n%s", out)
				require.FileExists(t, filepath.Join(bin, "mockery.called"))
				return
			}
			require.Error(t, err, out)
			require.Contains(t, out, tc.fail, "make gen must name the build error")
			require.Contains(t, out, "FAIL: код не собирается")
			require.NoFileExists(t, filepath.Join(bin, "mockery.called"), "make gen runs mockery over code that does not build")
		})
	}
}
