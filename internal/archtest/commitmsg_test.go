package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCommitMsgHook: первая строка коммита — Conventional Commit с типом из CLAUDE.md § Git workflow или
// сообщение самого git; предел subject — 72 символа, а не байта, при любой локали коммитящего (под
// LC_ALL=C русская буква — два байта).
func TestCommitMsgHook(t *testing.T) {
	t.Parallel()
	hook := filepath.Join(repoRoot(t), "scripts", "git-hooks", "commit-msg")
	require.FileExists(t, hook) // скрипт читает bash: без этого кеш go test не видит его правок
	long := strings.Repeat("я", 72)
	for _, tc := range []struct {
		name string
		msg  string
		ok   bool
	}{
		{name: "type and scope", msg: "feat(user): add password reset\n", ok: true},
		{name: "no scope, body", msg: "fix: ключ ошибки логов — error\n\nтело\n", ok: true},
		{name: "meta", msg: "meta(plan): план закрыт\n", ok: true},
		{name: "scope with slash", msg: "fix(auth/session): ротация\n", ok: true},
		{name: "breaking mark", msg: "feat(api)!: ответ без поля\n", ok: true},
		{name: "72 cyrillic letters", msg: "fix(gate): " + long + "\n", ok: true},
		{name: "72 letters and trailing space", msg: "fix(gate): " + long + "  \r\n", ok: true},
		{name: "72 mixed characters", msg: "fix(gate): " + strings.Repeat("я→x", 24) + "\n", ok: true},
		{name: "git revert", msg: "Revert \"fix(gate): pre-push проверяет пушимый коммит\"\n\nThis reverts commit abc.\n", ok: true},
		{name: "git reapply", msg: "Reapply \"fix(gate): pre-push проверяет пушимый коммит\"\n", ok: true},
		{name: "git fixup", msg: "fixup! fix(gate): pre-push проверяет пушимый коммит\n", ok: true},
		{name: "git squash", msg: "squash! fix(gate): pre-push проверяет пушимый коммит\n", ok: true},
		{name: "git amend", msg: "amend! fix(gate): pre-push проверяет пушимый коммит\n", ok: true},
		{name: "73 cyrillic letters", msg: "fix(gate): " + long + "я\n"},
		{name: "73 latin letters", msg: "fix(gate): " + strings.Repeat("x", 73) + "\n"},
		{name: "free text", msg: "update stuff\n"},
		{name: "unknown type", msg: "ci: gitea actions\n"},
		{name: "capital type", msg: "Fix(gate): тип с заглавной\n"},
		{name: "capital scope", msg: "docs(README): scope с заглавной\n"},
		{name: "empty scope", msg: "fix(): пустой scope\n"},
		{name: "no space after colon", msg: "fix(gate):без пробела\n"},
		{name: "empty subject", msg: "fix(gate): \n"},
		{name: "merge", msg: "Merge branch 'main' into feature\n"},
		{name: "empty message", msg: "\n"},
	} {
		for _, locale := range []string{"C", "C.UTF-8"} {
			t.Run(locale+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				file := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
				require.NoError(t, os.WriteFile(file, []byte(tc.msg), 0o600))
				cmd := exec.CommandContext(t.Context(), "bash", hook, file)
				cmd.Env = append(os.Environ(), "LC_ALL="+locale)
				out, err := cmd.CombinedOutput()
				if tc.ok {
					require.NoError(t, err, "commit-msg refuses a valid message: %s", out)
					return
				}
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "commit-msg accepts %q: %s", tc.msg, out)
				require.Equal(t, 1, exit.ExitCode(), "commit-msg must refuse, not break: %s", out)
			})
		}
	}
}
