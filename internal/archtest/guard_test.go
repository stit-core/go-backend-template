package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGuardBash: хук .claude/hooks/guard-bash.sh не пускает из сессии Claude удобные обходы git-хуков —
// флаги (и в кавычках, и через интерпретатор), алиасы и include конфига, снятие секции core,
// SKIP_PREPUSH и hooksPath, send-pack, правку и chmod хуков и .git/config — и не мешает обычной работе.
func TestGuardBash(t *testing.T) {
	t.Parallel()
	guard := filepath.Join(repoRoot(t), ".claude", "hooks", "guard-bash.sh")
	require.FileExists(t, guard) // скрипт читает bash: без этого кеш go test не видит его правок
	for _, tc := range []struct {
		cmd     string
		blocked bool
	}{
		{"git commit -n -m x", true},
		{"git commit -an -m x", true},
		{`git commit "-n" -m x`, true},
		{"git commit '-an' -m x", true},
		{"git push --no-verify", true},
		{`git push "--no-verify" origin main`, true},
		{"git push --no-veri origin main", true},
		{"git -c alias.c='commit -n' c -m x", true},
		{"git config alias.p 'push --no-verify'", true},
		{"SKIP_PREPUSH=1 git push", true},
		{"git -c core.hooksPath=/dev/null push", true},
		{"chmod -x scripts/git-hooks/pre-push", true},
		{"cd scripts/git-hooks && chmod -x pre-push", true},
		{"cp /bin/true scripts/git-hooks/pre-commit", true},
		{"echo > scripts/git-hooks/commit-msg", true},
		{"sed -i 's/make check/true/' scripts/git-hooks/pre-push", true},
		{"git checkout HEAD~1 -- scripts/git-hooks/pre-push", true},
		{"mv scripts/git-hooks/pre-push /tmp/", true},
		{`"git" commit -n -m x`, true},
		{"'git' commit -n -m x", true},
		{`\git commit -n -m x`, true},
		{`g\it commit -n -m x`, true},
		{"$'git' commit -n -m x", true},
		{"git commit --no-ver'i'fy -m x", true},
		{"bash -c 'git commit -n -m x'", true},
		{`sh -c "git commit -n -m x"`, true},
		{`eval "git commit -n -m x"`, true},
		{"printf 'git commit -n -m x' | bash", true},
		{"bash <<'EOF'\ngit commit -n -m x\nEOF", true},
		{"bash <<'EOF'\nchmod -x scripts/git-hooks/pre-push\nEOF", true},
		{"sed -i '/hooksPath/d' .git/config", true},
		{`printf '[core]\n\thooksPath = /dev/null\n' >> .git/config`, true},
		{"echo x >> .git/config", true},
		{"git -c include.path=/tmp/x.cfg push", true},
		{"git config --global include.path /tmp/x.cfg", true},
		{`git -c core.hooks"P"ath=/dev/null push`, true},
		{"git config --remove-section core", true},
		{"git config --local --rename-section CORE corex", true},
		{"git config remove-section core", true},
		{"git send-pack origin HEAD:refs/heads/main", true},
		{"echo 'export SKIP_PREPUSH=1' >> ~/.bashrc", true},
		{`git commit -m "fix: -n and --no-verify in a message"`, false},
		{"python3 - <<'EOF'\nprint('bash -c, SKIP_PREPUSH and hooksPath here are data')\nEOF", false},
		{"git commit -q -F - <<'EOF'\nfix: x\n\ngit commit -n\nEOF", false},
		{"git push origin main", false},
		{"git log --oneline -5", false},
		{"cat scripts/git-hooks/pre-push", false},
		{"bash scripts/git-hooks/pre-push", false},
		{"bash -n scripts/git-hooks/pre-commit", false},
		{"git diff scripts/git-hooks/pre-push", false},
		{"grep -rn alias. README.md", false},
		{"git log --grep 'alias.'", false},
		{"find . -name pre-push", false},
		{"awk 'NR<5' scripts/git-hooks/pre-push", false},
		{"sort scripts/git-hooks/commit-msg", false},
		{"cp scripts/git-hooks/pre-push /tmp/pre-push.copy", false},
		{"cat .git/config", false},
		{"git config --remove-section remote.old", false},
		{`git commit -m "docs: git config --remove-section core"`, false},
		{"bash -c 'make check'", false},
		{"make check", false},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			t.Parallel()
			var input bytes.Buffer
			enc := json.NewEncoder(&input)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": tc.cmd}}))
			cmd := exec.CommandContext(t.Context(), "bash", guard)
			cmd.Stdin = &input
			out, err := cmd.CombinedOutput()
			if !tc.blocked {
				require.NoError(t, err, "the guard blocks ordinary work: %s", out)
				return
			}
			var exit *exec.ExitError
			require.True(t, errors.As(err, &exit) && exit.ExitCode() == 2, "the guard lets a bypass of the git hooks through (err %v): %s", err, out)
		})
	}
}
