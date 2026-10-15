package archtest

import (
	"bufio"
	"bytes"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// generatedHeader — заголовок сгенерированного файла: по нему файл выпадает из покрытия
// (scripts/tools/cover.sh) и из lint.
var generatedHeader = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

// sqlcFiles — файлы, которые sqlc пишет в каталог вывода домена db: общие и по файлу запросов из соседнего
// queries/ (users.sql → users.sql.go).
func sqlcFiles(t *testing.T, db string) map[string]bool {
	t.Helper()
	files := map[string]bool{"db.go": true, "models.go": true, "querier.go": true, "copyfrom.go": true, "batch.go": true}
	queries, err := filepath.Glob(filepath.Join(repoRoot(t), path.Dir(db), "queries", "*.sql"))
	require.NoError(t, err)
	for _, q := range queries {
		files[filepath.Base(q)+".go"] = true
	}
	return files
}

// TestGeneratedHeaderOnlyInGeneratorOutput: заголовок сгенерированного выводит файл из покрытия и lint, поэтому
// он стоит только у вывода генераторов гейта — sqlc (каталог infra/postgres/db домена, файлы, которые sqlc
// пишет) и mockery (mocks), — а в каталоге вывода sqlc нет других файлов: рукописный файл под заголовком обошёл
// бы гейт, а без него — make gen. Сгенерированный клиент pkg/ — генератор в make gen и его каталог в этом тесте:
// правка гейта.
func TestGeneratedHeaderOnlyInGeneratorOutput(t *testing.T) {
	t.Parallel()
	sqlc := map[string]map[string]bool{} // каталог вывода sqlc → его файлы
	for _, f := range goFiles(t) {
		src, err := os.ReadFile(filepath.Join(repoRoot(t), f.path))
		require.NoError(t, err)
		gen := generatedHeader.Match(src)
		switch {
		case strings.HasSuffix(f.pkg, "/infra/postgres/db"):
			if sqlc[f.pkg] == nil {
				sqlc[f.pkg] = sqlcFiles(t, f.pkg)
			}
			if !gen || !sqlc[f.pkg][path.Base(f.path)] {
				t.Errorf("%s: sqlc writes no such file — infra/postgres/db holds only the output of make gen (db.go, models.go, <file of queries/>.sql.go)", f.path)
			}
		case gen && path.Base(f.pkg) != "mocks":
			t.Errorf("%s: the generated-code header takes the file out of coverage and lint — only the output of the generators of make gen carries it: sqlc (infra/postgres/db), mockery (mocks)", f.path)
		}
	}
}

// TestBuildConstraints: гейт проверяет код в сборке с тегом integration, образ собирает без тегов. Файл,
// который ограничение сборки исключает из сборки гейта, выпал бы из vet, lint, тестов и покрытия, а в
// бинарь попал бы; ограничение у не-теста разводит то, что проверено, и то, что собрано. Поэтому
// ограничение — только //go:build integration и только у *_test.go, а суффикс имени (_windows.go,
// _arm64.go) не исключает файл из сборки гейта на этой машине. Каталоги testdata и «_»-файлы go не видит.
func TestBuildConstraints(t *testing.T) {
	t.Parallel()
	gate := build.Default
	gate.BuildTags = []string{"integration"}
	for _, f := range goFiles(t) {
		if slices.ContainsFunc(strings.Split(f.path, "/"), func(part string) bool {
			return part == "testdata" || strings.HasPrefix(part, "_")
		}) {
			continue
		}
		file := filepath.Join(repoRoot(t), f.path)
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, line := range buildLines(src) {
			if expr, err := constraint.Parse(line); err != nil || !f.test || expr.String() != "integration" {
				t.Errorf("%s: %q — a build constraint is //go:build integration on a *_test.go only: "+
					"another one takes the file out of vet, lint, tests and coverage of the gate or out of the image", f.path, line)
			}
		}
		ok, err := gate.MatchFile(filepath.Dir(file), filepath.Base(file))
		require.NoError(t, err, f.path)
		if !ok {
			t.Errorf("%s: the build of the gate (tag integration) skips the file: its name (_windows.go, _arm64.go) or "+
				"constraint takes it out of vet, lint, tests and coverage", f.path)
		}
	}
}

// buildLines — строки ограничений сборки (//go:build и // +build) в заголовке файла до package.
func buildLines(src []byte) []string {
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			break
		}
		if constraint.IsGoBuild(line) || constraint.IsPlusBuild(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

// linterDirective — директива, которой линтер глушит себя мимо //nolint: revive:disable… и своя директива
// gosec (её gosec не слушает — nosec: true в .golangci.yml, — а читатель примет за действующую).
var linterDirective = regexp.MustCompile(`^(//|/\*)\s*revive:|#nosec`)

// TestLinterDirectives: ослабление lint — только //nolint с именем включённого линтера и причиной
// (nolintlint): директива самого линтера глушит его без имени и причины.
func TestLinterDirectives(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(repoRoot(t), f.path), nil, parser.ParseComments)
		require.NoError(t, err)
		for _, group := range file.Comments {
			for _, c := range group.List {
				if linterDirective.MatchString(c.Text) {
					t.Errorf("%s:%d: %q — a linter directive past //nolint: weaken lint only by //nolint:<linter> // <reason>",
						f.path, fset.Position(c.Pos()).Line, c.Text)
				}
			}
		}
	}
}
