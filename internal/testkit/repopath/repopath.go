package repopath

import (
	"path/filepath"
	"runtime"
)

// Migrations returns the absolute path to the repo's migrations/ directory.
// Relative file:// paths break in golang-migrate (".." is parsed as a URL host).
func Migrations() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
}
