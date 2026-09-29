package archtest

import (
	"errors"
	"go/ast"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/testkit/allowlist"
)

// TestDomainsAreWired: домен, не подключённый к сервису, молчит тихо — граф валиден и тесты
// зелёные, а ручки отвечают 404, подписчик не слушает, outbox-таблицы нет.
func TestDomainsAreWired(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := goFiles(t)
	for _, d := range domains(t) {
		pkg := "internal/domain/" + d
		for _, w := range []struct{ sub, in, name, why string }{
			{"module", "cmd/api", "Module", "its fx module is not in domainModules of cmd/api"},
			{"delivery/http", "cmd/api", "Register", "its HTTP operations are not registered by registerHTTP (cmd/api/http.go)"},
			{"delivery/nats", pkg + "/module", "Register", "its NATS subscriber is not invoked by its module (fx.Invoke(Register))"},
			{"infra/outbox", "cmd/migrate", "Migrations", "its outbox table is not among the migration sources of cmd/migrate"},
		} {
			if _, err := os.Stat(filepath.Join(root, pkg, w.sub)); err != nil {
				continue
			}
			if !refers(t, files, w.in, module+"/"+pkg+"/"+w.sub, w.name) {
				t.Errorf("domain %s: %s", d, w.why)
			}
		}
	}
}

// refers — не-тестовый код под dir обращается к pkgPath.name.
func refers(t *testing.T, files []goFile, dir, pkgPath, name string) bool {
	t.Helper()
	for _, f := range files {
		if f.test || !under(f.pkg, dir) || !slices.Contains(f.imports, pkgPath) {
			continue
		}
		file := parseGo(t, f.path)
		byName := importsByName(file)
		found := false
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
				id, ok := sel.X.(*ast.Ident)
				found = found || ok && byName[id.Name] == pkgPath
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// proseAllowlist — мёртвые ссылки прозы в текущем тексте.
var proseAllowlist = allowlist.List{
	"docs/reference/domain-mapping.md: internal/platform/shared/":          "ADP-03",
	"docs/reference/domain-mapping.md: internal/platform/shared/errors.go": "ADP-03",
}

// hubTargets — цели make хаба <org>/infra: проза называет их, чтобы сказать, что здесь их нет.
var hubTargets = []string{"bump", "ps", "ship"}

var (
	codeSpan    = regexp.MustCompile("(?s)```.*?```|`[^`\n]+`")
	makeCall    = regexp.MustCompile(`\bmake\s+([a-z][a-z0-9-]*)`)
	makeTarget  = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*):`)
	goSymbol    = regexp.MustCompile(`\.[A-Z]\w*$`)
	placeholder = regexp.MustCompile(`[<>{}*$\[\]|…]|NNNN|\.\.\.`)
)

// TestProseReferencesExist: make-цели и пути репо в коде прозы (CLAUDE.md, README, docs/,
// .claude/rules и skills) есть в репо — иначе агент идёт по мёртвой ссылке. Репо — то,
// что отслеживает git: локальное (test/.venv, неотслеживаемый файл) есть не в каждом клоне.
// Плейсхолдеры (<d>, NNNN, *) — не ссылки; артефакт docs/stack/<kind>.md — по его шаблону.
func TestProseReferencesExist(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err)
	var targets []string
	for _, m := range makeTarget.FindAllStringSubmatch(string(makefile), -1) {
		targets = append(targets, m[1])
	}
	files := trackedFiles(t, root)
	repo := map[string]bool{} // файлы и каталоги (с "/" на конце) репо
	for _, f := range files {
		repo[f] = true
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			repo[d+"/"] = true
		}
	}

	var dead []string
	for _, doc := range proseFiles(files) {
		data, err := os.ReadFile(filepath.Join(root, doc))
		require.NoError(t, err)
		for _, span := range codeSpan.FindAllString(string(data), -1) {
			for _, m := range makeCall.FindAllStringSubmatch(span, -1) {
				if !slices.Contains(targets, m[1]) && !slices.Contains(hubTargets, m[1]) {
					dead = append(dead, doc+": make "+m[1])
				}
			}
			for _, word := range strings.Fields(strings.Trim(span, "`")) {
				ref := strings.TrimRight(strings.TrimLeft(word, `"'(`), `"'.,;:()`) // точка в начале — .claude
				ref, _, _ = strings.Cut(ref, ":")
				top, _, isPath := strings.Cut(ref, "/")
				if placeholder.MatchString(ref) || !isPath || !repo[top+"/"] {
					continue
				}
				if !inRepo(repo, goSymbol.ReplaceAllString(ref, "")) && !gitIgnored(t, root, ref) {
					dead = append(dead, doc+": "+ref)
				}
			}
		}
	}
	proseAllowlist.Check(t, dead, "the prose points to a missing make target or to a path git does not track")
}

func inRepo(repo map[string]bool, ref string) bool {
	ref = strings.TrimSuffix(ref, "/")
	if repo[ref] || repo[ref+"/"] {
		return true
	}
	// Артефакт стека docs/stack/<kind>.md появляется у форка — по шаблону _templates/<kind>.md.
	kind, ok := strings.CutPrefix(ref, "docs/stack/")
	return ok && !strings.Contains(kind, "/") && repo["docs/stack/_templates/"+kind]
}

// trackedFiles — файлы индекса git, которые есть в рабочем дереве: удалённый файл — уже не ссылка.
func trackedFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "git ls-files")
	var files []string
	for _, f := range strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 }) {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			files = append(files, f)
		}
	}
	require.NotEmpty(t, files)
	return files
}

// gitIgnored — путь исключён .gitignore: локальный, а не ссылка на репо. "/" на конце — чтобы git
// узнал игнорируемый каталог, которого нет на диске (test/.venv в свежем клоне).
func gitIgnored(t *testing.T, root, ref string) bool {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "check-ignore", "-q", strings.TrimSuffix(ref, "/")+"/")
	cmd.Dir = root
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false
	}
	require.NoError(t, err, "git check-ignore %s", ref)
	return true
}

// workDocs — рабочие документы проекта: планы и память stack-deep-dive. Они описывают и файлы,
// которых ещё нет, поэтому проверка прозы их не касается.
var workDocs = []string{".claude/plans", ".claude/skills/stack-deep-dive/state"}

// proseFiles — .md репо без рабочих документов и артефактов стека docs/stack/<kind>.md.
func proseFiles(files []string) []string {
	var docs []string
	for _, f := range files {
		switch {
		case path.Ext(f) != ".md", slices.ContainsFunc(workDocs, func(d string) bool { return under(f, d) }):
		case path.Dir(f) == "docs/stack" && f != "docs/stack/README.md":
		default:
			docs = append(docs, f)
		}
	}
	return docs
}
