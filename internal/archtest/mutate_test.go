package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMutate: раннер make mutate (scripts/tools/mutate.py) на игрушечном модуле — убитая, выжившая, известная
// эквивалентная, несобирающаяся и устаревшая мутации и файл кода без мутаций в отчёте и коде выхода; с --strict отказ и
// за спецификацию, разошедшуюся с кодом; --anchors сверяет только якоря, --build собирает мутации без тестов, --check —
// то и другое; прогон идёт по рабочему дереву — блок спецификации и файл git add -N до коммита гоняются, индекс
// разработчика прогон не трогает; убитая не гоняется заново, пока её блок и файлы те же (ключ результата — не HEAD), а
// с --fresh — гоняется; правка дерева разработчика остаётся, временные файлы go прогона — нет, и прогона, оборванного
// SIGKILL, — тоже; каталог worktree без .git (оборванное создание), копия и перенос репо прогону не мешают.
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
			"## ST p — правка, которой в коде нет\ninternal/p/p.go\n- return 42\n+ return 43\n\n" +
			"## IV p — правка, которая не собирается\ninternal/p/p.go\n- return 0\n+ return \"zero\"\n",
	}))
	git(t, repo, "init", "-q", "-b", "main")
	commitAll(t, repo)
	writeFiles(t, repo, map[string]string{"internal/p/p.go": sign + "\n// Правка разработчика вне коммита.\n"})
	mutate := func(dir string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), python, append([]string{"-I", "scripts/tools/mutate.py"}, args...)...)
		cmd.Dir = dir
		cmd.Env = callerEnv("PATH="+os.Getenv("PATH"), "MUTATE_JOBS=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := mutate(repo, "--check")
	require.Error(t, err, "a stale anchor fails the check:\n%s", out)
	require.Contains(t, out, "ST: «было» в internal/p/p.go: вхождений 0")
	out, err = mutate(repo, "--anchors", "K1", "IV")
	require.NoError(t, err, "the anchors of K1 and IV are in place:\n%s", out)
	require.Contains(t, out, "OK: якоря 2 мутаций")
	require.NotContains(t, out, "сборка", "--anchors builds nothing")
	out, err = mutate(repo, "--check", "K1", "IV")
	require.Error(t, err, "--check builds the mutations:\n%s", out)
	require.Contains(t, out, "не собирается (1)")

	require.NoError(t, os.RemoveAll(filepath.Join(repo, ".cache", "mutate", "wt")))
	writeFiles(t, repo, map[string]string{
		".cache/mutate/tmp/killed/go-build": "a stage of a run killed by SIGKILL\n",
		".cache/mutate/wt/internal/p/p.go":  "the first creation of the worktree broke off: no .git\n",
	})
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

	out, err = mutate(repo, "--build")
	require.Error(t, err, "a mutation that does not build fails --build:\n%s", out)
	for _, want := range []string{"собрались 3", "не собирается (1)", "  IV p", "устаревший якорь (1)", "  ST p"} {
		require.Contains(t, out, want)
	}
	require.NotContains(t, out, "базовый прогон", "--build runs no tests")

	// Прогон идёт по рабочему дереву: блок спецификации, которого нет в HEAD, гоняется до коммита — и по новому файлу,
	// который индекс знает лишь записью git add -N; индекс разработчика прогон не трогает.
	spec := readFiles(t, repo, "scripts/tools/mutants.txt")["scripts/tools/mutants.txt"]
	writeFiles(t, repo, map[string]string{
		"scripts/tools/mutants.txt": spec + "\n## K2 p — Sign: знак положительного\ninternal/p/p.go\n- return 1\n+ return 7\n" +
			"\n## K3 r — Abs: модуль отрицательного\ninternal/r/r.go\n- return -x\n+ return x\n",
		"internal/r/r.go":      "package r\n\n// Abs — модуль числа.\nfunc Abs(x int) int {\n\tif x < 0 {\n\t\treturn -x\n\t}\n\treturn x\n}\n",
		"internal/r/r_test.go": "package r\n\nimport \"testing\"\n\nfunc TestAbs(t *testing.T) {\n\tif Abs(-2) != 2 {\n\t\tt.Fatal(\"abs\")\n\t}\n}\n",
	})
	git(t, repo, "add", "-N", "internal/r")
	status := git(t, repo, "status", "--porcelain")
	out, err = mutate(repo, "K2", "K3")
	require.NoError(t, err, "blocks of the spec and a file of git add -N before their commit run on the tree of the developer:\n%s", out)
	require.Contains(t, out, "мутации идут по рабочему дереву")
	require.Contains(t, out, "убиты 2 — юнит 2")
	require.Equal(t, status, git(t, repo, "status", "--porcelain"), "the run changes the index of the developer")
	git(t, repo, "rm", "-q", "-r", "--cached", "internal/r")
	require.NoError(t, os.RemoveAll(filepath.Join(repo, "internal", "r")))

	// Ключ результата — блок мутации и её файлы, а не HEAD: коммит мимо них убитую K1 заново не гоняет, правка её файла
	// и --fresh — гоняют; выжившая S1 идёт заново всегда.
	const reused = "с тем же ключом (блок и его файлы) и тестом на месте: 1"
	writeFiles(t, repo, map[string]string{"internal/p/p.go": sign, "scripts/tools/mutants.txt": spec})
	require.Empty(t, git(t, repo, "status", "--porcelain"), "the tree of the developer is back to HEAD")
	out, err = mutate(repo)
	require.Error(t, err, "S1 survives:\n%s", out)
	writeFiles(t, repo, map[string]string{
		"internal/q/q.go": "package q\n\n// Q — код без мутаций: правка мимо файлов мутаций.\nfunc Q() int { return 2 }\n",
	})
	commitAll(t, repo)
	out, err = mutate(repo)
	require.Error(t, err, "S1 survives again:\n%s", out)
	require.Contains(t, out, reused)
	require.Contains(t, out, "ВЫЖИЛИ — НОВЫЕ (1)")
	writeFiles(t, repo, map[string]string{"internal/p/p.go": sign + "\n// Правка файла мутаций.\n"})
	commitAll(t, repo)
	out, err = mutate(repo)
	require.Error(t, err, "S1 survives again:\n%s", out)
	require.NotContains(t, out, "с тем же ключом", "a change of its file reruns K1")
	out, err = mutate(repo, "--fresh")
	require.Error(t, err, "S1 survives again:\n%s", out)
	require.NotContains(t, out, "с тем же ключом", "--fresh reruns K1")
	require.Contains(t, out, "убиты 1 — юнит 1")

	// В счёт — последняя строка мутации: вердикт, который дописал после её убийства оборванный прогон, убийство отменяет.
	const results = ".cache/mutate/results.tsv"
	tsv := readFiles(t, repo, results)[results]
	killed := regexp.MustCompile(`(?m)^(\S+)\tK1\tunit\t`).FindStringSubmatch(tsv)
	require.NotNil(t, killed, "K1 is killed in %s:\n%s", results, tsv)
	writeFiles(t, repo, map[string]string{results: tsv + killed[1] + "\tK1\tsurvived\t1s\t\t\n"})
	out, err = mutate(repo)
	require.Error(t, err, "S1 survives again:\n%s", out)
	require.NotContains(t, out, "с тем же ключом", "a later verdict of K1 overrides its kill")
	require.Contains(t, out, "убиты 1 — юнит 1")

	// Тест, ослабленный в другом файле, ключ K1 не видит: обычный прогон берёт её прежнее убийство, а --strict (шаг
	// релиза) — только вердикты того же снимка и находит выжившую; повтор строгого прогона того же снимка их берёт.
	test := readFiles(t, repo, "internal/p/p_test.go")["internal/p/p_test.go"]
	writeFiles(t, repo, map[string]string{"internal/p/p_test.go": strings.Replace(test, " || Sign(0) != 0", "", 1)})
	commitAll(t, repo)
	out, err = mutate(repo)
	require.Error(t, err, "S1 survives again:\n%s", out)
	require.Contains(t, out, reused, "the weakened test is out of the key of K1")
	out, err = mutate(repo, "--strict")
	require.Error(t, err, "K1 and S1 survive the weakened test:\n%s", out)
	require.NotContains(t, out, "с тем же ключом", "--strict reruns the kills of other snapshots")
	require.Contains(t, out, "ВЫЖИЛИ — НОВЫЕ (2)")
	out, err = mutate(repo, "--strict")
	require.Error(t, err, "the same verdicts:\n%s", out)
	require.Contains(t, out, "вердикты прогонов того же снимка: 4", "--strict reruns the mutation that does not build")
	require.Contains(t, out, "ВЫЖИЛИ — НОВЫЕ (2)")
	writeFiles(t, repo, map[string]string{"internal/p/p_test.go": test})
	commitAll(t, repo)

	// Запись worktree, чьё создание оборвалось, остаётся под замком, и go.work разработчика в корне (модуля worktree в нём
	// нет) — прогону не мешают.
	git(t, repo, "worktree", "lock", "--reason", "initializing", filepath.Join(".cache", "mutate", "wt"))
	require.NoError(t, os.RemoveAll(filepath.Join(repo, ".cache", "mutate", "wt")))
	writeFiles(t, repo, map[string]string{"go.work": "go 1.26\n\nuse .\n"})
	out, err = mutate(repo, "K1")
	require.NoError(t, err, "a locked record of a missing worktree and go.work of the developer:\n%s", out)
	require.NoError(t, os.Remove(filepath.Join(repo, "go.work")))

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

// TestMutateStages: стадии мутации на игрушечном модуле с internal/archtest и программой cmd/x. Сначала юнит-тесты
// пакетов, кроме archtest, затем юнит + integration их же, затем archtest: убитая тестом пакета или integration
// archtest не ждёт, а поломку, которую видит один archtest, убивает юнит-стадия; archtest с тегом integration
// гоняется, пока у его теста с тегом другие файлы, а с теми же — один раз, и выжившая проходит всё остальное. Сборка — компиляция без линковки: правка
// пакета программу не линкует, ошибка компиляции программы и пропажа func main (её видит линковка задетой программы)
// — «не собирается». Тесты раннера (TestMutate*) стадии не гоняют: правка кода их вердикт не меняет. go, gofumpt и
// python3 — настоящие, archtest игрушки читает код, как настоящий.
func TestMutateStages(t *testing.T) {
	t.Parallel()
	python, err := exec.LookPath("python3")
	require.NoError(t, err, "make mutate, make new-domain and this test of the gate run on python3: install it")
	repo := t.TempDir()
	const sign = "package p\n\n// Sign — знак числа.\nfunc Sign(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\tif x < 0 {\n\t\treturn -1\n\t}\n\treturn 0\n}\n"
	// archtest игрушки отвергает в коде p слово word — так он видит поломку, которой не видят тесты пакета.
	arch := func(build, name, word string) string {
		return build + "package archtest\n\nimport (\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n)\n\nfunc " + name +
			"(t *testing.T) {\n\tsrc, err := os.ReadFile(\"../p/p.go\")\n\tif err != nil || strings.Contains(string(src), \"" + word +
			"\") {\n\t\tt.Fatal(\"arch\")\n\t}\n}\n"
	}
	writeFiles(t, repo, union(readFiles(t, repoRoot(t), "scripts/tools/mutate.py"), map[string]string{
		".gitignore":                      "/.cache/\n",
		"go.mod":                          "module example.com/stages\n\ngo 1.26\n",
		"internal/p/p.go":                 sign,
		"internal/p/p_test.go":            "package p\n\nimport \"testing\"\n\nfunc TestSign(t *testing.T) {\n\tif Sign(5) != 1 || Sign(0) != 0 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"internal/p/neg_test.go":          "//go:build integration\n\npackage p\n\nimport \"testing\"\n\nfunc TestSignNegative(t *testing.T) {\n\tif Sign(-3) != -1 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"internal/archtest/arch_test.go":  arch("", "TestArch", "TODO"),
		"internal/archtest/zero_test.go":  arch("", "TestArchZero", ">= 0"),
		"internal/archtest/fixme_test.go": arch("//go:build integration\n\n", "TestArchTagged", "FIXME"),
		"internal/archtest/runner_test.go": "package archtest\n\nimport \"testing\"\n\nfunc TestMutateToy(t *testing.T) {\n" +
			"\tt.Fatal(\"the stages of a mutation run the tests of the runner\")\n}\n",
		"cmd/x/main.go": "package main\n\nimport \"example.com/stages/internal/p\"\n\nfunc main() {\n\t_ = p.Sign(1)\n}\n",
		"scripts/tools/mutants.txt": "## U1 p — Sign: ноль положительный\ninternal/p/p.go\n- if x > 0 {\n+ if x >= 0 {\n\n" +
			"## A1 p — Sign: пометка, которую видит только archtest\ninternal/p/p.go\n- // Sign — знак числа.\n+ // Sign — знак числа. TODO\n\n" +
			"## I1 p — Sign: знак отрицательного\ninternal/p/p.go\n- return -1\n+ return -2\n\n" +
			"## T1 p — Sign: пометка, которую видит только archtest с тегом\ninternal/p/p.go\n- // Sign — знак числа.\n+ // Sign — знак числа. FIXME\n\n" +
			"## IV x — программа не компилируется\ncmd/x/main.go\n- _ = p.Sign(1)\n+ _ = p.Sign(\"1\")\n\n" +
			"## NM x — программа без func main\ncmd/x/main.go\n- func main() {\n+ func run() {\n",
	}))
	git(t, repo, "init", "-q", "-b", "main")
	commitAll(t, repo)
	mutate := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), python, append([]string{"-I", "scripts/tools/mutate.py"}, args...)...)
		cmd.Dir = repo
		cmd.Env = callerEnv("PATH="+os.Getenv("PATH"), "MUTATE_JOBS=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	results := func() map[string][]string {
		t.Helper()
		got := map[string][]string{}
		for line := range strings.Lines(readFiles(t, repo, ".cache/mutate/results.tsv")[".cache/mutate/results.tsv"]) {
			cols := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
			got[cols[1]] = []string{cols[2], cols[4]}
		}
		return got
	}
	logOf := func(id string) string {
		t.Helper()
		name := ".cache/mutate/logs/" + id + ".log"
		return readFiles(t, repo, name)[name]
	}

	out, err := mutate()
	require.NoError(t, err, "no new survivors:\n%s", out)
	require.Contains(t, out, "убиты 4 — юнит 2, integration 2")
	require.Contains(t, out, "не собирается (2)")
	got := results()
	require.Equal(t, []string{"unit", "TestSign"}, got["U1"], "a mutation killed by its package does not wait for archtest")
	require.Equal(t, []string{"unit", "TestArch"}, got["A1"], "archtest runs on the unit stage after the other packages")
	require.Equal(t, []string{"integration", "TestSignNegative"}, got["I1"])
	require.Equal(t, []string{"integration", "TestArchTagged"}, got["T1"], "archtest with other files under the tag runs again")
	require.Equal(t, "invalid", got["IV"][0])
	require.Contains(t, got["IV"][1], "cannot use")
	require.Equal(t, "invalid", got["NM"][0])
	require.Contains(t, got["NM"][1], "function main is undeclared")
	require.NotContains(t, logOf("U1"), "$ go build", "a mutation of a package links no program")
	require.NotContains(t, logOf("I1"), "./internal/archtest", "a mutation killed by integration does not wait for archtest")
	archLog := logOf("A1")
	require.Less(t, strings.Index(archLog, "$ go test -tags=integration ./cmd/x ./internal/p "),
		strings.Index(archLog, "$ go test -skip ^TestMutate ./internal/archtest "), "archtest runs after integration")
	require.Contains(t, logOf("NM"), "$ go build -o /dev/null ./cmd/x", "a mutation of a program links it")

	// С теми же файлами archtest с тегом — тот же тест: один раз, на юнит-стадии; выжившая проходит остальное.
	require.NoError(t, os.Remove(filepath.Join(repo, "internal", "archtest", "fixme_test.go")))
	commitAll(t, repo)
	out, err = mutate("T1")
	require.Error(t, err, "T1 survives without the tagged archtest:\n%s", out)
	require.Contains(t, out, "internal/archtest гоняется последним, после integration, и один раз")
	require.Contains(t, out, "ВЫЖИЛИ — НОВЫЕ (1)")
	survivor := logOf("T1")
	for _, want := range []string{
		"$ go test ./cmd/x ./internal/p ", "$ go test -skip ^TestMutate ./internal/archtest ",
		"$ go test -tags=integration ./cmd/x ./internal/p ",
	} {
		require.Contains(t, survivor, want, "the survivor passes all tests of both stages")
	}
	require.NotContains(t, survivor, "-tags=integration -skip ^TestMutate ./internal/archtest", "the same archtest under the tag runs once")
}

// TestMutateShared: серверы integration общего режима testkit — один раз на прогон, а не на мутацию. Модуль с китом
// (заглушка golibinfra: константы общего режима и Start*, который берёт сервер из TESTKIT_SHARED), docker — заглушка:
// раннер снимает контейнеры прошлого прогона его репо, поднимает Postgres один раз на базовый прогон и все мутации,
// отдаёт файл серверов integration-стадии (тест модуля без него падает), убирает контейнер в конце и при SIGTERM посреди
// мутации. Модуль без вызовов кита серверов не поднимает (TestMutate, TestMutateStages).
func TestMutateShared(t *testing.T) {
	t.Parallel()
	python, err := exec.LookPath("python3")
	require.NoError(t, err, "make mutate, make new-domain and this test of the gate run on python3: install it")
	repo, state, bin := t.TempDir(), t.TempDir(), t.TempDir()
	const sign = "package p\n\n// Sign — знак числа.\nfunc Sign(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\tif x < 0 {\n\t\treturn -1\n\t}\n\treturn 0\n}\n"
	marker := filepath.Join(state, "blocked")
	writeFiles(t, repo, union(readFiles(t, repoRoot(t), "scripts/tools/mutate.py"), map[string]string{
		".gitignore": "/.cache/\n",
		"go.mod": "module example.com/shared\n\ngo 1.26\n\nrequire git.stit.tech/stit-core/golibinfra v0.0.0\n\n" +
			"replace git.stit.tech/stit-core/golibinfra => ./kit\n",
		"kit/go.mod": "module git.stit.tech/stit-core/golibinfra\n\ngo 1.26\n",
		"kit/testkit/testkit.go": "// Package testkit — заглушка кита: общий режим и Start*, который берёт сервер из него.\npackage testkit\n\n" +
			"import (\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n)\n\n" +
			"// SharedEnv — переменная файла серверов.\nconst SharedEnv = \"TESTKIT_SHARED\"\n\n" +
			"// PostgresImage — образ Postgres.\nconst PostgresImage = \"postgres:17-alpine\"\n\n" +
			"// StartPostgres — URL общего Postgres; без файла серверов — отказ.\nfunc StartPostgres(t testing.TB) string {\n" +
			"\tdata, err := os.ReadFile(os.Getenv(SharedEnv))\n\tif err != nil || !strings.Contains(string(data), PostgresImage) {\n" +
			"\t\tt.Fatalf(\"no shared postgres: %v %s\", err, data)\n\t}\n\treturn string(data)\n}\n",
		"internal/p/p.go":      sign,
		"internal/p/p_test.go": "package p\n\nimport \"testing\"\n\nfunc TestSign(t *testing.T) {\n\tif Sign(5) != 1 || Sign(0) != 0 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"internal/p/db_test.go": "//go:build integration\n\npackage p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\t\"time\"\n\n" +
			"\t\"git.stit.tech/stit-core/golibinfra/testkit\"\n)\n\n" +
			"func TestDB(t *testing.T) {\n\ttestkit.StartPostgres(t)\n\tif Sign(-5) == -9 {\n" +
			"\t\t_ = os.WriteFile(os.Getenv(\"MUTATE_BLOCKED\"), nil, 0o600)\n\t\ttime.Sleep(time.Minute)\n\t}\n" +
			"\tif Sign(-1) != -1 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"scripts/tools/mutants.txt": "## U1 p — Sign: ноль положительный\ninternal/p/p.go\n- if x > 0 {\n+ if x >= 0 {\n\n" +
			"## I1 p — Sign: знак отрицательного\ninternal/p/p.go\n- return -1\n+ return -2\n\n" +
			"## B1 p — Sign: знак, на котором тест модуля ждёт\ninternal/p/p.go\n- return -1\n+ return -9\n",
	}))
	// docker-заглушка: контейнеры — строки running, снятые — removed, вызовы — calls.
	writeExecutable(t, filepath.Join(bin, "docker"), "#!/bin/bash\ns="+state+"\necho \"$*\" >> \"$s/calls\"\ncase \"$1\" in\n"+
		"run) id=c$RANDOM$RANDOM; echo \"$id\" >> \"$s/running\"; echo \"$id\";;\nport) echo 127.0.0.1:5432;;\n"+
		"logs) printf 'database system is ready to accept connections\\ndatabase system is ready to accept connections\\n';;\n"+
		"ps) cat \"$s/running\" 2>/dev/null;;\n"+
		"rm) shift; while [ \"${1#-}\" != \"$1\" ]; do shift; done\n"+
		"  for id in \"$@\"; do grep -vx -- \"$id\" \"$s/running\" > \"$s/r\"; mv \"$s/r\" \"$s/running\"; echo \"$id\" >> \"$s/removed\"; done;;\n"+
		"esac\n")
	writeFiles(t, state, map[string]string{"running": "leftover\n"})
	git(t, repo, "init", "-q", "-b", "main")
	commitAll(t, repo)
	mutate := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(t.Context(), python, append([]string{"-I", "scripts/tools/mutate.py"}, args...)...)
		cmd.Dir = repo
		cmd.Env = callerEnv("PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MUTATE_BLOCKED="+marker, "MUTATE_JOBS=1")
		return cmd
	}
	lines := func(name string) []string {
		data, err := os.ReadFile(filepath.Join(state, name))
		if os.IsNotExist(err) {
			return nil
		}
		require.NoError(t, err)
		return strings.Fields(string(data))
	}

	out, err := mutate("U1", "I1").CombinedOutput()
	require.NoError(t, err, "a run on the shared servers:\n%s", out)
	require.Contains(t, string(out), "серверы integration — один раз на прогон")
	require.Contains(t, string(out), "убиты 2 — юнит 1, integration 1")
	runs := 0
	for _, call := range strings.Split(strings.TrimSpace(readFiles(t, state, "calls")["calls"]), "\n") {
		if strings.HasPrefix(call, "run ") {
			runs++
			require.Contains(t, call, "postgres:17-alpine", "the image of the kit")
			require.Contains(t, call, "--label mutate.run=", "the label of the run finds its containers")
			require.Contains(t, call, "--name mutate-", "a name of the run")
		}
		if strings.HasPrefix(call, "rm ") {
			require.Contains(t, call, " -v ", "a container of the run leaves its anonymous volumes")
		}
	}
	require.Equal(t, 1, runs, "one Postgres for the base run and all mutations")
	require.Empty(t, lines("running"), "the run leaves its containers")
	require.Contains(t, lines("removed"), "leftover", "a container of the previous run outlives the next run")

	// SIGTERM посреди integration-стадии: контейнер прогона снимается.
	cmd := mutate("B1")
	require.NoError(t, cmd.Start())
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, time.Minute, 50*time.Millisecond,
		"the integration test of B1 blocks")
	require.NotEmpty(t, lines("running"), "the shared server of the run is up")
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	require.Error(t, cmd.Wait(), "SIGTERM ends the run")
	require.Empty(t, lines("running"), "SIGTERM leaves the containers of the run")
}

// TestMutateParallel: несколько исполнителей — тот же итог, те же вердикты и тот же порядок строк хода, что у одного;
// у исполнителя свой worktree (wt, wt-2, …), после прогона — снова снимок; лишний worktree прошлого прогона с большим
// числом исполнителей снимается; SIGTERM посреди мутации снимает её процессы, результат готовой соседней мутации уже
// в results.tsv — следующий прогон её не повторяет.
func TestMutateParallel(t *testing.T) {
	t.Parallel()
	python, err := exec.LookPath("python3")
	require.NoError(t, err, "make mutate, make new-domain and this test of the gate run on python3: install it")
	repo := t.TempDir()
	marker := filepath.Join(t.TempDir(), "blocked")
	const sign = "package p\n\n// Sign — знак числа.\nfunc Sign(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\tif x < 0 {\n\t\treturn -1\n\t}\n\treturn 0\n}\n"
	writeFiles(t, repo, union(readFiles(t, repoRoot(t), "scripts/tools/mutate.py"), map[string]string{
		".gitignore":           "/.cache/\n",
		"go.mod":               "module example.com/par\n\ngo 1.26\n",
		"internal/p/p.go":      sign,
		"internal/p/p_test.go": "package p\n\nimport \"testing\"\n\nfunc TestSign(t *testing.T) {\n\tif Sign(5) != 1 || Sign(0) != 0 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"internal/p/neg_test.go": "//go:build integration\n\npackage p\n\nimport (\n\t\"os\"\n\t\"strconv\"\n\t\"testing\"\n\t\"time\"\n)\n\n" +
			"func TestSignNegative(t *testing.T) {\n\tif Sign(-5) == -9 && os.Getenv(\"MUTATE_BLOCK\") != \"\" {\n" +
			"\t\t_ = os.WriteFile(os.Getenv(\"MUTATE_BLOCK\"), []byte(strconv.Itoa(os.Getpid())), 0o600)\n\t\ttime.Sleep(time.Minute)\n\t}\n" +
			"\tif Sign(-3) != -1 {\n\t\tt.Fatal(\"sign\")\n\t}\n}\n",
		"scripts/tools/mutants.txt": "## B1 p — Sign: знак, на котором тест модуля ждёт\ninternal/p/p.go\n- return -1\n+ return -9\n\n" +
			"## K1 p — Sign: ноль положительный\ninternal/p/p.go\n- if x > 0 {\n+ if x >= 0 {\n\n" +
			"## K2 p — Sign: знак положительного\ninternal/p/p.go\n- return 1\n+ return 7\n\n" +
			"## I1 p — Sign: знак отрицательного\ninternal/p/p.go\n- return -1\n+ return -2\n\n" +
			"## E1 p — Sign: отрицательные иначе\n= для целых x < 0 и x <= -1 — одно и то же\ninternal/p/p.go\n- if x < 0 {\n+ if x <= -1 {\n\n" +
			"## IV p — правка, которая не собирается\ninternal/p/p.go\n- return 0\n+ return \"zero\"\n",
	}))
	git(t, repo, "init", "-q", "-b", "main")
	commitAll(t, repo)
	mutate := func(jobs string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(t.Context(), python, append([]string{"-I", "scripts/tools/mutate.py"}, args...)...)
		cmd.Dir = repo
		cmd.Env = callerEnv("PATH="+os.Getenv("PATH"), "MUTATE_JOBS="+jobs)
		return cmd
	}
	// Строки хода без времени и итог — у одного и у трёх исполнителей одни и те же.
	stable := func(out string) string {
		_, lines, ok := strings.Cut(out, "[1/")
		require.True(t, ok, "no lines of the run:\n%s", out)
		return regexp.MustCompile(`(?m)^(\[\d+/\d+\] \S+ \S+ — \S+) \d+s`).ReplaceAllString("[1/"+lines, "$1")
	}
	ids := []string{"K1", "K2", "I1", "E1", "IV"}
	one, err := mutate("1", ids...).CombinedOutput()
	require.NoError(t, err, "one executor:\n%s", one)
	three, err := mutate("3", ids...).CombinedOutput()
	require.NoError(t, err, "three executors:\n%s", three)
	require.Contains(t, string(three), "→ исполнителей: 3 (MUTATE_JOBS)")
	require.Equal(t, stable(string(one)), stable(string(three)), "the lines of the run and the summary do not depend on the executors")
	require.Contains(t, string(three), "убиты 3 — юнит 2, integration 1")
	for _, wt := range []string{"wt", "wt-2", "wt-3"} {
		require.Empty(t, git(t, filepath.Join(repo, ".cache", "mutate", wt), "status", "--porcelain"), "the worktree %s is the snapshot again", wt)
	}
	out, err := mutate("2", "K1", "K2").CombinedOutput()
	require.NoError(t, err, "two executors:\n%s", out)
	require.NoDirExists(t, filepath.Join(repo, ".cache", "mutate", "wt-3"), "a worktree of more executors outlives the run")
	require.NotContains(t, git(t, repo, "worktree", "list"), "wt-3")

	// SIGTERM посреди integration-стадии B1: её процессы сняты, K1 соседнего исполнителя уже в results.tsv.
	require.NoError(t, os.Remove(filepath.Join(repo, ".cache", "mutate", "results.tsv")))
	cmd := exec.CommandContext(t.Context(), python, "-I", "scripts/tools/mutate.py", "B1", "K1")
	cmd.Dir = repo
	cmd.Env = callerEnv("PATH="+os.Getenv("PATH"), "MUTATE_JOBS=2", "MUTATE_BLOCK="+marker)
	require.NoError(t, cmd.Start())
	results := filepath.Join(repo, ".cache", "mutate", "results.tsv")
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(results)
		_, blocked := os.Stat(marker)
		return err == nil && strings.Contains(string(data), "\tK1\t") && blocked == nil
	}, time.Minute, 50*time.Millisecond, "K1 is done while B1 waits")
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	require.Error(t, cmd.Wait(), "SIGTERM ends the run")
	data, err := os.ReadFile(marker)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	require.Eventually(t, func() bool { return syscall.Kill(pid, 0) != nil }, 10*time.Second, 50*time.Millisecond,
		"the test of B1 outlives SIGTERM")
	for _, wt := range []string{"wt", "wt-2"} {
		require.Empty(t, git(t, filepath.Join(repo, ".cache", "mutate", wt), "status", "--porcelain"), "the worktree %s keeps the mutation", wt)
	}
	out, err = mutate("2").CombinedOutput()
	require.NoError(t, err, "the run after SIGTERM:\n%s", out)
	require.Contains(t, string(out), "с тем же ключом (блок и его файлы) и тестом на месте: 1", "the interrupted run goes on from K1")
	require.Contains(t, string(out), "B1 p — integration", "B1 does not wait without MUTATE_BLOCK")
}
