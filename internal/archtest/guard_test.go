package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	require.GreaterOrEqual(t, cases, 400, "guard-cases.txt: the list is cut")
}
