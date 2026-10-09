package archtest

import (
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
