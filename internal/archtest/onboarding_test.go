package archtest

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// identityPaths — пути доменов идентичности из .template/identity: проект с внешней идентичностью их
// удаляет (prune).
func identityPaths(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".template", "identity"))
	require.NoError(t, err)
	var paths []string
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			paths = append(paths, line)
		}
	}
	require.NotEmpty(t, paths, ".template/identity lists no paths")
	return paths
}

// identityExternal — docs/stack/auth.md выбрал внешнюю идентичность: домены идентичности удалены по решению.
func identityExternal(t *testing.T) bool {
	t.Helper()
	f, err := os.Open(filepath.Join(repoRoot(t), "docs", "stack", "auth.md"))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	scanner := bufio.NewScanner(f)
	for fences := 0; fences < 2 && scanner.Scan(); {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			fences++
		} else if value, ok := strings.CutPrefix(line, "identity:"); ok && fences == 1 {
			return strings.TrimSpace(value) == "external"
		}
	}
	require.NoError(t, scanner.Err())
	return false
}

// TestIdentityPaths: строка .template/identity без файлов — домен идентичности, который prune оставил бы.
// У проекта с внешней идентичностью путей нет — prune их удалил.
func TestIdentityPaths(t *testing.T) {
	t.Parallel()
	if identityExternal(t) {
		return
	}
	files := trackedFiles(t, repoRoot(t))
	for _, p := range identityPaths(t) {
		if !slices.ContainsFunc(files, func(f string) bool { return under(f, p) }) {
			t.Errorf(".template/identity: %q is not a tracked file or directory", p)
		}
	}
}

// TestPruneNamesIdentityDependents: файл вне доменов идентичности, который знает их пакеты, prune правит —
// процедура (references/prune.md скилла init-service) называет каждый такой файл, иначе после неё сборка
// упала бы на файле, которого она не знает.
func TestPruneNamesIdentityDependents(t *testing.T) {
	t.Parallel()
	prune, err := os.ReadFile(filepath.Join(repoRoot(t), ".claude", "skills", "init-service", "references", "prune.md"))
	require.NoError(t, err)
	bundle := identityPaths(t)
	inBundle := func(p string) bool { return slices.ContainsFunc(bundle, func(b string) bool { return under(p, b) }) }
	for _, f := range goFiles(t) {
		if inBundle(f.path) || strings.Contains(string(prune), "`"+f.path+"`") {
			continue
		}
		for _, imp := range f.imports {
			if pkg, ok := strings.CutPrefix(imp, module+"/"); ok && inBundle(pkg) {
				t.Errorf("%s imports %s of the identity domains: name it in the prune procedure (.claude/skills/init-service/references/prune.md)", f.path, imp)
				break
			}
		}
	}
}

// canonicalOrigin — origin канонического репо шаблона.
var canonicalOrigin = regexp.MustCompile(`[:/]stit-core/go-backend-template(\.git)?/?$`)

// TestCanonicalTemplate: в каноническом репо шаблона (origin — stit-core/go-backend-template) module path —
// шаблона, внешней идентичности нет: de-template и prune разрушили бы шаблон, а с ними гейт красный и
// pre-push их не отправит. У форка origin свой — ему можно.
func TestCanonicalTemplate(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	cmd := exec.CommandContext(t.Context(), "git", "config", "--get", "remote.origin.url")
	cmd.Dir = root
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return // origin нет — не канонический репо
	}
	require.NoError(t, err, "git config --get remote.origin.url")
	if !canonicalOrigin.MatchString(strings.TrimSpace(string(out))) {
		return
	}
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	mod := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindSubmatch(gomod)
	require.NotNil(t, mod, "go.mod has no module line")
	require.True(t, strings.HasSuffix(string(mod[1]), "/stit-core/go-backend-template"),
		"origin is the canonical template, but go.mod says module %s: de-template runs on a fork, never on the template", mod[1])
	require.False(t, identityExternal(t), "origin is the canonical template, but docs/stack/auth.md chose external identity: prune runs on a fork, never on the template")
}

// TestStackValidator: шаблоны stack как есть — заглушки, de-template развернул бы из них пустой фундамент:
// со status: stable валидатор отвергает каждую секцию и невыбранную идентичность; те же артефакты с
// содержимым вместо плейсхолдеров проходят и --stable (вход de-template).
func TestStackValidator(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "tools", "validate-stack-doc.sh")
	require.FileExists(t, script) // скрипт читает bash: без этого кеш go test не видит его правок
	templates, err := filepath.Glob(filepath.Join(root, "docs", "stack", "_templates", "*.md"))
	require.NoError(t, err)
	require.Len(t, templates, 8, "eight stack kinds")
	stub, filled := t.TempDir(), t.TempDir()
	sections := 0
	for _, p := range templates {
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		text := strings.Replace(string(data), "\nstatus: draft\n", "\nstatus: stable\n", 1)
		sections += strings.Count(text, "\n## ")
		writeFiles(t, stub, map[string]string{filepath.Base(p): text})
		writeFiles(t, filled, map[string]string{filepath.Base(p): fillStackTemplate(text)})
	}

	out, err := runIn(t, root, "bash", script, stub)
	require.Error(t, err, out)
	require.Equal(t, sections, strings.Count(out, "пуста (status=stable"), "every section of an unchanged template is empty:\n%s", out)
	require.Contains(t, out, "auth.md: frontmatter.identity")

	out, err = runIn(t, root, "bash", script, "--stable", filled)
	require.NoError(t, err, out)
}

// fillStackTemplate — шаблон стека с содержимым: плейсхолдер (от строки с `<` до строки, которая кончается
// `>`) — строка решения, идентичность — своя.
func fillStackTemplate(text string) string {
	var b strings.Builder
	inside := false
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		switch {
		case inside:
			inside = !strings.HasSuffix(trimmed, ">")
		case strings.HasPrefix(trimmed, "<"):
			inside = !strings.HasSuffix(trimmed, ">")
			b.WriteString("Решение архитектора.\n")
		case strings.HasPrefix(trimmed, "identity:"):
			b.WriteString("identity: own\n")
		default:
			b.WriteString(line)
		}
	}
	return b.String()
}
