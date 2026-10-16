package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewDomainRefusesOrRollsBack: make new-domain с ключевым словом Go, объявлением пакета домена и таблицей,
// которую уже создаёт миграция, отказывает до первой правки, а слово кода среза, на котором срез собирается
// (status), — нет; сбой после первой правки (здесь — make gen без Makefile; у имени, которое код среза уже
// объявляет, — сборка) откатывает все правки скрипта, и блоки мутаций среза тоже: дерево — как до запуска. Песочница —
// git-репо со скриптом, go.mod, cmd/api, конфигами генераторов, спецификацией мутаций, миграциями репо и своей
// миграцией с занятой таблицей (у форка после prune таблиц доменов идентичности нет); инструменты — заглушки.
func TestNewDomainRefusesOrRollsBack(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := readFiles(t, root, "scripts/new-domain.sh", "go.mod", "cmd/api/main.go", "cmd/api/http.go", "sqlc.yaml", ".mockery.yml", "openapi.json",
		"scripts/tools/mutants.txt")
	migrations, err := os.ReadDir(filepath.Join(root, "migrations"))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	for _, m := range migrations {
		files = union(files, readFiles(t, root, "migrations/"+m.Name()))
	}
	files["migrations/9000_occupied.up.sql"] = "CREATE TABLE probe_items (id UUID PRIMARY KEY);\n"

	bin := stubTools(t, "sqlc", "mockery")
	gofumpt, err := exec.LookPath("true")
	require.NoError(t, err)
	require.NoError(t, os.Symlink(gofumpt, filepath.Join(bin, "gofumpt")))
	for _, tc := range []struct {
		name   string
		domain string
		want   string
	}{
		{name: "a keyword", domain: "func", want: "ключевое слово"},
		{name: "a declaration of the domain package", domain: "repository", want: "объявление пакета домена"},
		{name: "a word of the slice that builds", domain: "status", want: "правки откатаны"},
		{name: "an occupied table", domain: "probe_item", want: "таблицу probe_items уже создаёт миграция"},
		{name: "a failure after the first edit", domain: freeDomain(files), want: "правки откатаны"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, files)
			git(t, repo, "init", "-q", "-b", "main")
			commitAll(t, repo)

			cmd := exec.CommandContext(t.Context(), "bash", "scripts/new-domain.sh", tc.domain)
			cmd.Dir = repo
			cmd.Env = callerEnv("PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "new-domain %s must fail here:\n%s", tc.domain, out)
			require.Contains(t, string(out), tc.want)
			require.Empty(t, git(t, repo, "status", "--porcelain", "--untracked-files=all"), "new-domain %s leaves the tree changed:\n%s", tc.domain, out)
		})
	}
}

// freeDomain — имя домена, которого у сервиса нет (ни алиаса в cmd/api, ни таблицы в миграциях): срез с ним
// доходит до первой правки, какие бы домены ни завёл форк.
func freeDomain(files map[string]string) string {
	var all strings.Builder
	for _, text := range files {
		all.WriteString(strings.ToLower(text))
	}
	name := "probe"
	for strings.Contains(all.String(), name+"mod") || strings.Contains(all.String(), name+"http") ||
		strings.Contains(all.String(), "table "+name+"s") {
		name += "x"
	}
	return name
}
