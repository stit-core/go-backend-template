package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// callerEnv — окружение теста без переменных make и git вызывающего (тест идёт и внутри make check, а тот —
// в хуке git) и без PATH, с конфигом git без глобального и системного; extra — поверх.
func callerEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, "MAKE") && !strings.HasPrefix(key, "GIT_") && key != "MFLAGS" && key != "GNUMAKEFLAGS" && key != "PATH" {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	return append(env, extra...)
}

// stubTools — каталог для PATH: названные инструменты — false, а go отвечает только на go env (по нему
// Makefile узнаёт GOFLAGS, test.sh — cgo): шаг гейта, который дошёл бы до go test, падает сразу.
func stubTools(t *testing.T, tools ...string) string {
	t.Helper()
	bin := t.TempDir()
	fail, err := exec.LookPath("false")
	require.NoError(t, err)
	for _, tool := range tools {
		require.NoError(t, os.Symlink(fail, filepath.Join(bin, tool)))
	}
	realGo, err := exec.LookPath("go")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\n[ \"$1\" = env ] && exec \""+realGo+
		"\" \"$@\"\necho \"go $*: the probe has no toolchain\" >&2\nexit 1\n"), 0o700))
	return bin
}

// shellForgeries — окружение оболочки, которым вызывающий подменил бы bash-скрипт: файл автозапуска
// (BASH_ENV), функции с именами его команд (export -f) и опция «читать, но не исполнять» (SHELLOPTS).
func shellForgeries(t *testing.T) map[string][]string {
	t.Helper()
	exit0 := filepath.Join(t.TempDir(), "exit0.sh")
	require.NoError(t, os.WriteFile(exit0, []byte("exit 0\n"), 0o600))
	return map[string][]string{
		"BASH_ENV":         {"BASH_ENV=" + exit0},
		"function set":     {"BASH_FUNC_set%%=() { exit 0; }"},
		"function env":     {"BASH_FUNC_env%%=() { :; }"},
		"function read":    {"BASH_FUNC_read%%=() { return 1; }"},
		"SHELLOPTS=noexec": {"SHELLOPTS=noexec"},
	}
}

// TestMakeCheckCannotBeForged: флаги make, переменные вызывающего (командная строка и окружение) и лишний
// makefile (check: или check::, до или после Makefile, MAKEFILES) не подделывают «✓ check clean». Makefile
// под подделку (переприсваивает MAKEFILE_LIST) обходит любую сверку внутри make — это правка гейта, вердикт
// для remote — pre-push. Makefile — копия репо в песочнице: гейт снимает coverage.out, а test-int вокруг
// этого теста пишет его в дерево репо. Инструменты гейта здесь — заглушки: gofumpt роняет первый шаг, а go
// отвечает только на go env. Форма, прошедшая самозащиту, с -i дошла бы до test-int, а он снова запустил бы
// этот тест; без go шаги падают сразу, и подделка видна за секунды.
func TestMakeCheckCannotBeForged(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeFiles(t, repo, readFiles(t, repoRoot(t), "Makefile"))
	bin, fakes := stubTools(t, "gofumpt", "golangci-lint", "sqlc", "mockery"), t.TempDir()
	shell := shellForgeries(t)
	writeFiles(t, fakes, map[string]string{
		"colon.mk":  "check:\n\t@echo \"✓ check clean\"\n",
		"double.mk": "check::\n\t@echo \"✓ check clean\"\n",
		"shell.mk":  "override SHELL := /bin/true\n",
		"goals.mk":  "MAKECMDGOALS := x\n",
	})
	colon, double, shellMk := filepath.Join(fakes, "colon.mk"), filepath.Join(fakes, "double.mk"), filepath.Join(fakes, "shell.mk")
	goals := filepath.Join(fakes, "goals.mk")
	for _, tc := range []struct {
		name string
		args []string
		env  []string
		gate bool // настоящий гейт запускается (и падает на заглушке первого шага)
	}{
		{name: "make check", args: []string{"check"}, gate: true},
		{name: "-f ./Makefile", args: []string{"-f", "./Makefile", "check"}, gate: true},
		{name: "second makefile with check:", args: []string{"-f", "Makefile", "-f", colon, "check"}},
		{name: "-o and second makefile", args: []string{"-o", "gate-makefiles", "-f", "Makefile", "-f", colon, "check"}},
		{name: "second makefile with check:: after", args: []string{"-f", "Makefile", "-f", double, "check"}},
		{name: "second makefile with check:: before", args: []string{"-f", double, "-f", "Makefile", "check"}},
		{name: "second makefile with SHELL after", args: []string{"-f", "Makefile", "-f", shellMk, "check"}},
		{name: "MAKEFILES", args: []string{"check"}, env: []string{"MAKEFILES=" + double}},
		{name: "--eval", args: []string{"--eval", "SHELL:=/bin/true", "check"}},
		{name: "-E rule", args: []string{"-E", "check:: ; @echo \"✓ check clean\"", "check"}},
		{name: "-k and second makefile", args: []string{"-k", "-f", "Makefile", "-f", double, "check"}},
		{name: "-j and second makefile", args: []string{"-j2", "-f", "Makefile", "-f", double, "check"}},
		{name: "-p", args: []string{"-p", "check"}},
		{name: "-i", args: []string{"-i", "check"}},
		{name: "-k", args: []string{"-k", "check"}},
		{name: "-j8", args: []string{"-j8", "check"}},
		{name: "--ignore-errors", args: []string{"--ignore-errors", "check"}},
		{name: "--keep-going", args: []string{"--keep-going", "check"}},
		{name: "--dry-run", args: []string{"--dry-run", "check"}},
		{name: "--touch", args: []string{"--touch", "check"}},
		{name: "--question", args: []string{"--question", "check"}},
		{name: "-e with GATE", args: []string{"-e", "check"}, env: []string{"GATE=true"}},
		{name: "MAKEFLAGS=i on the command line", args: []string{"check", "MAKEFLAGS=i"}},
		{name: "MAKEFLAGS=i in env", args: []string{"check"}, env: []string{"MAKEFLAGS=i"}},
		{name: "GOFLAGS in env", args: []string{"check"}, env: []string{"GOFLAGS=-run=^$"}},
		{name: "GOFLAGS on the command line", args: []string{"check", "GOFLAGS=-run=^$"}},
		{name: "API_TEST_MAIN_ARGS in env", args: []string{"check"}, env: []string{"API_TEST_MAIN_ARGS=healthcheck"}},

		// Формы RENF-01 повторного аудита: отказы самозащиты включались по MAKECMDGOALS, а её задаёт вызывающий.
		{name: "-i with MAKECMDGOALS on the command line", args: []string{"-i", "check", "MAKECMDGOALS=x"}},
		{name: "-i with MAKECMDGOALS in env", args: []string{"-i", "check"}, env: []string{"MAKECMDGOALS=x"}},
		{name: "-n with MAKECMDGOALS on the command line", args: []string{"-n", "check", "MAKECMDGOALS=x"}},
		{name: "-n with MAKECMDGOALS in env", args: []string{"-n", "check"}, env: []string{"MAKECMDGOALS=x"}},
		{name: "MAKEFLAGS=i and MAKECMDGOALS in env", args: []string{"check"}, env: []string{"MAKECMDGOALS=x", "MAKEFLAGS=i"}},
		{name: "GATE and MAKECMDGOALS on the command line", args: []string{"check", "MAKECMDGOALS=x", "GATE=true"}},
		{name: "SHELL and MAKECMDGOALS on the command line", args: []string{"check", "MAKECMDGOALS=x", "SHELL=/bin/true"}},
		{name: "GOFLAGS and MAKECMDGOALS in env", args: []string{"check"}, env: []string{"MAKECMDGOALS=x", "GOFLAGS=-skip=TestLayout"}},
		{name: "MAKECMDGOALS from a makefile read before", args: []string{"-i", "-f", goals, "-f", "Makefile", "check"}},
		{name: "-i, second makefile and MAKEFILE_LIST", args: []string{"-i", "-f", "Makefile", "-f", double, "check", "MAKEFILE_LIST=Makefile"}},
		{name: "MAKEFILE_LIST on the command line", args: []string{"-f", "Makefile", "-f", double, "check", "MAKEFILE_LIST=Makefile"}},
		{name: "-i behind a goal alias from --eval", args: []string{"-i", "--eval", "x: check", "x"}},
		{name: "-k behind a goal alias and a second makefile", args: []string{"-k", "-f", "Makefile", "-f", double, "--eval", "x: check", "x"}},
		// MAKEOVERRIDES make передаёт под-make шагов переменными их командной строки.
		{name: "MAKEOVERRIDES in env", args: []string{"check"}, env: []string{"MAKEOVERRIDES=SHELL=/bin/true"}},
		{name: "MAKEOVERRIDES in env and a variable on the command line", args: []string{"check", "PKG=./pkg/..."}, env: []string{"MAKEOVERRIDES=SHELL=/bin/true"}},
		{name: "MAKEOVERRIDES on the command line", args: []string{"check", "MAKEOVERRIDES=SHELL=/bin/true"}},

		{name: "-o of a step", args: []string{"-o", "fmt-check", "-o", "lint", "check"}, gate: true},
		{name: "two goals", args: []string{"check", "lint"}, gate: true},
		{name: "PKG, VERBOSE, FORMAT and COVER_FLOOR", args: []string{"check", "PKG=./pkg/...", "VERBOSE=1", "FORMAT=detailed", "COVER_FLOOR=1"}, gate: true},
		{name: "guard variables on the command line", args: []string{"check", "gate_bad=", "gate_refuse=", "gate_forged="}, gate: true},
		{name: "BASH_ENV in env", args: []string{"check"}, env: shell["BASH_ENV"], gate: true},
		{name: "SHELLOPTS=noexec in env", args: []string{"check"}, env: shell["SHELLOPTS=noexec"], gate: true},
		{name: "exported function set in env", args: []string{"check"}, env: shell["function set"], gate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.CommandContext(t.Context(), "make", tc.args...)
			cmd.Dir = repo
			cmd.Env = callerEnv(append([]string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, tc.env...)...)
			data, err := cmd.CombinedOutput()
			out := string(data)
			require.Error(t, err, "make %v must fail here:\n%s", tc.args, out)
			require.NotContains(t, out, "✓ check clean", "make %v forges the verdict of the gate", tc.args)
			if !tc.gate {
				require.NotContains(t, out, "→ make fmt-check", "make %v must refuse to start the gate", tc.args)
				return
			}
			require.Contains(t, out, "→ make fmt-check", "make %v must run the real gate", tc.args)
			require.NotContains(t, out, "→ go vet", "make %v went past the failed step", tc.args)
		})
	}
}

// TestCheckCoversItsOwnRun: cover-check гейта читает профиль своего test-int — coverage.out прежнего прогона
// гейт снимает до шагов, и test-int, который профиля не написал, чужим не прикрыть. Шаги здесь — зелёные
// заглушки, cover.sh — настоящий.
func TestCheckCoversItsOwnRun(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	repo, bin := t.TempDir(), t.TempDir()
	tools, err := os.ReadDir(filepath.Join(root, "scripts", "tools"))
	require.NoError(t, err)
	files := map[string]string{"coverage.out": "mode: atomic\nprofile of an earlier run\n"}
	for _, tool := range tools {
		files["scripts/tools/"+tool.Name()] = "exit 0\n"
	}
	ok, err := exec.LookPath("true")
	require.NoError(t, err)
	for _, tool := range []string{"gofumpt", "go", "sqlc", "mockery"} {
		require.NoError(t, os.Symlink(ok, filepath.Join(bin, tool)))
	}
	writeFiles(t, repo, union(files, readFiles(t, root, "Makefile", "scripts/tools/cover.sh")))
	git(t, repo, "init", "-q")

	out, err := runEnv(t, repo, []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, "make", "check")
	require.Error(t, err, "the gate must fail without a profile of its own test-int:\n%s", out)
	require.Contains(t, out, "FAIL: нет профиля coverage.out", "cover-check read the profile of an earlier run:\n%s", out)
	require.NotContains(t, out, "✓ check clean")
}

// TestPrePushGatesPushedCommit: pre-push гоняет make check верхнего коммита пушимого ref'а в свежем
// worktree — по дереву и Makefile самого коммита, а переменные make и оболочки пушащего до гейта не
// доходят: каждая из них меняет шаги или выключает его самозащиту. Гейт здесь игрушечный: Makefile коммита печатает вердикт
// (файл verdict) и то, что дошло до него из окружения.
func TestPrePushGatesPushedCommit(t *testing.T) {
	t.Parallel()
	hook := filepath.Join(repoRoot(t), "scripts", "git-hooks", "pre-push")
	require.FileExists(t, hook) // хук читает bash: без этого кеш go test не видит его правок
	repo, tmp := t.TempDir(), t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	commit := func(verdict string) string {
		t.Helper()
		writeFiles(t, repo, map[string]string{
			"Makefile": "check:\n\t@echo \"toy gate: $$(cat verdict) goals=$(MAKECMDGOALS) make=$(notdir $(MAKE)) flags=[$(MAKEFLAGS)] shell=[$${BASH_ENV-}$${SHELLOPTS-}$${BASHOPTS-}]\"; test \"$$(cat verdict)\" = green\n",
			"verdict":  verdict + "\n",
		})
		commitAll(t, repo)
		return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	}
	push := func(stdin string, env ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), hook, "origin", "git@example.org:acme/backend.git")
		cmd.Dir = repo
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Env = callerEnv(append([]string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + tmp}, env...)...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	zero := strings.Repeat("0", 40)
	red := commit("red")

	out, err := push("refs/heads/main " + red + " refs/heads/main " + zero + "\n")
	require.Error(t, err, "a red commit must not be pushed:\n%s", out)
	require.Contains(t, out, "toy gate: red goals=check make=make flags=[] shell=[]")

	forge := filepath.Join(tmp, "forge.mk")
	writeFiles(t, tmp, map[string]string{"forge.mk": "check::\n\t@echo forged\n"})
	out, err = push("refs/heads/main "+red+" refs/heads/main "+zero+"\n",
		"MAKEFLAGS=i", "MFLAGS=-i", "GNUMAKEFLAGS=-i", "MAKEFILES="+forge, "MAKECMDGOALS=x", "MAKE=true", "MAKELEVEL=3")
	require.Error(t, err, "make variables of the pusher's environment let a red commit through:\n%s", out)
	require.Contains(t, out, "toy gate: red goals=check make=make flags=[] shell=[]", "make variables of the pusher reach the gate")
	require.NotContains(t, out, "forged")

	// Окружение оболочки пушащего: bash-хук его не читает, и до гейта оно не доходит.
	for name, env := range shellForgeries(t) {
		out, err = push("refs/heads/main "+red+" refs/heads/main "+zero+"\n", env...)
		require.Error(t, err, "%s in the pusher's environment lets a red commit through:\n%s", name, out)
		require.Contains(t, out, "toy gate: red goals=check make=make flags=[] shell=[]", "%s reaches the gate", name)
	}

	green := commit("green")
	out, err = push("refs/heads/main " + green + " refs/heads/main " + red + "\n")
	require.NoError(t, err, "a green commit must be pushed:\n%s", out)
	require.Contains(t, out, "toy gate: green")

	out, err = push("refs/heads/old " + zero + " refs/heads/old " + red + "\n")
	require.NoError(t, err, "a deleted ref has nothing to check:\n%s", out)
	require.NotContains(t, out, "toy gate")
}

// TestGateStepsIgnoreShellEnvironment: шаги-скрипты гейта (bash) на красном дереве остаются красными при
// любом окружении оболочки вызывающего — и там, где рецепт make идёт в bash (SHELL=/bin/bash). Дерево
// игрушечное: Makefile и скрипты шаблона, линтер — false, go — заглушка без go test.
func TestGateStepsIgnoreShellEnvironment(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	repo, bin := t.TempDir(), stubTools(t, "golangci-lint")
	writeFiles(t, repo, readFiles(t, root, "Makefile", "scripts/tools/lint.sh", "scripts/tools/test.sh"))
	git(t, repo, "init", "-q", "-b", "main")
	forgeries := shellForgeries(t)
	forgeries["clean environment"] = nil
	for step, refusal := range map[string]string{"lint": "FAIL: .golangci.yml", "test": "FAIL: go test exit"} {
		for name, env := range forgeries {
			for _, shell := range []string{"", "SHELL=/bin/bash"} {
				args := []string{"--no-print-directory", step}
				if shell != "" {
					args = append(args, shell)
				}
				cmd := exec.CommandContext(t.Context(), "make", args...)
				cmd.Dir = repo
				cmd.Env = callerEnv(append([]string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, env...)...)
				out, err := cmd.CombinedOutput()
				require.Error(t, err, "make %v with %s: the step must stay red:\n%s", args, name, out)
				require.Contains(t, string(out), refusal, "make %v with %s: the step must run", args, name)
			}
		}
	}
}

// TestHooksIgnoreCallerEnvironment: хуки git — bash: окружение оболочки коммитящего (файл автозапуска,
// функции, опции) их не выключает, а переменные make и оболочки не доходят до проверок, которые хук
// запускает. commit-msg отвергает сообщение не по форме; pre-commit падает на первом шаге (gofumpt — false)
// и дальше не идёт.
func TestHooksIgnoreCallerEnvironment(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	commitMsg, preCommit := filepath.Join(root, "scripts", "git-hooks", "commit-msg"), filepath.Join(root, "scripts", "git-hooks", "pre-commit")
	require.FileExists(t, commitMsg) // хуки читает bash: без этого кеш go test не видит их правок
	require.FileExists(t, preCommit)
	repo, bin := t.TempDir(), stubTools(t, "gofumpt")
	writeFiles(t, repo, union(readFiles(t, root, "Makefile"), map[string]string{"COMMIT_EDITMSG": "update stuff\n"}))
	git(t, repo, "init", "-q", "-b", "main")
	forgeries := shellForgeries(t)
	forgeries["make flags"] = []string{"MAKEFLAGS=i", "MAKECMDGOALS=x"}
	for name, env := range forgeries {
		run := func(hook string, args ...string) (string, error) {
			cmd := exec.CommandContext(t.Context(), hook, args...)
			cmd.Dir = repo
			cmd.Env = callerEnv(append([]string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, env...)...)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}
		out, err := run(commitMsg, filepath.Join(repo, "COMMIT_EDITMSG"))
		require.Error(t, err, "%s: commit-msg accepts a message that is not a Conventional Commit:\n%s", name, out)

		out, err = run(preCommit)
		require.Error(t, err, "%s: pre-commit passes a red tree:\n%s", name, out)
		require.Contains(t, out, "FAIL: gofumpt", "%s: pre-commit must run its first step", name)
		require.NotContains(t, out, "pre-commit: go vet", "%s: pre-commit went past the failed step", name)
	}
}

// releaseSandbox — репо с Makefile и release.sh шаблона на заглушках: скрипты шагов гейта, preflight и
// инструменты (go, sqlc, mockery) отвечают успехом, smoke называет себя и падает.
type releaseSandbox struct {
	repo string
	bin  string
}

// newReleaseSandbox: gofumpt — программа на месте gofumpt (false роняет первый шаг гейта); files — файлы
// репо поверх заглушек.
func newReleaseSandbox(t *testing.T, gofumpt string, files map[string]string) releaseSandbox {
	t.Helper()
	root := repoRoot(t)
	s := releaseSandbox{repo: t.TempDir(), bin: t.TempDir()}
	tools, err := os.ReadDir(filepath.Join(root, "scripts", "tools"))
	require.NoError(t, err)
	stubs := map[string]string{"scripts/release-preflight.sh": "exit 0\n"}
	for _, tool := range tools {
		stubs["scripts/tools/"+tool.Name()] = "exit 0\n"
	}
	stubs["scripts/tools/e2e.sh"] = "echo \"SMOKE of the Makefile: $*\"\nexit 1\n"
	for tool, program := range map[string]string{"gofumpt": gofumpt, "go": "true", "sqlc": "true", "mockery": "true"} {
		path, err := exec.LookPath(program)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(path, filepath.Join(s.bin, tool)))
	}
	writeFiles(t, s.repo, union(stubs, readFiles(t, root, "Makefile", "scripts/release.sh"), files))
	git(t, s.repo, "init", "-q", "-b", "main")
	git(t, s.repo, "remote", "add", "origin", "git@git.example.org:acme/backend.git")
	commitAll(t, s.repo)
	return s
}

// release запускает релиз v9.9.9 без окружения вызывающего make и git (тест идёт и внутри make check) и
// отдаёт его вывод; env — поверх.
func (s releaseSandbox) release(t *testing.T, env ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash", "scripts/release.sh", "v9.9.9")
	cmd.Dir = s.repo
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, "GIT_") && !strings.HasPrefix(key, "MAKE") && key != "MFLAGS" && key != "GNUMAKEFLAGS" && key != "PATH" {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "RELEASE_YES=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"PATH="+s.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestReleaseGateIgnoresGNUmakefile: make читает GNUmakefile раньше Makefile, а -f в под-make не
// переходит — свой GNUmakefile (и игнорируемый git: дерево «чистое») подменил бы make check релиза, его
// шаги или smoke. Релиз зовёт make с -f Makefile, шаги гейта — тоже. Гейт здесь — Makefile репо на
// заглушках инструментов: с gofumpt-false падает его первый шаг, с зелёными шагами — smoke.
func TestReleaseGateIgnoresGNUmakefile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		gofumpt string
		want    []string
	}{
		{name: "gate", gofumpt: "false", want: []string{"FAIL: gofumpt"}},
		{name: "smoke after a green gate", gofumpt: "true", want: []string{"✓ check clean", "SMOKE of the Makefile: --smoke"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newReleaseSandbox(t, tc.gofumpt, nil)
			writeFiles(t, s.repo, map[string]string{
				"GNUmakefile":       "check e2e-smoke fmt-check lint test-int build gen-check:\n\t@echo \"FORGED $@\"\n",
				".git/info/exclude": "GNUmakefile\n",
			})

			out, err := s.release(t)
			require.Error(t, err, "the release must stop here:\n%s", out)
			require.NotContains(t, out, "FORGED", "a GNUmakefile replaces a step of the release")
			for _, want := range tc.want {
				require.Contains(t, out, want, "the release must run the steps of the Makefile")
			}
			require.Empty(t, git(t, s.repo, "tag", "-l"), "a failed step must leave no tag")
		})
	}
}

// TestReleaseStopsWithoutModuleAccess: сборка образа берёт приватные модули go.mod по SSH ключом
// ssh-agent. Без ключа релиз останавливается на release-preflight env с причиной — до гейта и до тега, а
// не на сборке образа после него.
func TestReleaseStopsWithoutModuleAccess(t *testing.T) {
	t.Parallel()
	s := newReleaseSandbox(t, "true", readFiles(t, repoRoot(t), "scripts/release-preflight.sh", "go.mod", "Dockerfile"))

	out, err := s.release(t, "SSH_AUTH_SOCK=")
	require.Error(t, err, "the release must stop without an ssh-agent key:\n%s", out)
	require.Contains(t, out, "release-preflight: в ssh-agent нет ключей", "the refusal names its reason")
	require.NotContains(t, out, "→ make fmt-check", "the environment of the release is checked before its gate")
	require.Empty(t, git(t, s.repo, "tag", "-l"), "a release that cannot build its images must leave no tag")
}

// TestReleasePreflight: фазы release-preflight.sh. env — окружение релиза до гейта: ветка, чистое дерево,
// ключи ssh-agent читают приватные модули go.mod (хосты — GOPRIVATE из Dockerfile); tag — тег на HEAD
// после гейта: annotated, формата vX.Y.Z и тот, что запрошен. ssh-add, ssh и docker здесь — заглушки в PATH.
func TestReleasePreflight(t *testing.T) {
	t.Parallel()
	script := readFiles(t, repoRoot(t), "scripts/release-preflight.sh")
	env := func(require string) map[string]string {
		return map[string]string{
			"Dockerfile":              "ENV GOPRIVATE=git.example.org/*\n",
			"go.mod":                  "module git.example.org/acme/backend\n\nrequire " + require + "\n",
			"scripts/tools/doctor.sh": "echo DOCTOR\n",
		}
	}
	private, public := env("git.example.org/acme/kit v1.2.3"), env("github.com/acme/kit v1.2.3")
	for _, tc := range []struct {
		name   string
		branch string
		files  map[string]string
		tag    []string // аргументы git tag
		dirty  bool
		sshAdd string // программа на месте ssh-add: false — в агенте нет ключей
		args   []string
		want   string
		ssh    bool // preflight идёт по ssh за приватным модулем
		ok     bool
	}{
		{name: "annotated semver tag", tag: []string{"-a", "v1.2.3", "-m", "release"}, args: []string{"tag", "v1.2.3"}, want: "OK (tag=v1.2.3)", ok: true},
		{name: "no tag", args: []string{"tag", "v1.2.3"}, want: "на HEAD нет тега"},
		{name: "another tag", tag: []string{"-a", "v1.2.3", "-m", "release"}, args: []string{"tag", "v1.2.4"}, want: "≠ тег на HEAD (v1.2.3)"},
		{name: "lightweight tag", tag: []string{"v1.2.3"}, args: []string{"tag", "v1.2.3"}, want: "не annotated"},
		{name: "not semver", tag: []string{"-a", "v1.2", "-m", "release"}, args: []string{"tag", "v1.2"}, want: "не соответствует формату vX.Y.Z"},
		{name: "tag phase without a version", args: []string{"tag"}, want: "usage"},
		{name: "no phase", want: "usage"},
		{name: "not main", branch: "feature", files: public, args: []string{"env"}, want: "branch=feature, релиз только с main"},
		{name: "dirty tree", files: public, dirty: true, args: []string{"env"}, want: "working tree dirty"},
		{name: "no key in the ssh-agent", files: private, sshAdd: "false", args: []string{"env"}, want: "в ssh-agent нет ключей"},
		{
			name: "a private module is closed to the key", files: private, args: []string{"env"},
			want: "git@git.example.org:acme/kit.git не читается ключами ssh-agent", ssh: true,
		},
		{name: "no private modules", files: public, args: []string{"env"}, want: "release-preflight: OK (окружение)", ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, bin := t.TempDir(), t.TempDir()
			for tool, program := range map[string]string{"ssh-add": tc.sshAdd, "docker": "true"} {
				if program == "" {
					program = "true"
				}
				path, err := exec.LookPath(program)
				require.NoError(t, err)
				require.NoError(t, os.Symlink(path, filepath.Join(bin, tool)))
			}
			// ssh-заглушка записывает свои аргументы и отказывает: хост ключу не открывается.
			sshArgs := filepath.Join(bin, "ssh.args")
			require.NoError(t, os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >\"$0.args\"\nexit 1\n"), 0o700))
			writeFiles(t, repo, union(script, tc.files))
			branch := tc.branch
			if branch == "" {
				branch = "main"
			}
			git(t, repo, "init", "-q", "-b", branch)
			commitAll(t, repo)
			if tc.tag != nil {
				git(t, repo, append([]string{"tag"}, tc.tag...)...)
			}
			if tc.dirty {
				writeFiles(t, repo, map[string]string{"untracked.txt": "x\n"})
			}

			cmd := exec.CommandContext(t.Context(), "bash", append([]string{"scripts/release-preflight.sh"}, tc.args...)...)
			cmd.Dir = repo
			for _, kv := range os.Environ() {
				if key, _, _ := strings.Cut(kv, "="); !strings.HasPrefix(key, "GIT_") && key != "PATH" {
					cmd.Env = append(cmd.Env, kv)
				}
			}
			cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			data, err := cmd.CombinedOutput()
			out := string(data)
			require.Contains(t, out, tc.want)
			if called, readErr := os.ReadFile(sshArgs); tc.ssh {
				require.NoError(t, readErr, "the preflight reads the private module over ssh: %s", out)
				require.Contains(t, string(called), "-F /dev/null -o BatchMode=yes -o IdentityFile=none",
					"the check sees what the image build will: the ssh-agent alone, no ~/.ssh/config and no key files")
				require.Contains(t, string(called), "git@git.example.org")
			} else {
				require.ErrorIs(t, readErr, os.ErrNotExist, "nothing to read over ssh here: %s", called)
			}
			if tc.ok {
				require.NoError(t, err, out)
				return
			}
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, out)
			require.Equal(t, 2, exit.ExitCode(), "a refusal of the preflight is exit 2: %s", out)
		})
	}
}

// TestReleaseBuildForwardsSSHAgent: приватные модули сборка образа берёт по SSH ключом ssh-agent — go mod
// download в Dockerfile'ах релиза требует ssh-mount, в окружении этой команды git идёт за модулем хоста
// GOPRIVATE по ssh, а ключ хоста в свежем образе принимается (accept-new); release.sh передаёт агент каждой
// сборке. Без любой из частей релиз упал бы на сборке образа уже после тега.
func TestReleaseBuildForwardsSSHAgent(t *testing.T) {
	t.Parallel()
	files := readFiles(t, repoRoot(t), "Dockerfile", "Dockerfile.migrate", "scripts/release.sh")
	commands := func(name string) []string {
		return strings.Split(strings.ReplaceAll(files[name], "\\\n", " "), "\n")
	}
	// fetches — адрес, по которому git пойдёт за url в окружении команды RUN: go в ней — заглушка, а
	// git ls-remote --get-url раскрывает insteadOf без сети.
	fetches := func(run, url string) string {
		bin := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexec git ls-remote --get-url "+url+"\n"), 0o700))
		script := regexp.MustCompile(`^RUN(\s+--\S+)*\s+`).ReplaceAllString(run, "")
		out, err := runEnv(t, t.TempDir(), []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, "sh", "-c", script)
		require.NoError(t, err, out)
		return out
	}
	for _, dockerfile := range []string{"Dockerfile", "Dockerfile.migrate"} {
		var hosts []string
		for _, cmd := range commands(dockerfile) {
			if private, ok := strings.CutPrefix(cmd, "ENV GOPRIVATE="); ok {
				for _, pattern := range strings.Split(private, ",") {
					hosts = append(hosts, strings.TrimSuffix(pattern, "/*"))
				}
			}
		}
		require.NotEmpty(t, hosts, "%s: GOPRIVATE names the hosts of the private modules", dockerfile)
		downloads := 0
		for _, cmd := range commands(dockerfile) {
			if strings.Contains(cmd, "go mod download") {
				downloads++
				require.Contains(t, cmd, "--mount=type=ssh,required=true", "%s: go mod download takes private modules with the key of the ssh-agent", dockerfile)
				for _, host := range hosts {
					require.Equal(t, "git@"+host+":acme/kit\n", fetches(cmd, "https://"+host+"/acme/kit"),
						"%s: go fetches %s over https, the key of the ssh-agent works over ssh only", dockerfile, host)
				}
				require.Contains(t, cmd, "StrictHostKeyChecking=accept-new", "%s: a fresh image knows no host key, and BatchMode does not ask", dockerfile)
			}
		}
		require.Equal(t, 1, downloads, "%s: one RUN downloads the modules", dockerfile)
	}
	builds := 0
	for _, cmd := range commands("scripts/release.sh") {
		if strings.HasPrefix(cmd, "docker buildx build") {
			builds++
			require.Contains(t, cmd, " --ssh default ", "release.sh: every image build forwards the ssh-agent: %s", cmd)
		}
	}
	require.Equal(t, 2, builds, "release.sh builds the api and the migrate images")
}
