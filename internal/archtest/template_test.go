package archtest

import (
	"bytes"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTemplatePaths: строку .template/paths без файлов перенос обвязки молча пропустит; тег обвязки
// на HEAD совпадает с .template/version — форк с коммита тега начнёт перенос с этой версии.
func TestTemplatePaths(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := trackedFiles(t, root)
	paths, err := os.ReadFile(filepath.Join(root, ".template", "paths"))
	require.NoError(t, err)
	for line := range strings.Lines(string(paths)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.TrimPrefix(line, ":(exclude)")
		if !slices.ContainsFunc(files, func(f string) bool { return under(f, p) }) {
			t.Errorf(".template/paths: %q is not a tracked file or directory", line)
		}
	}

	version, err := os.ReadFile(filepath.Join(root, ".template", "version"))
	require.NoError(t, err)
	v := strings.TrimSpace(string(version))
	require.Regexp(t, `^template/v\d+\.\d+\.\d+$`, v, ".template/version")
	cmd := exec.CommandContext(t.Context(), "git", "tag", "--points-at", "HEAD", "--list", "template/v*")
	cmd.Dir = root
	tags, err := cmd.Output()
	require.NoError(t, err, "git tag --points-at HEAD")
	for _, tag := range strings.Fields(string(tags)) {
		require.Equal(t, tag, v, "HEAD is tagged %s, .template/version says %s", tag, v)
		kind := exec.CommandContext(t.Context(), "git", "cat-file", "-t", "refs/tags/"+tag)
		kind.Dir = root
		out, err := kind.Output()
		require.NoError(t, err, "git cat-file -t %s", tag)
		require.Equal(t, "tag", strings.TrimSpace(string(out)), "%s is a lightweight tag: make template-update shows the annotation of the tag", tag)
	}
}

var conflictMarker = regexp.MustCompile(`(?m)^(<{7}|>{7})( |$)`)

// TestNoConflictMarkers: маркер конфликта переноса обвязки в прозе или скрипте не ломает сборку —
// без проверки он прошёл бы гейт.
func TestNoConflictMarkers(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, at := range conflictMarkers(t, root, trackedFiles(t, root)) {
		t.Errorf("%s: merge conflict marker", at)
	}
}

func TestConflictMarkers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"conflict.md": "a\n<<<<<<< ours\nb\n=======\nc\n>>>>>>> theirs\n",
		"heading.md":  "Заголовок\n=======\n",
		"sub/file.md": "s\n",
	})
	require.NoError(t, os.Symlink("sub", filepath.Join(root, "link")))
	// sub — как submodule в git ls-files, link — tracked symlink на каталог.
	require.Equal(t, []string{"conflict.md:2"}, conflictMarkers(t, root, []string{"conflict.md", "heading.md", "sub", "link"}))
}

// TestTemplateUpdate: make template-update переносит в форк с другим module path и своей историей
// правки путей шаблона от версии форка до тега и называет то, что форку решать самому.
func TestTemplateUpdate(t *testing.T) {
	t.Parallel()
	script := templateScript(t)
	tmpl, fork := t.TempDir(), t.TempDir()

	v1 := map[string]string{
		".template/paths":   "# пути шаблона\n.template\nh\n:(exclude)h/own\n",
		".template/version": "template/v1.0.0\n",
		"go.mod":            "module example.com/tmpl\n",
		"h/rule.md":         "r1\nr2\nr3\n",
		"h/check.go":        "package h\n\nimport (\n\t\"example.com/tmpl/x\"\n)\n\nvar A = x.A\n",
		"h/merged.md":       "m1\nm2\nm3\nm4\nm5\nm6\nm7\nm8\n",
		"h/conflict.md":     "c1\nc2\nc3\n",
		"h/dropped.md":      "d1\n",
		"h/removed.md":      "x1\n",
		"h/edited.md":       "e1\n",
		"h/gone.md":         "g1\n",
		"h/own/local.md":    "o1\n",
	}
	writeFiles(t, tmpl, v1)
	git(t, tmpl, "init", "-q")
	commitAll(t, tmpl)
	git(t, tmpl, "tag", "-a", "template/v1.0.0", "-m", "template/v1.0.0\n\nОбвязка: первая версия")
	writeFiles(t, tmpl, map[string]string{
		".template/version": "template/v1.1.0\n",
		"h/rule.md":         "r1\nr2 v2\nr3\n",
		"h/check.go":        "package h\n\nimport (\n\t\"example.com/tmpl/x\"\n\t\"example.com/tmpl/y\"\n)\n\nvar A = x.A + y.B\n",
		"h/merged.md":       "m1\nm2 v2\nm3\nm4\nm5\nm6\nm7\nm8\n",
		"h/conflict.md":     "c1\nc2 template\nc3\n",
		"h/dropped.md":      "d1 v2\n",
		"h/own/local.md":    "o1 v2\n",
		"h/new.md":          "n1\n",
	})
	git(t, tmpl, "rm", "-q", "h/removed.md", "h/edited.md", "h/gone.md")
	commitAll(t, tmpl)
	git(t, tmpl, "tag", "-a", "template/v1.1.0", "-m", "template/v1.1.0\n\nBreaking:\n- h/rule.md → правка форка")

	fork1 := maps.Clone(v1)
	maps.Copy(fork1, map[string]string{
		"go.mod":        "module example.com/fork\n",
		"h/check.go":    "package h\n\nimport (\n\t\"example.com/fork/x\"\n)\n\nvar A = x.A\n",
		"h/merged.md":   "m1\nm2\nm3\nm4\nm5\nm6\nm7 fork\nm8\n",
		"h/conflict.md": "c1\nc2 fork\nc3\n",
		"h/edited.md":   "e1 fork\n",
	})
	delete(fork1, "h/dropped.md")
	delete(fork1, "h/gone.md")
	writeFiles(t, fork, fork1)
	git(t, fork, "init", "-q")
	commitAll(t, fork)
	git(t, fork, "remote", "add", "--no-tags", "template", tmpl)

	status := func() string { return bash(t, fork, script, "--status") }
	require.Contains(t, status(), "template/v1.0.0 · тегов шаблона в репо нет")
	preview := bash(t, fork, script)
	require.Contains(t, preview, "- h/rule.md → правка форка")
	require.NotContains(t, preview, "первая версия")
	require.Contains(t, status(), "template/v1.0.0 · в шаблоне новее: template/v1.1.0")

	out, err := runIn(t, fork, "bash", script, "template/v1.1.0")
	require.Error(t, err, out)
	require.Contains(t, out, "конфликты — разреши и git add:\n  h/conflict.md\n")
	require.Contains(t, out, "нет в форке — не тронуты:\n  h/dropped.md\n")
	require.Contains(t, out, "шаблон удалил, форк менял — реши сам:\n  h/edited.md\n")
	require.NotContains(t, out, "h/gone.md", "deleted by both the template and the fork: nothing to decide")
	require.Equal(t, map[string]string{
		".template/version": "template/v1.1.0\n",
		"h/rule.md":         "r1\nr2 v2\nr3\n",
		"h/check.go":        "package h\n\nimport (\n\t\"example.com/fork/x\"\n\t\"example.com/fork/y\"\n)\n\nvar A = x.A + y.B\n",
		"h/merged.md":       "m1\nm2 v2\nm3\nm4\nm5\nm6\nm7 fork\nm8\n",
		"h/edited.md":       "e1 fork\n",
		"h/own/local.md":    "o1\n",
		"h/new.md":          "n1\n",
	}, readFiles(t, fork, ".template/version", "h/rule.md", "h/check.go", "h/merged.md", "h/edited.md", "h/own/local.md", "h/new.md"))
	require.NoFileExists(t, filepath.Join(fork, "h", "dropped.md"))
	require.NoFileExists(t, filepath.Join(fork, "h", "removed.md"))
	require.Contains(t, git(t, fork, "status", "--porcelain"), "UU h/conflict.md")
	require.Contains(t, status(), "template/v1.1.0 · новее известных тегов шаблона нет")

	// Канон — тег версии в истории и module path шаблона: клон со своим module path — форк, его дрейф виден.
	require.Contains(t, bash(t, tmpl, script, "--status"), "template/v1.1.0 +0 — канон шаблона")
	clone := t.TempDir()
	git(t, clone, "clone", "-q", tmpl, ".")
	git(t, clone, "checkout", "-q", "template/v1.0.0")
	writeFiles(t, clone, map[string]string{"go.mod": "module example.com/clone\n"})
	commitAll(t, clone)
	require.Contains(t, bash(t, clone, script, "--status"), "template/v1.0.0 · в шаблоне новее: template/v1.1.0")
}

// TestTemplateUpdateFirstBase: первый перенос идёт от коммита шаблона, из которого создан форк (дерево
// первого коммита), — форк с кончика main между тегами не получает ложных конфликтов, а форк,
// созданный до .template/, получает её тем же переносом.
func TestTemplateUpdateFirstBase(t *testing.T) {
	t.Parallel()
	script := templateScript(t)
	tmpl := t.TempDir()
	legacy := map[string]string{"go.mod": "module example.com/tmpl\n", "h/l.md": "l0\n"}
	v1 := map[string]string{".template/paths": ".template\nh\n", ".template/version": "template/v1.0.0\n"}
	between := map[string]string{"h/l.md": "l1\n", "h/n.md": "n1\n"}
	git(t, tmpl, "init", "-q")
	for _, files := range []map[string]string{legacy, v1, between} {
		writeFiles(t, tmpl, files)
		commitAll(t, tmpl)
		if files[".template/version"] != "" {
			git(t, tmpl, "tag", "-a", "template/v1.0.0", "-m", "template/v1.0.0")
		}
	}
	writeFiles(t, tmpl, map[string]string{".template/version": "template/v1.1.0\n", "h/l.md": "l2\n", "h/n.md": "n2\n"})
	commitAll(t, tmpl)
	git(t, tmpl, "tag", "-a", "template/v1.1.0", "-m", "template/v1.1.0")

	for name, tc := range map[string]struct {
		root map[string]string
		base string // база в выводе переноса; "" — коммит шаблона по дереву, без имени
	}{
		"between tags":     {root: union(legacy, v1, between)},
		"before .template": {root: legacy},
		"at the tag":       {root: union(legacy, v1), base: "template/v1.0.0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fork := t.TempDir()
			writeFiles(t, fork, tc.root)
			git(t, fork, "init", "-q")
			commitAll(t, fork)
			writeFiles(t, fork, map[string]string{"go.mod": "module example.com/fork\n"})
			commitAll(t, fork)
			git(t, fork, "remote", "add", "--no-tags", "template", tmpl)
			out := bash(t, fork, script, "template/v1.1.0")
			if tc.base != "" {
				require.Contains(t, out, "обвязка: "+tc.base+" → template/v1.1.0")
			}
			require.Equal(t, map[string]string{".template/version": "template/v1.1.0\n", "h/l.md": "l2\n", "h/n.md": "n2\n"},
				readFiles(t, fork, ".template/version", "h/l.md", "h/n.md"))
		})
	}
}

// TestTemplateUpdateBeforeVersionTag: форк, созданный раньше тега своей версии, до этой версии не
// переносится — она бы не сдвинулась, и следующий перенос повторил бы те же правки; правки до тега
// приходят переносом до следующего тега.
func TestTemplateUpdateBeforeVersionTag(t *testing.T) {
	t.Parallel()
	script := templateScript(t)
	tmpl, fork := t.TempDir(), t.TempDir()
	v1 := map[string]string{
		".template/paths": ".template\nh\n", ".template/version": "template/v1.0.0\n",
		"go.mod": "module example.com/tmpl\n", "h/k.md": "k0\n",
	}
	release := map[string]string{".template/version": "template/v1.1.0\n"} // коммит выпуска, тег — позже
	git(t, tmpl, "init", "-q")
	for _, step := range []struct {
		files map[string]string
		tag   string
	}{
		{v1, "template/v1.0.0"},
		{release, ""},
		{map[string]string{"h/k.md": "k1\n"}, "template/v1.1.0"},
		{map[string]string{".template/version": "template/v1.2.0\n", "h/q.md": "q1\n"}, "template/v1.2.0"},
	} {
		writeFiles(t, tmpl, step.files)
		commitAll(t, tmpl)
		if step.tag != "" {
			git(t, tmpl, "tag", "-a", step.tag, "-m", step.tag)
		}
	}

	writeFiles(t, fork, union(v1, release))
	git(t, fork, "init", "-q")
	commitAll(t, fork)
	writeFiles(t, fork, map[string]string{"go.mod": "module example.com/fork\n"})
	commitAll(t, fork)
	git(t, fork, "remote", "add", "--no-tags", "template", tmpl)

	preview := bash(t, fork, script)
	require.NotContains(t, preview, "  template/v1.1.0\n", "the preview offers tags newer than the fork version only")
	require.Contains(t, preview, "  template/v1.2.0\n")
	out, err := runIn(t, fork, "bash", script, "template/v1.1.0")
	require.Error(t, err, out)
	require.Contains(t, out, "TO=template/v1.1.0 не новее версии форка template/v1.1.0")
	require.Empty(t, git(t, fork, "status", "--porcelain"), "a refused update leaves the tree as is")

	bash(t, fork, script, "template/v1.2.0")
	require.Equal(t, map[string]string{".template/version": "template/v1.2.0\n", "h/k.md": "k1\n", "h/q.md": "q1\n"},
		readFiles(t, fork, ".template/version", "h/k.md", "h/q.md"))
}

func templateScript(t *testing.T) string {
	t.Helper()
	script := filepath.Join(repoRoot(t), "scripts", "tools", "template-update.sh")
	require.FileExists(t, script) // скрипт читает bash: без этого кеш go test не видит его правок
	return script
}

// runIn — команда в dir без GIT_* окружения вызывающего (хук git задаёт GIT_DIR своего репо) и без
// глобального конфига git: подпись коммитов и хуки пользователя тест не видят.
func runIn(t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=archtest", "GIT_AUTHOR_EMAIL=archtest@example.com",
		"GIT_COMMITTER_NAME=archtest", "GIT_COMMITTER_EMAIL=archtest@example.com")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runIn(t, dir, "git", args...)
	require.NoError(t, err, "git %v: %s", args, out)
	return out
}

func commitAll(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "commit")
}

func bash(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runIn(t, dir, "bash", args...)
	require.NoError(t, err, "bash %v: %s", args, out)
	return out
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
}

func union(sets ...map[string]string) map[string]string {
	all := map[string]string{}
	for _, set := range sets {
		maps.Copy(all, set)
	}
	return all
}

func readFiles(t *testing.T, dir string, names ...string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		files[name] = string(data)
	}
	return files
}

// conflictMarkers — файл:строка первого маркера конфликта в каждом обычном файле: submodule и symlink
// в git ls-files — пути, а не текст репо.
func conflictMarkers(t *testing.T, root string, files []string) []string {
	t.Helper()
	var found []string
	for _, f := range files {
		path := filepath.Join(root, f)
		info, err := os.Lstat(path)
		require.NoError(t, err)
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		if loc := conflictMarker.FindIndex(data); loc != nil {
			found = append(found, f+":"+strconv.Itoa(bytes.Count(data[:loc[0]], []byte("\n"))+1))
		}
	}
	return found
}
