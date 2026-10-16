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
// коммитящего (под LC_ALL=C русская буква — два байта); у feat, fix и переноса обвязки chore(template) в теле —
// строка Review: (CLAUDE.md § Готово). Отказ переносу чужого коммита (cherry-pick --continue, git commit посреди
// rebase) говорит, как его дописать и закончить перенос; законченный rebase — не перенос. Хук идёт в своём репо, а не
// в репо разработчика: его перенос и следы законченного не меняют вердикт.
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
	clean := t.TempDir()
	git(t, clean, "init", "-q")
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
		{name: "template transfer", msg: "chore(template): обвязка template/v1.2.0\n\nотклонено: c/auth — свой вход" + review},
		{name: "chore of another scope", msg: "chore(deps): bump sqlc\n"},
		{name: "template transfer without review", msg: "chore(template): обвязка template/v1.2.0\n\nперенесено: c/auth\n", why: noReview},
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
				cmd.Dir = clean
				cmd.Env = callerEnv("PATH="+os.Getenv("PATH"), "LC_ALL="+locale)
				out, err := cmd.CombinedOutput()
				if tc.why == "" {
					require.NoError(t, err, "commit-msg refuses a valid message: %s", out)
					return
				}
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "commit-msg accepts %q: %s", tc.msg, out)
				require.Equal(t, 1, exit.ExitCode(), "commit-msg must refuse, not break: %s", out)
				require.Contains(t, string(out), tc.why, "commit-msg refuses %q for another reason", tc.msg)
				require.NotContains(t, string(out), "Перенос коммита", "no carry, no hint")
			})
		}
	}

	// Перенос коммита без следа ревью настоящими cherry-pick и rebase с конфликтом: отказ объясняет, как дописать
	// сообщение и закончить перенос — одиночный cherry-pick заканчивает git commit, rebase — только ещё
	// git rebase --continue; без переноса подсказки нет (таблица выше гоняет хук вне переноса).
	t.Run("carry", func(t *testing.T) {
		t.Parallel()
		hooks, repo := t.TempDir(), t.TempDir()
		writeExecutable(t, filepath.Join(hooks, "commit-msg"), files["scripts/git-hooks/commit-msg"])
		git(t, repo, "init", "-q", "-b", "main")
		commit := func(file, content, msg string) {
			writeFiles(t, repo, map[string]string{file: content})
			git(t, repo, "add", file)
			git(t, repo, "commit", "-qm", msg)
		}
		commit("f.txt", "a\n", "chore: init")
		git(t, repo, "checkout", "-q", "-b", "side1")
		commit("f.txt", "side1\n", "feat(x): старый коммит без следа")
		git(t, repo, "checkout", "-q", "-b", "side2", "main")
		commit("g.txt", "g\n", "chore: отдельный файл")
		commit("f.txt", "side2\n", "feat(y): правка f без следа")
		git(t, repo, "checkout", "-q", "main")
		commit("f.txt", "main\n", "chore: main")
		git(t, repo, "config", "core.hooksPath", hooks)
		git(t, repo, "config", "core.editor", "true")
		message := filepath.Join(t.TempDir(), "msg")
		require.NoError(t, os.WriteFile(message, []byte("feat(x): перенос\n\nReview: archtest — перенос\n"), 0o600))
		conflict := func(args ...string) {
			out, err := runIn(t, repo, "git", args...)
			require.Error(t, err, "a conflict stops git %v: %s", args, out)
			writeFiles(t, repo, map[string]string{"f.txt": strings.Join(args, " ") + "\n"})
			git(t, repo, "add", "f.txt")
		}

		conflict("cherry-pick", "side1")
		out, err := runIn(t, repo, "git", "cherry-pick", "--continue")
		require.Error(t, err, out)
		require.Contains(t, out, "если в цепочке есть ещё коммиты — git cherry-pick --continue")
		git(t, repo, "commit", "-q", "-F", message)
		_, err = runIn(t, repo, "git", "rev-parse", "-q", "--verify", "CHERRY_PICK_HEAD")
		require.Error(t, err, "git commit finishes a single cherry-pick")

		// git rebase --continue хук не зовёт: отказ с подсказкой получает git commit посреди переноса.
		git(t, repo, "checkout", "-q", "side2")
		conflict("rebase", "main")
		out, err = runIn(t, repo, "git", "commit", "--no-edit")
		require.Error(t, err, out)
		require.Contains(t, out, "затем всегда git rebase --continue")
		git(t, repo, "commit", "-q", "-F", message)
		rebase := strings.TrimSpace(git(t, repo, "rev-parse", "--git-path", "rebase-merge"))
		require.DirExists(t, filepath.Join(repo, rebase), "git commit does not finish a rebase")
		git(t, repo, "rebase", "--continue")
		require.NoDirExists(t, filepath.Join(repo, rebase))
		require.Equal(t, git(t, repo, "rev-parse", "HEAD"), git(t, repo, "rev-parse", "side2"))

		// Законченный rebase с конфликтом git оставляет REBASE_HEAD: отказ после него — без подсказки переноса.
		writeFiles(t, repo, map[string]string{"h.txt": "h\n"})
		git(t, repo, "add", "h.txt")
		out, err = runIn(t, repo, "git", "commit", "-qm", "feat(z): после переноса")
		require.Error(t, err, out)
		require.Contains(t, out, noReview)
		require.NotContains(t, out, "Перенос коммита", "a finished rebase is no carry")
	})
}
