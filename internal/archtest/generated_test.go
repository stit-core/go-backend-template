package archtest

import (
	"bufio"
	"bytes"
	"go/build"
	"go/build/constraint"
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

// generatorOutput — каталог, куда пишет генератор: sqlc (infra/postgres/db домена) и mockery (mocks).
func generatorOutput(pkg string) bool {
	return strings.HasSuffix(pkg, "/infra/postgres/db") || path.Base(pkg) == "mocks"
}

// TestGeneratedHeaderOnlyInGeneratorOutput: заголовок сгенерированного выводит файл из покрытия и lint,
// поэтому он стоит только там, куда пишут генераторы, — в выводе sqlc и mockery или в пакете pkg/, где
// сгенерировано всё (клиент gRPC, OpenAPI); рукописный файл с ним обошёл бы гейт.
func TestGeneratedHeaderOnlyInGeneratorOutput(t *testing.T) {
	t.Parallel()
	generated := map[string]bool{}
	pkgFiles := map[string][]string{}
	for _, f := range goFiles(t) {
		src, err := os.ReadFile(filepath.Join(repoRoot(t), f.path))
		require.NoError(t, err)
		generated[f.path] = generatedHeader.Match(src)
		if !f.test {
			pkgFiles[f.pkg] = append(pkgFiles[f.pkg], f.path)
		}
	}
	for file, gen := range generated {
		pkg := path.Dir(file)
		switch {
		case !gen, generatorOutput(pkg):
		case under(pkg, "pkg") && !slices.ContainsFunc(pkgFiles[pkg], func(f string) bool { return !generated[f] }):
		default:
			t.Errorf("%s: the generated-code header takes the file out of coverage and lint — only generator output carries it: sqlc (infra/postgres/db), mockery (mocks), a fully generated client package in pkg/", file)
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
