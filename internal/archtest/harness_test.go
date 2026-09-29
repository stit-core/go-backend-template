package archtest

import (
	"go/ast"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
var proseAllowlist = allowlist{
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
// .claude/rules и skills) существуют — иначе агент идёт по мёртвой ссылке (HAR-11).
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
	topDirs := repoTopDirs(t, root)

	var dead []string
	for _, doc := range proseFiles(t, root) {
		data, err := os.ReadFile(filepath.Join(root, doc))
		require.NoError(t, err)
		for _, span := range codeSpan.FindAllString(string(data), -1) {
			for _, m := range makeCall.FindAllStringSubmatch(span, -1) {
				if !slices.Contains(targets, m[1]) && !slices.Contains(hubTargets, m[1]) {
					dead = append(dead, doc+": make "+m[1])
				}
			}
			for _, word := range strings.Fields(strings.Trim(span, "`")) {
				ref := strings.Trim(word, `"'.,;:()`)
				ref, _, _ = strings.Cut(ref, ":")
				if placeholder.MatchString(ref) || !slices.Contains(topDirs, strings.Split(ref, "/")[0]) || !strings.Contains(ref, "/") {
					continue
				}
				if !repoPathExists(root, goSymbol.ReplaceAllString(ref, "")) {
					dead = append(dead, doc+": "+ref)
				}
			}
		}
	}
	proseAllowlist.check(t, dead, "the prose points to a make target or a repo path that does not exist")
}

func repoPathExists(root, ref string) bool {
	if _, err := os.Stat(filepath.Join(root, ref)); err == nil {
		return true
	}
	// Артефакт стека docs/stack/<kind>.md появляется у форка — по шаблону _templates/<kind>.md.
	kind, ok := strings.CutPrefix(ref, "docs/stack/")
	_, err := os.Stat(filepath.Join(root, "docs/stack/_templates", kind))
	return ok && !strings.Contains(kind, "/") && err == nil
}

// repoTopDirs — каталоги корня репо, на которые ссылается проза: без скрытых (кроме .claude) и bin.
func repoTopDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "bin" && (!strings.HasPrefix(e.Name(), ".") || e.Name() == ".claude") {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

// workDocs — рабочие документы проекта: планы, память stack-deep-dive, чужие worktree. Они
// описывают и файлы, которых ещё нет, поэтому проверка прозы их не касается.
var workDocs = []string{".claude/plans", ".claude/skills/stack-deep-dive/state", ".claude/worktrees"}

// proseFiles — .md обвязки: без рабочих документов, артефактов стека docs/stack/<kind>.md и
// скрытых каталогов.
func proseFiles(t *testing.T, root string) []string {
	t.Helper()
	var docs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir() && (slices.Contains(workDocs, rel) ||
			rel != "." && rel != ".claude" && strings.HasPrefix(d.Name(), ".")):
			return filepath.SkipDir
		case d.IsDir() || path.Ext(rel) != ".md":
		case path.Dir(rel) == "docs/stack" && rel != "docs/stack/README.md":
		default:
			docs = append(docs, rel)
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	return docs
}
