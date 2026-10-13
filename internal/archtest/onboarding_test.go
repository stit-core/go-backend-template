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
	return identityPathsIn(t, repoRoot(t))
}

func identityPathsIn(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".template", "identity"))
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
	return identityChoice(t, repoRoot(t)) == "external"
}

// identityChoice — значение identity во frontmatter docs/stack/auth.md; "" — стек не спроектирован.
func identityChoice(t *testing.T, root string) string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "docs", "stack", "auth.md"))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	scanner := bufio.NewScanner(f)
	for fences := 0; fences < 2 && scanner.Scan(); {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			fences++
		} else if value, ok := strings.CutPrefix(line, "identity:"); ok && fences == 1 {
			return strings.TrimSpace(value)
		}
	}
	require.NoError(t, scanner.Err())
	return ""
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
// упала бы на файле, которого она не знает. При identity: own prune не будет — проверки нет.
func TestPruneNamesIdentityDependents(t *testing.T) {
	t.Parallel()
	for _, e := range unnamedIdentityDependents(t, repoRoot(t), goFiles(t)) {
		t.Error(e)
	}
}

// TestIdentityDependents: адаптер нового домена к user по port.md процедура prune называет там, где prune
// возможен, — в шаблоне и у форка без решения или с внешней идентичностью; при identity: own — нет.
func TestIdentityDependents(t *testing.T) {
	t.Parallel()
	adapter := goFile{
		path: "internal/domain/order/module/adapters.go", pkg: "internal/domain/order/module",
		imports: []string{"context", module + "/internal/domain/order", module + "/internal/domain/user"},
	}
	refusal := adapter.path + " imports " + module + "/internal/domain/user of the identity domains"
	for name, tc := range map[string]struct {
		identity string // identity во frontmatter docs/stack/auth.md; "" — артефакта нет
		prune    string // строка процедуры prune
		refused  bool
	}{
		"template":              {refused: true},
		"undecided stack":       {identity: "<own|external>", refused: true},
		"external identity":     {identity: "external", refused: true},
		"own identity":          {identity: "own"},
		"named in the prune":    {prune: "- `" + adapter.path + "` — порт order к user."},
		"another file in prune": {prune: "- `internal/domain/order/module/module.go`", refused: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			files := map[string]string{
				".template/identity":                              "# домены идентичности\ninternal/domain/user\n",
				".claude/skills/init-service/references/prune.md": "# prune\n\n" + tc.prune + "\n",
			}
			if tc.identity != "" {
				files["docs/stack/auth.md"] = "---\nid: stack.auth\nkind: auth\nname: auth\nstatus: stable\nidentity: " + tc.identity + "\n---\n"
			}
			writeFiles(t, root, files)
			found := unnamedIdentityDependents(t, root, []goFile{adapter})
			if !tc.refused {
				require.Empty(t, found)
				return
			}
			require.Len(t, found, 1)
			require.Contains(t, found[0], refusal)
		})
	}
}

// unnamedIdentityDependents — файлы вне доменов идентичности, которые знают их пакеты, а процедура prune
// их не называет. Проекту со своей идентичностью prune не нужен: его домены берут user, как любой другой.
func unnamedIdentityDependents(t *testing.T, root string, files []goFile) []string {
	t.Helper()
	if identityChoice(t, root) == "own" {
		return nil
	}
	prune, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "init-service", "references", "prune.md"))
	require.NoError(t, err)
	bundle := identityPathsIn(t, root)
	inBundle := func(p string) bool { return slices.ContainsFunc(bundle, func(b string) bool { return under(p, b) }) }
	var found []string
	for _, f := range files {
		if inBundle(f.path) || strings.Contains(string(prune), "`"+f.path+"`") {
			continue
		}
		for _, imp := range f.imports {
			if pkg, ok := strings.CutPrefix(imp, module+"/"); ok && inBundle(pkg) {
				found = append(found, f.path+" imports "+imp+" of the identity domains: name it in the prune procedure (.claude/skills/init-service/references/prune.md)")
				break
			}
		}
	}
	return found
}

// canonicalOrigin — origin канонического репо шаблона.
var canonicalOrigin = regexp.MustCompile(`(?i)[:/]stit-core/go-backend-template(\.git)?/?$`)

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
// со status: stable валидатор отвергает каждую секцию и невыбранную идентичность. Заглушка вместо решения
// (TBD, TODO…, прочерк, многоточие, таблица без строк, одни подзаголовки) — тоже пустая секция; артефакты
// с решениями вместо плейсхолдеров проходят и --stable (вход de-template).
func TestStackValidator(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "tools", "validate-stack-doc.sh")
	require.FileExists(t, script) // скрипт читает bash: без этого кеш go test не видит его правок
	templates, err := filepath.Glob(filepath.Join(root, "docs", "stack", "_templates", "*.md"))
	require.NoError(t, err)
	require.Len(t, templates, 8, "eight stack kinds")
	texts := map[string]string{}
	sections := 0
	for _, p := range templates {
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		text := strings.Replace(string(data), "\nstatus: draft\n", "\nstatus: stable\n", 1)
		sections += strings.Count(text, "\n## ")
		texts[filepath.Base(p)] = text
	}
	stack := func(t *testing.T, fill func(string) string) string {
		t.Helper()
		dir := t.TempDir()
		for name, text := range texts {
			writeFiles(t, dir, map[string]string{name: fill(text)})
		}
		return dir
	}

	out, err := runIn(t, root, "bash", script, stack(t, func(text string) string { return text }))
	require.Error(t, err, out)
	require.Equal(t, sections, strings.Count(out, "пуста (status=stable"), "every section of an unchanged template is empty:\n%s", out)
	require.Contains(t, out, "auth.md: frontmatter.identity")

	for _, tc := range []struct {
		fill   string // строка вместо каждого плейсхолдера
		stable bool
	}{
		{fill: "Решение архитектора.", stable: true},
		{fill: "— не публикует: событий у сервиса нет", stable: true},
		{fill: "- `cmd/api` — HTTP и outbox", stable: true},
		{fill: "| Env | Тип |\n|---|---|\n| `PG_DSN` | строка |", stable: true},
		{fill: "TBD"},
		{fill: "_TBD_"},
		{fill: "- TBD."},
		{fill: "**TODO**: заполнить позже"},
		{fill: "—"},
		{fill: "..."},
		{fill: "- <заполнить>"},
		{fill: "| Env | Тип |\n|---|---|\n"}, // пустая строка: два плейсхолдера подряд — две таблицы, а не одна
		{fill: "### Подсекция"},
	} {
		t.Run(tc.fill, func(t *testing.T) {
			t.Parallel()
			out, err := runIn(t, root, "bash", script, "--stable", stack(t, func(text string) string { return fillStackTemplate(text, tc.fill) }))
			if tc.stable {
				require.NoError(t, err, "a decision is content: %q\n%s", tc.fill, out)
				return
			}
			require.Error(t, err, out)
			require.Equal(t, sections, strings.Count(out, "пуста (status=stable"), "a stub is no decision: %q\n%s", tc.fill, out)
		})
	}
}

// fillStackTemplate — шаблон стека, где каждый плейсхолдер (от строки с `<` до строки, которая кончается `>`)
// заменён строкой fill, а идентичность — своя.
func fillStackTemplate(text, fill string) string {
	var b strings.Builder
	inside := false
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		switch {
		case inside:
			inside = !strings.HasSuffix(trimmed, ">")
		case strings.HasPrefix(trimmed, "<"):
			inside = !strings.HasSuffix(trimmed, ">")
			b.WriteString(fill + "\n")
		case strings.HasPrefix(trimmed, "identity:"):
			b.WriteString("identity: own\n")
		default:
			b.WriteString(line)
		}
	}
	return b.String()
}
