package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMakeCheckCannotBeForged: флаги make, подмена переменных гейта и лишний makefile (check: или check::,
// до или после Makefile, MAKEFILES) не подделывают «✓ check clean». Makefile под подделку (переприсваивает
// MAKEFILE_LIST) обходит любую сверку внутри make — это правка гейта, вердикт для remote — pre-push.
// Настоящий гейт здесь падает на первом шаге: gofumpt в PATH — false, поэтому проба не гоняет гейт целиком.
func TestMakeCheckCannotBeForged(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	bin, fakes := t.TempDir(), t.TempDir()
	fail, err := exec.LookPath("false")
	require.NoError(t, err)
	require.NoError(t, os.Symlink(fail, filepath.Join(bin, "gofumpt")))
	writeFiles(t, fakes, map[string]string{
		"colon.mk":  "check:\n\t@echo \"✓ check clean\"\n",
		"double.mk": "check::\n\t@echo \"✓ check clean\"\n",
		"shell.mk":  "override SHELL := /bin/true\n",
	})
	colon, double, shell := filepath.Join(fakes, "colon.mk"), filepath.Join(fakes, "double.mk"), filepath.Join(fakes, "shell.mk")
	for _, tc := range []struct {
		name string
		args []string
		env  []string
		gate bool // настоящий гейт запускается (и падает на заглушке)
	}{
		{name: "make check", args: []string{"check"}, gate: true},
		{name: "-f ./Makefile", args: []string{"-f", "./Makefile", "check"}, gate: true},
		{name: "second makefile with check:", args: []string{"-f", "Makefile", "-f", colon, "check"}},
		{name: "-o and second makefile", args: []string{"-o", "gate-makefiles", "-f", "Makefile", "-f", colon, "check"}},
		{name: "second makefile with check:: after", args: []string{"-f", "Makefile", "-f", double, "check"}},
		{name: "second makefile with check:: before", args: []string{"-f", double, "-f", "Makefile", "check"}},
		{name: "second makefile with SHELL after", args: []string{"-f", "Makefile", "-f", shell, "check"}},
		{name: "MAKEFILES", args: []string{"check"}, env: []string{"MAKEFILES=" + double}},
		{name: "--eval", args: []string{"--eval", "SHELL:=/bin/true", "check"}},
		{name: "-E rule", args: []string{"-E", "check:: ; @echo \"✓ check clean\"", "check"}},
		{name: "-k", args: []string{"-k", "-f", "Makefile", "-f", double, "check"}},
		{name: "-j", args: []string{"-j2", "-f", "Makefile", "-f", double, "check"}},
		{name: "-p", args: []string{"-p", "check"}},
		{name: "-i", args: []string{"-i", "check"}},
		{name: "-e with GATE", args: []string{"-e", "check"}, env: []string{"GATE=true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.CommandContext(t.Context(), "make", tc.args...)
			cmd.Dir = root
			// Без окружения вызывающего make: тест идёт и внутри make check.
			for _, kv := range os.Environ() {
				key, _, _ := strings.Cut(kv, "=")
				switch key {
				case "MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKELEVEL", "MAKEFILES", "MAKEOVERRIDES", "PATH":
				default:
					cmd.Env = append(cmd.Env, kv)
				}
			}
			cmd.Env = append(cmd.Env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			cmd.Env = append(cmd.Env, tc.env...)
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "make %v must fail here:\n%s", tc.args, out)
			require.NotContains(t, string(out), "✓ check clean", "make %v forges the verdict of the gate", tc.args)
			if tc.gate {
				require.Contains(t, string(out), "→ make fmt-check", "make %v must run the real gate", tc.args)
			}
		})
	}
}

// TestReleaseGateIgnoresGNUmakefile: make читает GNUmakefile раньше Makefile, а -f в под-make не
// переходит — свой GNUmakefile (и игнорируемый git: дерево «чистое») подменил бы make check релиза, его
// шаги или smoke. Релиз зовёт make с -f Makefile, шаги гейта — тоже. Гейт здесь — Makefile репо на
// заглушках инструментов: с gofumpt-false падает его первый шаг, с зелёными шагами — smoke.
func TestReleaseGateIgnoresGNUmakefile(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	tools, err := os.ReadDir(filepath.Join(root, "scripts", "tools"))
	require.NoError(t, err)
	stubs := map[string]string{}
	for _, tool := range tools {
		stubs["scripts/tools/"+tool.Name()] = "exit 0\n"
	}
	stubs["scripts/tools/e2e.sh"] = "echo \"SMOKE of the Makefile: $*\"\nexit 1\n"
	for _, tc := range []struct {
		name    string
		gofumpt string // программа на месте gofumpt: false роняет первый шаг гейта
		want    []string
	}{
		{name: "gate", gofumpt: "false", want: []string{"FAIL: gofumpt"}},
		{name: "smoke after a green gate", gofumpt: "true", want: []string{"✓ check clean", "SMOKE of the Makefile: --smoke"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, bin := t.TempDir(), t.TempDir()
			for tool, program := range map[string]string{"gofumpt": tc.gofumpt, "go": "true", "sqlc": "true", "mockery": "true"} {
				path, err := exec.LookPath(program)
				require.NoError(t, err)
				require.NoError(t, os.Symlink(path, filepath.Join(bin, tool)))
			}
			writeFiles(t, repo, union(stubs, readFiles(t, root, "Makefile", "scripts/release.sh")))
			git(t, repo, "init", "-q")
			git(t, repo, "remote", "add", "origin", "git@git.example.org:acme/backend.git")
			commitAll(t, repo)
			writeFiles(t, repo, map[string]string{
				"GNUmakefile":       "check e2e-smoke fmt-check lint test-int build gen-check:\n\t@echo \"FORGED $@\"\n",
				".git/info/exclude": "GNUmakefile\n",
			})

			cmd := exec.CommandContext(t.Context(), "bash", "scripts/release.sh", "v9.9.9")
			cmd.Dir = repo
			// Без окружения вызывающего make и git: тест идёт и внутри make check.
			for _, kv := range os.Environ() {
				key, _, _ := strings.Cut(kv, "=")
				if !strings.HasPrefix(key, "GIT_") && !strings.HasPrefix(key, "MAKE") && key != "MFLAGS" && key != "GNUMAKEFLAGS" && key != "PATH" {
					cmd.Env = append(cmd.Env, kv)
				}
			}
			cmd.Env = append(cmd.Env, "RELEASE_YES=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "the release must stop here:\n%s", out)
			require.NotContains(t, string(out), "FORGED", "a GNUmakefile replaces a step of the release")
			for _, want := range tc.want {
				require.Contains(t, string(out), want, "the release must run the steps of the Makefile")
			}
			require.Empty(t, git(t, repo, "tag", "-l"), "a failed step must leave no tag")
		})
	}
}
