package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGuardBash: хук .claude/hooks/guard-bash.sh не пускает из сессии Claude удобные обходы git-хуков —
// флаги (и в кавычках, и через интерпретатор), алиасы и include конфига, снятие секции core,
// SKIP_PREPUSH и hooksPath, send-pack, запись в хуки и .git/config — и «читателями» (cp, sort, find,
// awk, sed, git --output), — и не мешает обычной работе. Случаи — testdata/guard-cases.txt.
func TestGuardBash(t *testing.T) {
	t.Parallel()
	guard := filepath.Join(repoRoot(t), ".claude", "hooks", "guard-bash.sh")
	require.FileExists(t, guard) // скрипт читает bash: без этого кеш go test не видит его правок
	data, err := os.ReadFile(filepath.Join("testdata", "guard-cases.txt"))
	require.NoError(t, err)
	cases := 0
	for n, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		verdict, command, ok := strings.Cut(line, " ")
		require.True(t, ok && (verdict == "A" || verdict == "B"), `guard-cases.txt:%d: want "A <command>" or "B <command>", got %q`, n+1, line)
		cases++
		command = strings.NewReplacer("⏎", "\n", "⇥", "\t").Replace(command)
		t.Run(line, func(t *testing.T) {
			t.Parallel()
			var input bytes.Buffer
			enc := json.NewEncoder(&input)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": command}}))
			cmd := exec.CommandContext(t.Context(), "bash", guard)
			cmd.Stdin = &input
			out, err := cmd.CombinedOutput()
			if verdict == "A" {
				require.NoError(t, err, "guard-cases.txt:%d: the guard blocks ordinary work: %s", n+1, out)
				return
			}
			var exit *exec.ExitError
			require.True(t, errors.As(err, &exit) && exit.ExitCode() == 2,
				"guard-cases.txt:%d: the guard lets a bypass of the git hooks through (err %v): %s", n+1, err, out)
		})
	}
	require.GreaterOrEqual(t, cases, 550, "guard-cases.txt: the list is cut")
}

// guardCommand — команда хука guard-bash в .claude/settings.json.
const guardCommand = `bash "$CLAUDE_PROJECT_DIR/.claude/hooks/guard-bash.sh"`

// barrierDeny — запреты .claude/settings.json: снять любой — ослабить гейт (CLAUDE.md § Сначала спросить).
var barrierDeny = []string{
	"Bash(*--no-veri*)", "Bash(*--no-gpg-sign*)", "Bash(git push --force*)", "Bash(git push -f*)",
	"Bash(git reset --hard*)", "Bash(git checkout -- *)", "Bash(git clean -f*)", "Bash(rm -rf *)",
	"Read(**/*.pem)", "Read(**/*.key)", "Read(**/.env)", "Read(**/.env.local)",
	"Edit(**/*.pem)", "Edit(**/*.key)", "Edit(**/infra/postgres/db/**)", "Edit(**/mocks/*.go)",
	"Edit(go.sum)", "Edit(**/.claude/projects/**/memory/**)",
}

// guardFields — поля обработчика guard, которые барьер не гасят. Остальные гасят или обходят его (Claude Code,
// hooks § Common fields и Command hook fields): if сужает вызовы, async и asyncRewake не ждут отказа, once
// снимает хук после первого раза, args запускает command без оболочки, короткий timeout обрывает guard.
var guardFields = map[string]bool{"type": true, "command": true, "shell": true, "statusMessage": true, "onFailure": true}

// guardHandler — обработчик запускает guard как есть: команда оболочки bash без полей, которые его гасят.
func guardHandler(handler map[string]json.RawMessage) bool {
	var typ, command, shell string
	for key, value := range handler {
		var s *string
		switch key {
		case "type":
			s = &typ
		case "command":
			s = &command
		case "shell":
			s = &shell
		}
		if !guardFields[key] || s != nil && json.Unmarshal(value, s) != nil {
			return false
		}
	}
	return typ == "command" && command == guardCommand && (shell == "" || shell == "bash")
}

// TestGuardWired: барьер от случайного обхода держат хук guard-bash и deny .claude/settings.json, пока они
// подключены. Хук, отвязанный от Bash, подменённый, выключенный (disableAllHooks) или обезвреженный полем
// обработчика, и снятый запрет молчат: TestGuardBash проверяет скрипт, а не его подключение.
func TestGuardWired(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".claude", "settings.json"))
	require.NoError(t, err)
	var settings struct {
		DisableAllHooks bool
		Permissions     struct{ Deny []string }
		Hooks           struct {
			PreToolUse []struct {
				Matcher string
				Hooks   []map[string]json.RawMessage
			}
		}
	}
	require.NoError(t, json.Unmarshal(data, &settings))
	require.False(t, settings.DisableAllHooks, ".claude/settings.json: disableAllHooks switches guard-bash off")
	wired := false
	for _, entry := range settings.Hooks.PreToolUse {
		// Matcher — имя инструмента или регулярное выражение; пусто и «*» — любой инструмент.
		bash := entry.Matcher == "" || entry.Matcher == "*"
		if !bash {
			re, err := regexp.Compile("^(?:" + entry.Matcher + ")$")
			require.NoError(t, err, ".claude/settings.json: matcher %q", entry.Matcher)
			bash = re.MatchString("Bash")
		}
		for _, h := range entry.Hooks {
			wired = wired || bash && guardHandler(h)
		}
	}
	require.True(t, wired, ".claude/settings.json: no PreToolUse hook of Bash runs %s as a plain bash command (fields if, async, once, args, timeout disarm it) — the guard is off", guardCommand)
	for _, rule := range barrierDeny {
		require.Contains(t, settings.Permissions.Deny, rule, ".claude/settings.json: deny lost %s — that weakens the gate (CLAUDE.md § Сначала спросить)", rule)
	}
}
