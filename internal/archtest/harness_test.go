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

// fsRoots — корни файловой системы: `/tmp`, `/app` в прозе — пути, а не скиллы.
var fsRoots = []string{"app", "bin", "dev", "etc", "home", "migrations", "mnt", "opt", "proc", "root", "run", "srv", "sys", "tmp", "usr", "var"}

// domainLayout — файлы раскладки домена (docs/reference/architecture.md): в прозе это термины, а не ссылки —
// файл есть в домене, когда есть его содержимое.
var domainLayout = []string{
	"entity.go", "dto.go", "errors.go", "events.go", "policy.go", "usecase.go", "trusted.go", "metrics.go",
	"handler.go", "view.go", "repository.go", "module.go", "adapters.go", "contract_test.go",
}

var (
	codeSpan    = regexp.MustCompile("(?s)```.*?```|`[^`\n]+`")
	makeCall    = regexp.MustCompile(`\bmake\s+([a-z][a-z0-9-]*)`)
	makeTarget  = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*):`)
	goSymbol    = regexp.MustCompile(`\.[A-Z]\w*$`)
	placeholder = regexp.MustCompile(`[<>{}*$\[\]|…]|NNNN|\.\.\.`)
	slashName   = regexp.MustCompile(`^/([a-z][a-z0-9-]*)$`)
	apiPath     = regexp.MustCompile(`^/api/[a-z0-9/_-]+$`)
	kebabName   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	testName    = regexp.MustCompile(`^Test[A-Z0-9_]\w*$`)
	// testRef — имя теста в слове кода прозы: `-run '^TestX$'`, `TestA|TestB`, `TestX/sub`; «Test<Name>» — нет.
	testRef = regexp.MustCompile(`(?:^|[^A-Za-z0-9_<])(Test[A-Z0-9_]\w*)`)
	// fileName — файл без пути (`Makefile`, `.env.example`, `errors.go`): есть в репо под этим именем.
	fileName = regexp.MustCompile(`^(Makefile|LICENSE|Dockerfile(\.\w+)?|\.?\w[\w.-]*\.(md|go|sql|ya?ml|json|sh|py|toml|txt|mod|sum|example))$`)
	// namedBy — слово перед `имя`: rule или правило — .claude/rules/<имя>.md, skill или скилл — скилл;
	// «nolintlint-правило» — не оно.
	namedBy = regexp.MustCompile(`(?i)(^|[\s(«"])(rules?|правил\p{L}*|skills?|скилл\p{L}*)\s+$`)
	// listSep — между именами перечня после rule/skill: «правила `a`, `b` и `c`».
	listSep = regexp.MustCompile(`^(\s*,\s*((и|или)\s+)?|\s+(и|или)\s+|\s*/\s*)$`)
)

// TestProseReferencesExist: в коде прозы (CLAUDE.md, README, docs/, .claude/rules и skills) есть в репо
// make-цели, пути (от корня репо или, если первый сегмент лежит рядом с документом, — от его каталога) и
// файлы без пути, скиллы (`/имя`, skill `имя`), правила (rule `имя`, и каждое имя перечня после) и тесты
// (`TestИмя` — функция *_test.go) — иначе агент идёт по мёртвой ссылке, а инвариант с пометкой теста
// кажется машинным. Репо — то, что отслеживает git: локальное (test/.venv, неотслеживаемый
// файл) есть не в каждом клоне. Плейсхолдеры (<d>, NNNN, *) — не ссылки; артефакт docs/stack/<kind>.md —
// по его шаблону; /имя, которое не скилл, — путь контракта openapi.json, встроенная команда или корень
// файловой системы; путь /api/… — операция openapi.json; правило с именем включённого линтера — его
// правило. Опечатку в первом сегменте пути и ссылку простым текстом вне кода проверка не видит.
func TestProseReferencesExist(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err)
	var targets []string
	for _, m := range makeTarget.FindAllStringSubmatch(string(makefile), -1) {
		targets = append(targets, m[1])
	}
	architecture, err := os.ReadFile(filepath.Join(root, "docs", "reference", "architecture.md"))
	require.NoError(t, err)
	for _, name := range domainLayout {
		require.Contains(t, string(architecture), name, "domainLayout names a file the domain layout of architecture.md does not")
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
	tests := testFuncs(t, root, files)
	skill := func(name string) bool {
		return repo[".claude/skills/"+name+"/SKILL.md"] || slices.Contains(notRepoSkills, name)
	}
	// Проект с внешней идентичностью удалил домены идентичности по решению: образцы прозы в них — в версии
	// шаблона, их проверяет самопроверка шаблона.
	var pruned []string
	if identityExternal(t) {
		pruned = identityPaths(t)
	}
	inPruned := func(ref string) bool {
		return slices.ContainsFunc(pruned, func(p string) bool { return under(strings.TrimSuffix(ref, "/"), p) })
	}

	exempt := identityExempt(t, root)
	linters := golangciEnabled(t)
	var dead []string
	for _, doc := range proseFiles(files) {
		data, err := os.ReadFile(filepath.Join(root, doc))
		require.NoError(t, err)
		text := string(data)
		listKind, prevEnd := "", 0 // имя перечня после rule/skill: «правила `a`, `b`»
		for _, loc := range codeSpan.FindAllStringIndex(text, -1) {
			span := strings.Trim(text[loc[0]:loc[1]], "`")
			for _, m := range makeCall.FindAllStringSubmatch(span, -1) {
				if !slices.Contains(targets, m[1]) && !slices.Contains(hubTargets, m[1]) {
					dead = append(dead, doc+": make "+m[1])
				}
			}
			kind, listed := "", false
			if m := namedBy.FindStringSubmatch(text[max(0, loc[0]-64):loc[0]]); m != nil {
				kind = "skill"
				if word := strings.ToLower(m[2]); strings.HasPrefix(word, "rule") || strings.HasPrefix(word, "правил") {
					kind = "rule"
				}
			} else if listKind != "" && listSep.MatchString(text[prevEnd:loc[0]]) {
				kind, listed = listKind, true
			}
			listKind, prevEnd = "", loc[1]
			if kind != "" && kebabName.MatchString(span) {
				listKind = kind
				missing := kind == "rule" && !repo[".claude/rules/"+span+".md"] && !linters[span] || kind == "skill" && !skill(span)
				// Имя дальше по перечню, которое называет каталог или файл репо, — не правило и не скилл:
				// «правило `comments` и `migrations`», «скилл `scaffold`, `field`».
				other := listed && (repo[span+"/"] || names[span] || names[span+".md"])
				if missing && !other {
					dead = append(dead, doc+": "+kind+" "+span)
				}
			}
			for _, word := range strings.Fields(span) {
				for _, m := range testRef.FindAllStringSubmatch(word, -1) {
					if !tests[m[1]] && !exempt(doc) {
						dead = append(dead, doc+": "+m[1])
					}
				}
				ref := strings.TrimRight(strings.TrimLeft(word, `"'(`), `"'.,;:()`) // точка в начале — .claude
				ref, _, _ = strings.Cut(ref, ":")
				ref = strings.TrimPrefix(ref, "./")
				if placeholder.MatchString(ref) {
					continue
				}
				if m := slashName.FindStringSubmatch(ref); m != nil {
					if !skill(m[1]) && !api[ref] && !slices.Contains(fsRoots, m[1]) {
						dead = append(dead, doc+": "+ref)
					}
					continue
				}
				// Путь API — операция openapi.json; у проекта с внешней идентичностью пути её доменов ушли вместе
				// с ними.
				if apiPath.MatchString(ref) {
					if !api[ref] && pruned == nil {
						dead = append(dead, doc+": "+ref)
					}
					continue
				}
				if testName.MatchString(ref) {
					continue // имя теста сверено выше
				}
				top, _, isPath := strings.Cut(ref, "/")
				near := path.Join(path.Dir(doc), top) // первый сегмент рядом с документом: references/…, stack/…
				switch {
				case !isPath && fileName.MatchString(ref):
					if !names[ref] && !slices.Contains(domainLayout, ref) && !gitIgnored(t, root, ref) {
						dead = append(dead, doc+": "+ref)
					}
				case isPath && repo[top+"/"]:
					if clean := goSymbol.ReplaceAllString(ref, ""); !inRepo(repo, clean) && !inPruned(clean) && !gitIgnored(t, root, ref) {
						dead = append(dead, doc+": "+ref)
					}
				case isPath && top != "" && top != ".." && repo[near+"/"]:
					if clean := path.Join(path.Dir(doc), goSymbol.ReplaceAllString(ref, "")); !inRepo(repo, clean) && !inPruned(clean) && !gitIgnored(t, root, clean) {
						dead = append(dead, doc+": "+ref)
					}
				}
			}
		}
	}
	proseAllowlist.Check(t, dead, "the prose points to a missing make target, path, file, skill, rule or test")
}

// testFuncs — функции Test… файлов *_test.go репо.
func testFuncs(t *testing.T, root string, files []string) map[string]bool {
	t.Helper()
	tests := map[string]bool{}
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, f), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && testName.MatchString(fn.Name.Name) {
				tests[fn.Name.Name] = true
			}
		}
	}
	require.NotEmpty(t, tests)
	return tests
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

// identityExempt — у проекта с внешней идентичностью ссылки файлов шаблона (они есть в каком-нибудь теге
// template/v*) на её домены мертвы по решению — их проверяет самопроверка шаблона; свои файлы проекта
// проверяются. Тегов шаблона нет — пропускаются все; идентичность своя — ни один.
func identityExempt(t *testing.T, root string) func(rel string) bool {
	t.Helper()
	if !identityExternal(t) {
		return func(string) bool { return false }
	}
	tags := strings.Fields(git(t, root, "tag", "--list", "template/v*"))
	files := map[string]bool{}
	for _, tag := range tags {
		for _, f := range strings.Split(git(t, root, "ls-tree", "-r", "-z", "--name-only", tag), "\x00") {
			files[f] = true
		}
	}
	return func(rel string) bool { return len(tags) == 0 || files[rel] }
}

// readRulesFirst — процедуры, которые пишут код (каталог — каждый его .md): шагом 0 они читают свои правила.
var readRulesFirst = []string{".claude/skills/scaffold/references", ".claude/skills/triage-failure/SKILL.md"}

var (
	heading1 = regexp.MustCompile(`(?m)^# .*$`)
	stepZero = regexp.MustCompile(`^\*\*Шаг 0\*\* — прочитай инструментом Read `)
	rulePath = regexp.MustCompile("`(\\.claude/rules/[a-z0-9-]+\\.md)`")
)

// TestRulesAndSkillsLoad: правило приходит в контекст после Read, Write или Edit файла под его paths: (или его
// чтения командой Bash) — первой записи нового файла оно не застаёт, поэтому процедура, которая пишет код, читает
// свои правила шагом 0; скилл зовут по name:, без него — по каталогу. Glob, который не ложится ни на один файл
// репо, правило без paths: (грузится в каждую сессию), name: не по каталогу и процедура без шага 0 ничем не
// падают: агент работает без правила, и этого никто не видит. paths: читается, как его читает Claude Code: список
// YAML или строка через запятую, «{a,b}» раскрыт, glob ложится на файл по правилам .gitignore. Glob без файлов
// репо — только локальный файл, который игнорирует git, и у проекта с внешней идентичностью — glob правила
// шаблона (identityExempt): слои, которые были только у её доменов (events.go, delivery/nats), пустуют до своих.
func TestRulesAndSkillsLoad(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := trackedFiles(t, root)
	exempt := identityExempt(t, root)
	for _, f := range files {
		dir := path.Dir(f)
		rule := under(dir, ".claude/rules") && path.Ext(f) == ".md" // правила Claude Code ищет и в подкаталогах
		skill := path.Dir(dir) == ".claude/skills" && path.Base(f) == "SKILL.md"
		procedure := path.Ext(f) == ".md" && slices.ContainsFunc(readRulesFirst, func(p string) bool { return under(f, p) })
		if !rule && !skill && !procedure {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, f))
		require.NoError(t, err)
		text := strings.ReplaceAll(string(data), "\r\n", "\n") // CRLF Claude Code читает так же
		head := frontmatter(text)
		switch {
		case rule:
			globs := ruleGlobs(head["paths"])
			if len(globs) == 0 {
				t.Errorf("%s: no paths: that Claude Code reads — the rule loads into every session; always-on text belongs in CLAUDE.md", f)
			}
			for _, g := range globs {
				if !globMatches(t, root, g.variants) && !exempt(f) {
					t.Errorf("%s: paths: %q matches no file of the repo — the rule never loads", f, g.raw)
				}
			}
		case skill:
			if name, ok := head["name"]; ok && !slices.Equal(name, []string{path.Base(dir)}) {
				t.Errorf("%s: name: %q — the skill is called by its directory, %q", f, name, path.Base(dir))
			}
		}
		if !procedure {
			continue
		}
		var first string
		if loc := heading1.FindStringIndex(text); loc != nil {
			first, _, _ = strings.Cut(strings.TrimLeft(text[loc[1]:], "\n"), "\n\n")
		}
		rules := rulePath.FindAllStringSubmatch(first, -1)
		if !stepZero.MatchString(first) || len(rules) == 0 {
			t.Errorf("%s: the paragraph under the title is not «**Шаг 0** — прочитай инструментом Read <rules>»: the procedure writes code without its rules", f)
		}
		for _, r := range rules {
			if !slices.Contains(files, r[1]) {
				t.Errorf("%s: step 0 reads %s, which is not in the repo", f, r[1])
			}
		}
	}
}

// frontmatter — шапка .md между строками «---»: ключ верхнего уровня → значение после «ключ:» и пункты его
// списка (блочного «- …» или flow «[a, b]», и в несколько строк) без кавычек и комментариев YAML. Шапку, которую
// YAML не разберёт, Claude Code читает второй раз, взяв в кавычки значение «ключ: …» верхнего уровня со знаком
// YAML (paths: **/*.go); не разобрал и так (пункт «- **/*.go») — шапки нет, nil.
func frontmatter(text string) map[string][]string {
	lines := strings.Split(text, "\n")
	if strings.TrimRight(lines[0], " \t") != "---" {
		return nil
	}
	for i, line := range lines[1:] {
		if strings.TrimRight(line, " \t") == "---" {
			if keys, ok := yamlHead(lines[1:i+1], false); ok {
				return keys
			}
			keys, _ := yamlHead(lines[1:i+1], true)
			return keys
		}
	}
	return nil // шапка не закрыта — это не шапка
}

var (
	// topValue — «ключ: значение» верхнего уровня, как его видит второй разбор шапки Claude Code.
	topValue = regexp.MustCompile(`^([a-zA-Z_-]+):\s+(\S.*)$`)
	// yamlSign — знаки, из-за которых Claude Code при втором разборе берёт значение в кавычки.
	yamlSign = regexp.MustCompile("[{}\\[\\]*&#!|>%@`]|: ")
)

// yamlHead — ключи строк шапки; false — YAML её не разберёт (и незакрытый flow-список). second — второй разбор
// Claude Code: значение «ключ: …» верхнего уровня без кавычек и не flow-список, со знаком YAML, — строка как есть;
// незакрытый «[» он берёт в кавычки, и строки списка под ним ломают разбор.
func yamlHead(lines []string, second bool) (map[string][]string, bool) {
	keys := map[string][]string{}
	key, flow := "", ""
	for _, raw := range lines {
		if m := topValue.FindStringSubmatch(strings.TrimRight(raw, " \t")); second && m != nil {
			v := m[2]
			flowList := strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]")
			if !quoted(v) && !flowList && yamlSign.MatchString(v) {
				if strings.HasPrefix(v, "[") {
					return nil, false
				}
				key = m[1]
				keys[key] = append(keys[key], v)
				continue
			}
		}
		line := strings.TrimRight(yamlUncomment(raw), " \t")
		item := strings.TrimSpace(line)
		var values []string
		switch {
		case flow != "":
			if flow += " " + item; !strings.HasSuffix(flow, "]") {
				continue
			}
			values, flow = splitFlow(flow[1:len(flow)-1]), ""
		case item == "":
		case strings.HasPrefix(item, "- ") && key != "":
			values = append(values, item[2:])
		case line[0] != ' ' && line[0] != '\t':
			k, v, _ := strings.Cut(line, ":")
			key, v = strings.TrimSpace(k), strings.TrimSpace(v)
			switch {
			case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
				values = splitFlow(v[1 : len(v)-1])
			case strings.HasPrefix(v, "["):
				flow = v
			case v != "":
				values = append(values, v)
			}
		}
		for _, v := range values {
			s, ok := yamlScalar(v)
			if !ok {
				return nil, false
			}
			keys[key] = append(keys[key], s)
		}
	}
	return keys, flow == ""
}

// quoted — значение YAML в кавычках.
func quoted(v string) bool {
	return len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'')
}

// yamlUncomment — строка без комментария YAML: «#» в начале или после пробела, вне кавычек.
func yamlUncomment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return line[:i]
		}
	}
	return line
}

// yamlScalar — значение без кавычек YAML: «"…"» с \" и \\ или «'…'» с удвоенной кавычкой. ok=false — без
// кавычек значение начинается со знака YAML (якорь, словарь, тег): строкой его YAML не прочтёт.
func yamlScalar(s string) (string, bool) {
	s = strings.TrimSpace(s)
	switch {
	case len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"':
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s[1 : len(s)-1]), true
	case len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'':
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), true
	}
	return s, s == "" || strings.IndexByte("*&!{}@`", s[0]) < 0
}

// splitFlow — пункты flow-списка YAML «a, "b"» между запятыми вне кавычек.
func splitFlow(s string) []string {
	var parts []string
	var quote byte
	from := 0
	for i := 0; i <= len(s); i++ {
		switch {
		case i == len(s) || s[i] == ',' && quote == 0:
			if part := strings.TrimSpace(s[from:i]); part != "" {
				parts = append(parts, part)
			}
			from = i + 1
		case quote != 0 && s[i] == quote:
			quote = 0
		case quote == 0 && (s[i] == '"' || s[i] == '\''):
			quote = s[i]
		}
	}
	return parts
}

// splitBraces — glob'ы строки paths: между запятыми вне «{…}», как делит Claude Code.
func splitBraces(s string) []string {
	var parts []string
	depth, from := 0, 0
	for i := 0; i <= len(s); i++ {
		switch {
		case i == len(s) || s[i] == ',' && depth == 0:
			if part := strings.TrimSpace(s[from:i]); part != "" {
				parts = append(parts, part)
			}
			from = i + 1
		case s[i] == '{':
			depth++
		case s[i] == '}':
			depth--
		}
	}
	return parts
}

// ruleGlob — glob paths: как написан и его варианты для сверки.
type ruleGlob struct {
	raw      string
	variants []string
}

var braceGroup = regexp.MustCompile(`^([^{]*)\{([^}]+)\}(.*)$`)

// ruleGlobs — glob'ы paths:, как их читает Claude Code: пункт списка и строка делятся по запятым вне «{…}»,
// «{a,b}» раскрыт, «/**» на конце снят. nil — paths: нет или он из одних «**»: правило грузится всегда.
func ruleGlobs(values []string) []ruleGlob {
	var globs []ruleGlob
	always := true
	for _, value := range values {
		for _, raw := range splitBraces(value) {
			g := ruleGlob{raw: raw}
			for todo := []string{raw}; len(todo) > 0; {
				s := todo[len(todo)-1]
				todo = todo[:len(todo)-1]
				if m := braceGroup.FindStringSubmatch(s); m != nil {
					for alt := range strings.SplitSeq(m[2], ",") {
						todo = append(todo, m[1]+strings.TrimSpace(alt)+m[3])
					}
				} else if s = strings.TrimSuffix(s, "/**"); s != "" {
					g.variants = append(g.variants, s)
					always = always && s == "**"
				}
			}
			if len(g.variants) > 0 {
				globs = append(globs, g)
			}
		}
	}
	if always {
		return nil
	}
	return globs
}

// globMatches — вариант glob'а ложится на файл индекса git по правилам .gitignore (без «/» — на любой глубине,
// как у Claude Code) или называет локальный файл, который игнорирует git.
func globMatches(t *testing.T, root string, variants []string) bool {
	t.Helper()
	args := []string{"ls-files", "--cached", "--ignored"}
	for _, v := range variants {
		args = append(args, "--exclude="+v)
		// Литерал — путь от корня репо: ведущий «/» лишь привязывает его к корню, «..» — за пределами репо.
		lit := strings.TrimPrefix(v, "/")
		if !strings.ContainsAny(v, "*?[!") && lit != ".." && !strings.HasPrefix(lit, "../") && gitIgnored(t, root, lit) {
			return true
		}
	}
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "git ls-files %v", variants)
	return len(out) > 0
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
		cmd := exec.CommandContext(t.Context(), "golangci-lint", kind, "--json", "--config", ".golangci.yml")
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
