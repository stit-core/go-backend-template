package archtest

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
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
// зелёные, а ручки отвечают 404, подписчик не слушает. Outbox-таблицы домена в cmd/migrate
// сверяет TestBoot (cmd/api) с графом сервиса.
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
var proseAllowlist = allowlist.List{}

// hubTargets — цели make хаба <org>/infra: проза называет их, чтобы сказать, что здесь их нет.
var hubTargets = []string{"bump", "ps", "ship"}

// notRepoSkills — /имена прозы, которые не скиллы репо: встроенные команды Claude Code и скилл
// соседнего <org>/strategy, куда проза отправляет проектировать домены.
var notRepoSkills = []string{"clear", "code-review", "security-review", "domain-deep-dive"}

var (
	codeSpan    = regexp.MustCompile("(?s)```.*?```|`[^`\n]+`")
	makeCall    = regexp.MustCompile(`\bmake\s+([a-z][a-z0-9-]*)`)
	makeTarget  = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*):`)
	goSymbol    = regexp.MustCompile(`\.[A-Z]\w*$`)
	placeholder = regexp.MustCompile(`[<>{}*$\[\]|…]|NNNN|\.\.\.`)
	slashName   = regexp.MustCompile(`^/([a-z][a-z0-9-]*)$`)
	kebabName   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// fileName — файл без пути (`Makefile`, `.env.example`, `errors.go`): есть в репо под этим именем.
	fileName = regexp.MustCompile(`^(Makefile|LICENSE|Dockerfile(\.\w+)?|\.?\w[\w.-]*\.(md|go|sql|ya?ml|json|sh|py|toml|txt|mod|sum|example))$`)
	// namedBy — слово перед `имя`: rule или правило — .claude/rules/<имя>.md, skill или скилл — скилл.
	namedBy = regexp.MustCompile(`(?i)(^|\P{L})(rule|правил\p{L}*|skill|скилл\p{L}*)\s+$`)
)

// TestProseReferencesExist: в коде прозы (CLAUDE.md, README, docs/, .claude/rules и skills) есть в репо
// make-цели, пути и файлы без пути, скиллы (`/имя`, skill `имя`) и правила (rule `имя`) — иначе агент
// идёт по мёртвой ссылке. Репо — то, что отслеживает git: локальное (test/.venv, неотслеживаемый файл)
// есть не в каждом клоне. Плейсхолдеры (<d>, NNNN, *) — не ссылки; артефакт docs/stack/<kind>.md — по
// его шаблону; /имя, которое не скилл, — путь контракта openapi.json или встроенная команда.
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
	repo := map[string]bool{}  // файлы и каталоги (с "/" на конце) репо
	names := map[string]bool{} // имена файлов репо без пути
	for _, f := range files {
		repo[f] = true
		names[path.Base(f)] = true
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			repo[d+"/"] = true
		}
	}
	api := openAPIPaths(t, root)
	skill := func(name string) bool {
		return repo[".claude/skills/"+name+"/SKILL.md"] || slices.Contains(notRepoSkills, name)
	}

	var dead []string
	for _, doc := range proseFiles(files) {
		data, err := os.ReadFile(filepath.Join(root, doc))
		require.NoError(t, err)
		text := string(data)
		for _, loc := range codeSpan.FindAllStringIndex(text, -1) {
			span := strings.Trim(text[loc[0]:loc[1]], "`")
			for _, m := range makeCall.FindAllStringSubmatch(span, -1) {
				if !slices.Contains(targets, m[1]) && !slices.Contains(hubTargets, m[1]) {
					dead = append(dead, doc+": make "+m[1])
				}
			}
			if m := namedBy.FindStringSubmatch(text[max(0, loc[0]-64):loc[0]]); m != nil && kebabName.MatchString(span) {
				rule := strings.EqualFold(m[2], "rule") || strings.HasPrefix(strings.ToLower(m[2]), "правил")
				if rule && !repo[".claude/rules/"+span+".md"] || !rule && !skill(span) {
					dead = append(dead, doc+": "+strings.ToLower(m[2])+" "+span)
				}
			}
			for _, word := range strings.Fields(span) {
				ref := strings.TrimRight(strings.TrimLeft(word, `"'(`), `"'.,;:()`) // точка в начале — .claude
				ref, _, _ = strings.Cut(ref, ":")
				if placeholder.MatchString(ref) {
					continue
				}
				if m := slashName.FindStringSubmatch(ref); m != nil {
					if !skill(m[1]) && !api[ref] {
						dead = append(dead, doc+": "+ref)
					}
					continue
				}
				top, _, isPath := strings.Cut(ref, "/")
				switch {
				case !isPath && fileName.MatchString(ref):
					if !names[ref] && !gitIgnored(t, root, ref) {
						dead = append(dead, doc+": "+ref)
					}
				case isPath && repo[top+"/"]:
					if !inRepo(repo, goSymbol.ReplaceAllString(ref, "")) && !gitIgnored(t, root, ref) {
						dead = append(dead, doc+": "+ref)
					}
				}
			}
		}
	}
	proseAllowlist.Check(t, dead, "the prose points to a missing make target, path, file, skill or rule")
}

// openAPIPaths — пути контракта API (openapi.json в корне репо).
func openAPIPaths(t *testing.T, root string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "openapi.json"))
	require.NoError(t, err)
	var spec struct{ Paths map[string]json.RawMessage }
	require.NoError(t, json.Unmarshal(data, &spec))
	require.NotEmpty(t, spec.Paths, "openapi.json has no paths")
	paths := map[string]bool{}
	for p := range spec.Paths {
		paths[p] = true
	}
	return paths
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
	// Неразрешённый конфликт идёт строкой на каждую стадию индекса — подряд, git ls-files сортирует пути.
	for _, f := range slices.Compact(strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 })) {
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

// TestNolintNamesEnabledLinters: //nolint выключенного линтера ничего не глушит, а nolintlint
// ловит неиспользуемую директиву только у включённого — мёртвая прошла бы молча.
func TestNolintNamesEnabledLinters(t *testing.T) {
	t.Parallel()
	enabled := golangciEnabled(t)
	for _, f := range goFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(repoRoot(t), f.path), nil, parser.ParseComments)
		require.NoError(t, err)
		for _, group := range file.Comments {
			for _, c := range group.List {
				for _, name := range nolintNames(c.Text) {
					if !enabled[name] {
						t.Errorf("%s:%d: //nolint:%s — the linter is off in .golangci.yml, the directive silences nothing", f.path, fset.Position(c.Pos()).Line, name)
					}
				}
			}
		}
	}
}

var nolintDirective = regexp.MustCompile(`^nolint( |:|$)`)

// nolintNames — линтеры директивы по разбору golangci-lint: ведущие «/» и пробелы, регистр и
// пробелы у имён не важны, после второго «//» — пояснение. nil — не директива или директива на
// все линтеры (её запрещает nolintlint).
func nolintNames(comment string) []string {
	text := strings.TrimLeft(comment, "/ ")
	if !nolintDirective.MatchString(text) || !strings.HasPrefix(text, "nolint:") {
		return nil
	}
	text, _, _ = strings.Cut(strings.TrimPrefix(text, "nolint:"), "//")
	var names []string
	for item := range strings.SplitSeq(text, ",") {
		switch name := strings.ToLower(strings.TrimSpace(item)); name {
		case "all":
			return nil
		case "":
		default:
			names = append(names, name)
		}
	}
	return names
}

// golangciEnabled — линтеры и форматтеры, включённые конфигом гейта, по ответу самого
// golangci-lint: проверка не повторяет его правила default и enable.
func golangciEnabled(t *testing.T) map[string]bool {
	t.Helper()
	enabled := map[string]bool{}
	for _, kind := range []string{"linters", "formatters"} {
		cmd := exec.CommandContext(t.Context(), "golangci-lint", kind, "--json")
		cmd.Dir = repoRoot(t)
		out, err := cmd.Output()
		require.NoError(t, err, "golangci-lint %s --json (pinned golangci-lint ставит make setup)", kind)
		var list struct{ Enabled []struct{ Name string } }
		require.NoError(t, json.Unmarshal(out, &list))
		for _, l := range list.Enabled {
			enabled[l.Name] = true
		}
	}
	require.NotEmpty(t, enabled, "golangci-lint reports no enabled linters")
	return enabled
}

func TestNolintNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		comment string
		want    []string
	}{
		{"//nolint:errcheck // reason", []string{"errcheck"}},
		{"//nolint: depguard // reason", []string{"depguard"}},
		{"//nolint:forbidigo, depguard", []string{"forbidigo", "depguard"}},
		{"//nolint:a,,b", []string{"a", "b"}},
		{"// nolint:ErrCheck", []string{"errcheck"}},
		{"/// nolint:gosec", []string{"gosec"}},
		{"//nolint", nil},
		{"//nolint:all", nil},
		{"//nolint:errcheck,all", nil},
		{"//nolintlint:errcheck", nil},
		{"// see //nolint:depguard", nil},
		{"/* nolint:depguard */", nil},
	} {
		t.Run(tc.comment, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, nolintNames(tc.comment))
		})
	}
}
