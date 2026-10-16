package archtest

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
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
	fromImage = regexp.MustCompile(`(?i)^[ \t]*FROM[ \t]+(?:--\S+[ \t]+)*(\S+)(?:[ \t]+AS[ \t]+(\S+))?`)
	// argLine — инструкция ARG Dockerfile: имя и значение по умолчанию.
	argLine = regexp.MustCompile(`(?i)^[ \t]*ARG[ \t]+([A-Za-z_][A-Za-z0-9_]*)(?:=(\S*))?`)
	// composeImage — образ сервиса compose.
	composeImage = regexp.MustCompile(`(?m)^[ \t]*image:[ \t]*["']?([^"'\s#]+)`)
)

// unpinnedBases — базовые образы Dockerfile не по digest. Образ FROM — со значениями ARG до первого FROM: образ
// закрепляют и в одном месте (ARG BASE=<образ>@sha256:…, FROM ${BASE}), а ARG без значения по умолчанию образа не
// называет; стадия сборки по имени — не образ.
func unpinnedBases(dockerfile string) []string {
	args, stages := map[string]string{}, map[string]bool{"scratch": true}
	var unpinned []string
	from := false
	for _, line := range strings.Split(dockerfile, "\n") {
		if m := argLine.FindStringSubmatch(line); m != nil && !from {
			args[m[1]] = strings.Trim(m[2], `"'`)
		}
		m := fromImage.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		from = true
		image := os.Expand(m[1], func(name string) string { return args[name] })
		if !stages[strings.ToLower(image)] && !strings.Contains(image, "@sha256:") {
			unpinned = append(unpinned, m[1])
		}
		if m[2] != "" {
			stages[strings.ToLower(m[2])] = true
		}
	}
	return unpinned
}

// TestImagesPinned: образ закреплён (CLAUDE.md § Не делать никогда: теги :latest и :edge): базовый образ
// Dockerfile — по digest (@sha256:): тег перевешивается, digest — нет; стадия сборки по имени — не образ. Образ
// compose стека e2e — с тегом, не :latest и не :edge.
func TestImagesPinned(t *testing.T) {
	t.Parallel()
	const digest = "@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab"
	for dockerfile, want := range map[string][]string{
		"FROM golang:1.26" + digest + " AS build\nFROM build\nFROM scratch\n":                nil,
		"ARG BASE=gcr.io/distroless/static" + digest + "\nFROM ${BASE}\nFROM $BASE AS run\n": nil,
		"FROM golang:latest\n":                                   {"golang:latest"},
		"ARG BASE=golang:1.26\nFROM ${BASE}\n":                   {"${BASE}"},
		"ARG BASE\nFROM $BASE\n":                                 {"$BASE"},
		"FROM scratch\nARG BASE=x" + digest + "\nFROM ${BASE}\n": {"${BASE}"},
	} {
		require.Equal(t, want, unpinnedBases(dockerfile), dockerfile)
	}
	root := repoRoot(t)
	for _, f := range trackedFiles(t, root) {
		base := path.Base(f)
		switch {
		case strings.HasPrefix(base, "Dockerfile"):
			for _, image := range unpinnedBases(readFiles(t, root, f)[f]) {
				t.Errorf("%s: FROM %s — a base image goes by digest (@sha256:), a tag moves", f, image)
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

// TestAllowReadsOnly: allow .claude/settings.json пускает без вопроса только то, что не пишет: ни одна пишущая форма
// команды (find -delete и -exec, git branch -D, go mod tidy) с его правилами не сходится. Раздела ask нет: ask
// спрашивает человека в любом режиме, и в bypass, — а чтение find и git branch Claude Code пускает без вопроса сам
// (встроенный набор команд только для чтения), и deny законному чтению не мешает. Правило Bash сопоставляется, как
// его читает Claude Code: «*» — любая строка.
func TestAllowReadsOnly(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".claude", "settings.json"))
	require.NoError(t, err)
	var settings struct{ Permissions map[string]json.RawMessage }
	require.NoError(t, json.Unmarshal(data, &settings))
	_, ask := settings.Permissions["ask"]
	require.False(t, ask, ".claude/settings.json: ask asks the human in every mode, bypass too")
	var allow, deny []string
	require.NoError(t, json.Unmarshal(settings.Permissions["allow"], &allow))
	require.NoError(t, json.Unmarshal(settings.Permissions["deny"], &deny))
	matches := func(rules []string, command string) bool {
		for _, rule := range rules {
			pattern, ok := strings.CutPrefix(rule, "Bash(")
			if !ok {
				continue
			}
			parts := strings.Split(strings.TrimSuffix(pattern, ")"), "*")
			for i := range parts {
				parts[i] = regexp.QuoteMeta(parts[i])
			}
			if regexp.MustCompile("^" + strings.Join(parts, ".*") + "$").MatchString(command) {
				return true
			}
		}
		return false
	}
	for _, command := range []string{
		"find . -name '*.tmp' -delete", "find . -name x -exec rm {} +", "find . -execdir chmod -x {} ;", "find . -ok rm {} ;",
		"find . -fprint list.txt", "find . -fls list.txt", "git branch -D nine/s7-gate", "git branch -m main trunk",
		"go mod tidy", "make tidy",
	} {
		require.False(t, matches(allow, command), "%s: allow lets a writing form through without a question", command)
	}
	for _, command := range []string{
		"find . -name '*.go' -type f", "find internal -path '*/mocks/*' -name '*.go'", "git branch --list 'nine/*'",
		"git branch --contains HEAD",
	} {
		require.False(t, matches(deny, command), "%s: deny blocks a read", command)
	}
}
