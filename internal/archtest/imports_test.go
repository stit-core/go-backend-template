package archtest

import (
	"path"
	"slices"
	"strings"
	"testing"
)

// TestImportBoundaries держит границы для не-тестового кода любого каталога internal/domain/*.
func TestImportBoundaries(t *testing.T) {
	t.Parallel()
	for _, f := range goFiles(t) {
		if f.test {
			continue
		}
		for _, imp := range f.imports {
			target, ok := strings.CutPrefix(imp, module+"/")
			if !ok {
				continue
			}
			if why := importViolation(f.pkg, target); why != "" {
				t.Errorf("%s imports %s: %s", f.path, target, why)
			}
		}
	}
}

// importViolation — почему пакет from не вправе импортировать пакет модуля target (пути от
// корня репо); "" — вправе.
func importViolation(from, target string) string {
	fromDomain, fromSub, inDomain := splitDomain(from)
	toDomain, toSub, toDomainPkg := splitDomain(target)
	switch {
	case path.Base(target) == "mocks":
		return "mocks are for tests only"
	case lowLayer(from) && !lowLayer(target):
		return "platform and pkg lie below the domains: they import only platform and pkg"
	case inDomain && !toDomainPkg && !lowLayer(target):
		return "besides domains, a domain imports only platform and pkg: any other package would bridge domains around the ports"
	case !toDomainPkg:
		return ""
	case toSub == "infra/outbox" && from != "internal/domain/"+toDomain+"/module" && !under(from, "cmd/migrate"):
		return "a domain outbox is mounted by its own module/ and migrated by cmd/migrate only"
	case toSub == "infra/postgres/db" && from != "internal/domain/"+toDomain+"/infra/postgres":
		return "sqlc types stay inside the domain's infra/postgres"
	case !inDomain:
		return ""
	case fromDomain != toDomain && fromSub != "module":
		return "another domain is reached through a port and an adapter in module/"
	case fromDomain != toDomain && toSub != "":
		return "module/ reaches another domain through its root package only"
	case fromSub == "" && toSub != "":
		return "the domain root depends on ports, not on its own subpackages"
	case under(fromSub, "delivery") && under(toSub, "infra"):
		return "delivery calls the usecase, not infra"
	}
	return ""
}

// lowLayer — platform и pkg: на них опираются домены, сами они доменов и прочего не знают.
func lowLayer(pkg string) bool {
	return under(pkg, "internal/platform") || under(pkg, "pkg")
}

// ioStdlib — stdlib, которая ходит в ОС, сеть или БД (по префиксу пути); парсеры из net — не I/O.
var (
	ioStdlib   = []string{"database/sql", "net", "os", "syscall"}
	netParsers = []string{"net/mail", "net/netip", "net/url"}
)

// rootForbidden — чего корень домена не видит: транспорт, драйверы и I/O живут за портами.
var rootForbidden = append([]string{
	"git.stit.tech/stit-core/golibinfra/httpkit",
	"git.stit.tech/stit-core/golibinfra/natskit",
	"github.com/danielgtaylor/huma",
	"github.com/jackc/pgx",
}, ioStdlib...)

func TestDomainRootIsTransportAgnostic(t *testing.T) {
	t.Parallel()
	for _, f := range domainRootFiles(t) {
		for _, imp := range f.imports {
			if underAny(imp, rootForbidden) {
				t.Errorf("%s imports %s: the domain root reaches transport, drivers and OS/network I/O only through ports", f.path, imp)
			}
		}
	}
}

// pureLayers — файлы корня домена без I/O: им доступны stdlib без побочных эффектов и потоков
// и платформенные value-примитивы.
var pureLayers = []string{"entity.go", "policy.go", "dto.go", "errors.go", "events.go"}

func TestPureLayersHaveNoIO(t *testing.T) {
	t.Parallel()
	for _, f := range domainRootFiles(t) {
		if !slices.Contains(pureLayers, path.Base(f.path)) {
			continue
		}
		for _, imp := range f.imports {
			if !pureLayerAllowed(imp) {
				t.Errorf("%s imports %s: pure layers allow side-effect-free stdlib, platform/{identity,shared} and pkg/policy only", f.path, imp)
			}
		}
	}
}

func pureLayerAllowed(imp string) bool {
	switch imp {
	case module + "/internal/platform/identity", module + "/internal/platform/shared", module + "/pkg/policy":
		return true
	}
	stdlib := !strings.Contains(imp, ".")
	return stdlib && !underAny(imp, ioStdlib) && !under(imp, "io") && imp != "bufio"
}

// underAny — imp совпадает с одним из путей или вложен в него, кроме парсеров из net.
func underAny(imp string, dirs []string) bool {
	return !slices.Contains(netParsers, imp) && slices.ContainsFunc(dirs, func(dir string) bool { return under(imp, dir) })
}

func domainRootFiles(t *testing.T) []goFile {
	t.Helper()
	var root []goFile
	for _, f := range goFiles(t) {
		if _, sub, ok := splitDomain(f.pkg); ok && sub == "" && !f.test {
			root = append(root, f)
		}
	}
	return root
}
