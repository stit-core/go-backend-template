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

// TestCommitMsgHook: первая строка коммита — Conventional Commit с типом из CLAUDE.md § Git workflow (тот же
// список, что TYPES хука) или сообщение самого git; предел subject — 72 символа, а не байта, при любой локали
// коммитящего (под LC_ALL=C русская буква — два байта); у feat и fix в теле — строка Review: (CLAUDE.md § Готово).
func TestCommitMsgHook(t *testing.T) {
	t.Parallel()
	hook := filepath.Join(repoRoot(t), "scripts", "git-hooks", "commit-msg")
	require.FileExists(t, hook) // скрипт читает bash: без этого кеш go test не видит его правок
	files := readFiles(t, repoRoot(t), "scripts/git-hooks/commit-msg", "CLAUDE.md")
	types := regexp.MustCompile(`(?m)^TYPES='([a-z|]+)'$`).FindStringSubmatch(files["scripts/git-hooks/commit-msg"])
	require.NotNil(t, types, "commit-msg: no TYPES='…' line")
	prose := regexp.MustCompile("типы `([a-z|]+)`").FindStringSubmatch(files["CLAUDE.md"])
	require.NotNil(t, prose, "CLAUDE.md § Git workflow: no «типы `a|b|…`»")
	require.Equal(t, types[1], prose[1], "the commit types of CLAUDE.md § Git workflow are not the types of the commit-msg hook")
	long := strings.Repeat("я", 72)
	const review = "\n\nReview: /code-review — находок нет\n"
	const (
		notConventional = "должна быть Conventional Commit"
		tooLong         = "длиннее 72 символов"
		noReview        = "нет строки Review:"
	)
	for _, tc := range []struct {
		name string
		msg  string
		why  string // "" — хук принимает сообщение
	}{
		{name: "type and scope", msg: "feat(user): add password reset" + review},
		{name: "no scope, body", msg: "fix: ключ ошибки логов — error\n\nтело\n\nReview: принимает хаб, infra:auth\n"},
		{name: "meta", msg: "meta(plan): план закрыт\n"},
		{name: "refactor without review", msg: "refactor(user): один rowToEntity\n"},
		{name: "scope with slash", msg: "fix(auth/session): ротация" + review},
		{name: "breaking mark", msg: "feat(api)!: ответ без поля" + review},
		{name: "review among trailers, CRLF", msg: "fix(gate): x\r\n\r\nтело\r\n\r\nReview: дирижёр — PASS\r\nCo-Authored-By: a <b@c>\r\n"},
		{name: "72 cyrillic letters", msg: "fix(gate): " + long + review},
		{name: "72 letters and trailing space", msg: "fix(gate): " + long + "  \r\n\r\nReview: x\r\n"},
		{name: "72 mixed characters", msg: "test(gate): " + strings.Repeat("я→x", 24) + "\n"},
		{name: "git revert", msg: "Revert \"fix(gate): pre-push проверяет пушимый коммит\"\n\nThis reverts commit abc.\n"},
		{name: "git reapply", msg: "Reapply \"fix(gate): pre-push проверяет пушимый коммит\"\n"},
		{name: "git fixup", msg: "fixup! fix(gate): pre-push проверяет пушимый коммит\n"},
		{name: "git squash", msg: "squash! fix(gate): pre-push проверяет пушимый коммит\n"},
		{name: "git amend", msg: "amend! fix(gate): pre-push проверяет пушимый коммит\n"},
		{name: "feat without review", msg: "feat(user): add password reset\n", why: noReview},
		{name: "fix, body without review", msg: "fix: ключ ошибки логов — error\n\nтело\n", why: noReview},
		{name: "review in the subject", msg: "fix(gate): Review: в subject\n", why: noReview},
		{name: "empty review", msg: "feat(api)!: ответ без поля\n\nReview: \n", why: noReview},
		{name: "commented review", msg: "fix(gate): x\n\n# Review: в комментарии\n", why: noReview},
		{name: "indented review", msg: "fix(gate): x\n\n  Review: с отступом\n", why: noReview},
		{name: "lowercase review", msg: "fix(gate): x\n\nreview: строчная\n", why: noReview},
		{name: "73 cyrillic letters", msg: "fix(gate): " + long + "я" + review, why: tooLong},
		{name: "73 latin letters", msg: "test(gate): " + strings.Repeat("x", 73) + "\n", why: tooLong},
		{name: "free text", msg: "update stuff\n", why: notConventional},
		{name: "unknown type", msg: "ci: gitea actions\n", why: notConventional},
		{name: "capital type", msg: "Fix(gate): тип с заглавной" + review, why: notConventional},
		{name: "capital scope", msg: "docs(README): scope с заглавной\n", why: notConventional},
		{name: "empty scope", msg: "fix(): пустой scope" + review, why: notConventional},
		{name: "no space after colon", msg: "fix(gate):без пробела" + review, why: notConventional},
		{name: "empty subject", msg: "fix(gate): " + review, why: notConventional},
		{name: "merge", msg: "Merge branch 'main' into feature\n", why: notConventional},
		{name: "empty message", msg: "\n", why: notConventional},
	} {
		for _, locale := range []string{"C", "C.UTF-8"} {
			t.Run(locale+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				file := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
				require.NoError(t, os.WriteFile(file, []byte(tc.msg), 0o600))
				cmd := exec.CommandContext(t.Context(), "bash", hook, file)
				cmd.Env = append(os.Environ(), "LC_ALL="+locale)
				out, err := cmd.CombinedOutput()
				if tc.why == "" {
					require.NoError(t, err, "commit-msg refuses a valid message: %s", out)
					return
				}
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "commit-msg accepts %q: %s", tc.msg, out)
				require.Equal(t, 1, exit.ExitCode(), "commit-msg must refuse, not break: %s", out)
				require.Contains(t, string(out), tc.why, "commit-msg refuses %q for another reason", tc.msg)
			})
		}
	}
}
