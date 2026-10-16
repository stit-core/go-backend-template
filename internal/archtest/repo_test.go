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

// TestAllowReadsOnly: allow .claude/settings.json пускает без вопроса чтение — и find, и git branch по списку, — а
// формы тех же команд, которые пишут или исполняют (find -delete, -exec…, git branch -D), и правку go.mod — только с
// вопросом (ask) или никогда (deny). Правило Bash сопоставляется, как его читает Claude Code: «*» — любая строка, deny и
// ask сильнее allow.
func TestAllowReadsOnly(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".claude", "settings.json"))
	require.NoError(t, err)
	var settings struct {
		Permissions struct{ Allow, Ask, Deny []string }
	}
	require.NoError(t, json.Unmarshal(data, &settings))
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
	p := settings.Permissions
	for command, silent := range map[string]bool{
		"find . -name '*.go' -type f":   true,
		"find internal -newer go.mod":   true,
		"git branch --contains HEAD":    true,
		"git branch --list 'nine/*'":    true,
		"find . -name '*.tmp' -delete":  false,
		"find -delete":                  false,
		"find . -name x -exec rm {} +":  false,
		"find . -execdir chmod -x {} ;": false,
		"find . -ok rm {} ;":            false,
		"find . -fprint list.txt":       false,
		"find . -fls list.txt":          false,
		"git branch -D nine/s7-gate":    false,
		"git branch -m main trunk":      false,
		"go mod tidy":                   false,
		"make tidy":                     false,
	} {
		allowed := matches(p.Allow, command) && !matches(p.Ask, command) && !matches(p.Deny, command)
		require.Equal(t, silent, allowed, "%s: allowed without a question — %v", command, allowed)
	}
}
