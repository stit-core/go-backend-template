package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMakeCheckCannotBeForged: «✓ check clean» печатает только настоящий гейт — ни флаги make, ни второй
// makefile, ни подмена переменных его не подделывают. Настоящий гейт здесь падает на первом шаге:
// gofumpt в PATH — false, поэтому проба не гоняет гейт целиком.
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
