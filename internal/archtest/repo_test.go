package archtest

import (
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// secretName — файл окружения с секретами (.env, .env.local, .env.production) или ключа (*.pem, *.key).
var secretName = regexp.MustCompile(`(^|/)\.env(\.[^/]*)?$|\.(pem|key)$`)

// secretFile — файл, которому не место в репо: шаблон окружения (*.example) — не секрет.
func secretFile(name string) bool {
	return secretName.MatchString(name) && !strings.HasSuffix(name, ".example")
}

// TestNoSecretFiles: секретам не место в репо (CLAUDE.md § Не делать никогда): файл окружения или ключа в индексе —
// отказ, и под именем, которого .gitignore не знает, и добавленный мимо него (git add -f).
func TestNoSecretFiles(t *testing.T) {
	t.Parallel()
	for name, secret := range map[string]bool{
		".env": true, "deploy/.env.production": true, "server.key": true, "tls/cert.pem": true,
		".env.example": false, ".env.local.example": false, ".envrc": false, "internal/platform/env.go": false,
	} {
		require.Equal(t, secret, secretFile(name), name)
	}
	for _, f := range trackedFiles(t, repoRoot(t)) {
		if secretFile(f) {
			t.Errorf("%s: a secret in the repo — the environment and the keys stay out of git, a template is *.example", f)
		}
	}
}

var (
	// fromImage — образ инструкции FROM Dockerfile: слово после FROM и его флагов (--platform=…), имя стадии — AS.
	fromImage = regexp.MustCompile(`(?im)^[ \t]*FROM[ \t]+(?:--\S+[ \t]+)*(\S+)(?:[ \t]+AS[ \t]+(\S+))?`)
	// composeImage — образ сервиса compose.
	composeImage = regexp.MustCompile(`(?m)^[ \t]*image:[ \t]*["']?([^"'\s#]+)`)
)

// TestImagesPinned: образ закреплён (CLAUDE.md § Не делать никогда: теги :latest и :edge): базовый образ
// Dockerfile — по digest (@sha256:): тег перевешивается, digest — нет; стадия сборки по имени — не образ. Образ
// compose стека e2e — с тегом, не :latest и не :edge.
func TestImagesPinned(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, f := range trackedFiles(t, root) {
		base := path.Base(f)
		switch {
		case strings.HasPrefix(base, "Dockerfile"):
			stages := map[string]bool{"scratch": true}
			for _, m := range fromImage.FindAllStringSubmatch(readFiles(t, root, f)[f], -1) {
				if !stages[strings.ToLower(m[1])] && !strings.Contains(m[1], "@sha256:") {
					t.Errorf("%s: FROM %s — a base image goes by digest (@sha256:), a tag moves", f, m[1])
				}
				if m[2] != "" {
					stages[strings.ToLower(m[2])] = true
				}
			}
		case strings.Contains(base, "compose") && (path.Ext(base) == ".yml" || path.Ext(base) == ".yaml"):
			for _, m := range composeImage.FindAllStringSubmatch(readFiles(t, root, f)[f], -1) {
				name := m[1][strings.LastIndex(m[1], "/")+1:]
				tag := ""
				if i := strings.LastIndex(name, ":"); i >= 0 {
					tag = name[i+1:]
				}
				if tag == "" || tag == "latest" || tag == "edge" {
					t.Errorf("%s: image %s — an image of the stack has a pinned tag, not :latest or :edge", f, m[1])
				}
			}
		}
	}
}
