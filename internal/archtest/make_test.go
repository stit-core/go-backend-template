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

// TestReleasePreflight: фазы release-preflight.sh — окружение (ветка) до гейта и тег на HEAD после него:
// annotated, формата vX.Y.Z и тот, что запрошен.
func TestReleasePreflight(t *testing.T) {
	t.Parallel()
	script := readFiles(t, repoRoot(t), "scripts/release-preflight.sh")
	for _, tc := range []struct {
		name   string
		branch string
		tag    []string // аргументы git tag
		args   []string
		want   string // подстрока вывода; у успеха — «OK»
		ok     bool
	}{
		{name: "annotated semver tag", tag: []string{"-a", "v1.2.3", "-m", "release"}, args: []string{"tag", "v1.2.3"}, want: "OK (tag=v1.2.3)", ok: true},
		{name: "no tag", args: []string{"tag", "v1.2.3"}, want: "на HEAD нет тега"},
		{name: "another tag", tag: []string{"-a", "v1.2.3", "-m", "release"}, args: []string{"tag", "v1.2.4"}, want: "≠ тег на HEAD (v1.2.3)"},
		{name: "lightweight tag", tag: []string{"v1.2.3"}, args: []string{"tag", "v1.2.3"}, want: "не annotated"},
		{name: "not semver", tag: []string{"-a", "v1.2", "-m", "release"}, args: []string{"tag", "v1.2"}, want: "не соответствует формату vX.Y.Z"},
		{name: "tag phase without a version", args: []string{"tag"}, want: "usage"},
		{name: "no phase", want: "usage"},
		{name: "not main", branch: "feature", args: []string{"env"}, want: "branch=feature, релиз только с main"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFiles(t, repo, script)
			branch := tc.branch
			if branch == "" {
				branch = "main"
			}
			git(t, repo, "init", "-q", "-b", branch)
			commitAll(t, repo)
			if tc.tag != nil {
				git(t, repo, append([]string{"tag"}, tc.tag...)...)
			}

			out, err := runIn(t, repo, "bash", append([]string{"scripts/release-preflight.sh"}, tc.args...)...)
			require.Contains(t, out, tc.want)
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
// download в Dockerfile'ах релиза требует ssh-mount, а release.sh передаёт агент каждой сборке. Без одной
// из половин релиз упал бы на сборке образа уже после тега.
func TestReleaseBuildForwardsSSHAgent(t *testing.T) {
	t.Parallel()
	files := readFiles(t, repoRoot(t), "Dockerfile", "Dockerfile.migrate", "scripts/release.sh")
	commands := func(name string) []string {
		return strings.Split(strings.ReplaceAll(files[name], "\\\n", " "), "\n")
	}
	for _, dockerfile := range []string{"Dockerfile", "Dockerfile.migrate"} {
		downloads := 0
		for _, cmd := range commands(dockerfile) {
			if strings.Contains(cmd, "go mod download") {
				downloads++
				require.Contains(t, cmd, "--mount=type=ssh,required=true", "%s: go mod download takes private modules with the key of the ssh-agent", dockerfile)
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
