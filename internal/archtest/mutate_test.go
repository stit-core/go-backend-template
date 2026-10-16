package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMutate: раннер make mutate (scripts/tools/mutate.py) на игрушечном модуле — убитая, выжившая, известная
// эквивалентная и устаревшая мутации и файл кода без мутаций в отчёте и коде выхода; с --strict отказ и за
// спецификацию, разошедшуюся с кодом; правка дерева разработчика остаётся, временные файлы go прогона — нет, и
// прогона, оборванного SIGKILL, — тоже; копия и перенос репо прогону не мешают.
// go, gofumpt и python3 — настоящие, integration-тестов в модуле нет.
func TestMutate(t *testing.T) {
	t.Parallel()
	python, err := exec.LookPath("python3")
	require.NoError(t, err, "make mutate, make new-domain and this test of the gate run on python3: install it")
	root := repoRoot(t)
	repo := t.TempDir()
	const sign = "package p\n\n// Sign — знак числа.\nfunc Sign(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\tif x < 0 {\n\t\treturn -1\n\t}\n\treturn 0\n}\n"
	writeFiles(t, repo, union(readFiles(t, root, "scripts/tools/mutate.py"), map[string]string{
		".gitignore":           "/.cache/\n",
		"go.mod":               "module example.com/mut\n\ngo 1.26\n",
		"internal/p/p.go":      sign,
		"internal/p/p_test.go": "package p\n\nimport \"testing\"\n\nfunc TestSign(t *testing.T) {\n\tif Sign(5) != 1 || Sign(0) != 0 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"internal/q/q.go":      "package q\n\n// Q — код без мутаций.\nfunc Q() int { return 1 }\n",
		"scripts/tools/mutants.txt": "## K1 p — Sign: ноль положительный\ninternal/p/p.go\n- if x > 0 {\n+ if x >= 0 {\n\n" +
			"## S1 p — Sign: знак отрицательного\ninternal/p/p.go\n- return -1\n+ return -2\n\n" +
			"## E1 p — Sign: знак отрицательного иначе\n= тесты не видят: отрицательных в тестах нет\ninternal/p/p.go\n- return -1\n+ return -3\n\n" +
			"## ST p — правка, которой в коде нет\ninternal/p/p.go\n- return 42\n+ return 43\n",
	}))
	git(t, repo, "init", "-q", "-b", "main")
	commitAll(t, repo)
	writeFiles(t, repo, map[string]string{"internal/p/p.go": sign + "\n// Правка разработчика вне коммита.\n"})
	mutate := func(dir string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), python, append([]string{"-I", "scripts/tools/mutate.py"}, args...)...)
		cmd.Dir = dir
		cmd.Env = callerEnv("PATH=" + os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := mutate(repo, "--check")
	require.Error(t, err, "a stale anchor fails the check:\n%s", out)
	require.Contains(t, out, "ST: «было» в internal/p/p.go: вхождений 0")

	writeFiles(t, repo, map[string]string{".cache/mutate/tmp/killed/go-build": "a stage of a run killed by SIGKILL\n"})
	out, err = mutate(repo)
	require.Error(t, err, "a new survivor fails the run:\n%s", out)
	for _, want := range []string{
		"убиты 1 — юнит 1, integration 0", "выжили известные эквивалентные (1): E1", "ВЫЖИЛИ — НОВЫЕ (1)",
		"  S1 p — Sign: знак отрицательного", "устаревший якорь (1)", "файлы кода без мутаций (1)", "  internal/q/q.go",
	} {
		require.Contains(t, out, want)
	}
	require.Contains(t, readFiles(t, repo, "internal/p/p.go")["internal/p/p.go"], "Правка разработчика вне коммита",
		"the run touches the tree of the developer")
	tmp, err := os.ReadDir(filepath.Join(repo, ".cache", "mutate", "tmp"))
	require.NoError(t, err)
	require.Empty(t, tmp, "temporary files of go outlive their stage")

	out, err = mutate(repo, "--strict", "K1", "E1")
	require.NoError(t, err, "a killed and a known equivalent mutation pass --strict:\n%s", out)
	require.Contains(t, out, "OK: новых выживших нет, спецификация совпадает с кодом")

	out, err = mutate(repo, "--strict", "K1", "ST")
	require.Error(t, err, "a stale anchor fails --strict:\n%s", out)
	require.Contains(t, out, "устаревших якорей — 1")

	// Worktree раннера у копии репо ведёт в прежний репо, у перенесённого — в никуда: раннер создаёт его заново, а
	// worktree прежнего репо не трогает.
	wt := filepath.Join(".cache", "mutate", "wt")
	head := git(t, filepath.Join(repo, wt), "rev-parse", "HEAD")
	copied := filepath.Join(t.TempDir(), "copy")
	_, err = runIn(t, repo, "cp", "-a", repo, copied)
	require.NoError(t, err)
	writeFiles(t, copied, map[string]string{"internal/p/p.go": sign + "\n// Коммит копии.\n"})
	commitAll(t, copied)
	out, err = mutate(copied, "K1")
	require.NoError(t, err, "a copy of the repo:\n%s", out)
	require.Equal(t, head, git(t, filepath.Join(repo, wt), "rev-parse", "HEAD"), "the run of a copy moves the worktree of the original")
	moved := filepath.Join(t.TempDir(), "moved")
	require.NoError(t, os.Rename(repo, moved))
	out, err = mutate(moved, "K1")
	require.NoError(t, err, "the moved repo:\n%s", out)
}
