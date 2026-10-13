package archtest

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHooksVisible: CI нет — хуки git (commit-msg, pre-commit, pre-push) единственная проверка изменения
// до remote. Клон без make setup, хук без права исполнения и чужой каталог хуков make status называет
// строкой HOOKS и итогом ATTENTION: hooks, make doctor — отказом; хуки этого дерева — OK, и файл рядом с
// ними, чьё имя git хуком не считает, вердикт не меняет. sqlc и docker здесь — заглушки: в песочнице нет
// ни sqlc.yaml, ни registry, вердикт — только о хуках.
func TestHooksVisible(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	hooks, err := filepath.Glob(filepath.Join(root, "scripts", "git-hooks", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, hooks)
	files := readFiles(t, root, "scripts/tools/status.sh", "scripts/tools/doctor.sh")
	for _, h := range hooks {
		name := "scripts/git-hooks/" + filepath.Base(h)
		files[name] = readFiles(t, root, name)[name]
	}
	wire := func(t *testing.T, repo string) { git(t, repo, "config", "core.hooksPath", "scripts/git-hooks") }
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, repo string)
		want  string // причина в строке HOOKS; "" — хуки подключены
	}{
		{name: "fresh clone", setup: func(*testing.T, string) {}, want: "нет исполняемого commit-msg"},
		{name: "make setup", setup: wire},
		{name: "a readme and a library next to the hooks", setup: func(t *testing.T, repo string) {
			wire(t, repo)
			writeFiles(t, repo, map[string]string{"scripts/git-hooks/README.md": "# hooks\n", "scripts/git-hooks/lib.sh": "x=1\n"})
		}},
		{name: "pre-push not executable", setup: func(t *testing.T, repo string) {
			wire(t, repo)
			require.NoError(t, os.Chmod(filepath.Join(repo, "scripts", "git-hooks", "pre-push"), 0o644))
		}, want: "нет исполняемого pre-push"},
		{name: "hooks of another tree", setup: func(t *testing.T, repo string) {
			other := t.TempDir()
			for _, h := range hooks {
				require.NoError(t, os.WriteFile(filepath.Join(other, filepath.Base(h)), []byte("#!/bin/sh\nexit 0\n"), 0o700))
			}
			git(t, repo, "config", "core.hooksPath", other)
		}, want: "не хук этого дерева"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, bin := t.TempDir(), t.TempDir()
			writeFiles(t, repo, files)
			for _, h := range hooks {
				require.NoError(t, os.Chmod(filepath.Join(repo, "scripts", "git-hooks", filepath.Base(h)), 0o755))
			}
			git(t, repo, "init", "-q")
			commitAll(t, repo)
			tc.setup(t, repo)
			stub, err := exec.LookPath("true")
			require.NoError(t, err)
			for _, tool := range []string{"sqlc", "docker"} {
				require.NoError(t, os.Symlink(stub, filepath.Join(bin, tool)))
			}
			path := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}

			status, err := runEnv(t, repo, path, "bash", "scripts/tools/status.sh")
			require.NoError(t, err, status)
			doctor, doctorErr := runEnv(t, repo, path, "bash", "scripts/tools/doctor.sh")
			if tc.want == "" {
				require.Contains(t, status, "\nHOOKS: OK\n")
				require.True(t, strings.HasSuffix(status, "\nOK\n"), "wired hooks raise no attention:\n%s", status)
				require.NoError(t, doctorErr, doctor)
				return
			}
			require.Regexp(t, `\nHOOKS: не подключены — make setup \(.*`+tc.want, status)
			require.True(t, strings.HasSuffix(status, "\nATTENTION: hooks\n"), "status must raise the attention of the cockpit:\n%s", status)
			require.Contains(t, doctor, "FAIL : git hooks: не подключены — make setup")
			var exit *exec.ExitError
			require.True(t, errors.As(doctorErr, &exit) && exit.ExitCode() == 2, "doctor refuses without hooks: %v\n%s", doctorErr, doctor)
		})
	}
}

// TestSetupWiresHooksFirst: make setup подключает хуки до установки инструментов — её сбой (нет сети, нет
// go) не оставляет клон без хуков.
func TestSetupWiresHooksFirst(t *testing.T) {
	t.Parallel()
	repo, bin := t.TempDir(), t.TempDir()
	writeFiles(t, repo, readFiles(t, repoRoot(t), "Makefile", "scripts/install-tools.sh"))
	git(t, repo, "init", "-q")
	commitAll(t, repo)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 1\n"), 0o700))

	out, err := runEnv(t, repo, []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, "make", "setup")
	require.Error(t, err, "go fails here, the tools are not installed:\n%s", out)
	require.Equal(t, "scripts/git-hooks", strings.TrimSpace(git(t, repo, "rev-parse", "--git-path", "hooks")),
		"make setup wires the hooks before it installs the tools:\n%s", out)
}
