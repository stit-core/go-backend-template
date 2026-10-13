package archtest

import (
	"os"
	"os/exec"
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
