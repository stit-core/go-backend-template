package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReleaseImage: путь образа — <host>/<org>/<репо> из origin: у сервисов одной орги свои пути, и релиз
// второго не перезаписывает образ первого. Confirm-промпт называет путь целиком, handoff — имя сервиса для
// хаба (<SVC>_TAG так, как его строит make bump хаба, make bump SVC=<репо>). Origin, из которого путь не
// вывести, и имя репо, из которого хаб не выведет переменную тега, — отказ до тега; токен из https-origin
// в вывод не попадает. make doctor ищет логин registry по хосту того же пути. Шаги гейта, preflight и
// docker здесь — заглушки (newReleaseSandbox): docker записывает свои аргументы.
func TestReleaseImage(t *testing.T) {
	t.Parallel()
	doctor := readFiles(t, repoRoot(t), "scripts/tools/doctor.sh")
	for _, tc := range []struct {
		origin  string
		image   string // путь образа; "" — из origin не выводится
		svc     string
		tagVar  string
		refusal string // причина отказа релиза; "" — релиз идёт
	}{
		{origin: "git@git.example.org:acme/own.git", image: "git.example.org/acme/own", svc: "own", tagVar: "OWN_TAG"},
		{origin: "https://git.example.org/acme/ext", image: "git.example.org/acme/ext", svc: "ext", tagVar: "EXT_TAG"},
		{origin: "git@git.example.org:acme/backend.git", image: "git.example.org/acme/backend", svc: "backend", tagVar: "BACKEND_TAG"},
		{
			origin: "ssh://git@git.example.org:2222/acme/Billing-API.git/", image: "git.example.org/acme/billing-api",
			svc: "billing-api", tagVar: "BILLING_API_TAG",
		},
		{origin: "https://ci:s3cr3t@Git.Example.org:3000/acme/own.git", image: "git.example.org:3000/acme/own", svc: "own", tagVar: "OWN_TAG"},
		{origin: "/srv/git/own.git", refusal: "release: путь образа берётся из origin"},
		{origin: "https://ci:s3cr3t@git.example.org/group/sub/own.git", refusal: "а origin: https://git.example.org/group/sub/own.git"},
		{origin: "git@git.example.org:acme/1c-gateway.git", image: "git.example.org/acme/1c-gateway", refusal: "make bump ищет тег в 1C_GATEWAY_TAG"},
		{origin: "git@git.example.org:acme/my.service.git", image: "git.example.org/acme/my.service", refusal: "make bump ищет тег в MY.SERVICE_TAG"},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			t.Parallel()
			s := newReleaseSandbox(t, "true", union(doctor, map[string]string{"scripts/tools/e2e.sh": "exit 0\n"}))
			git(t, s.repo, "remote", "set-url", "origin", tc.origin)
			require.NoError(t, os.WriteFile(filepath.Join(s.bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$0.args\"\n"), 0o700))
			govulncheck, err := exec.LookPath("true")
			require.NoError(t, err)
			require.NoError(t, os.Symlink(govulncheck, filepath.Join(s.bin, "govulncheck")))

			prompt, err := s.release(t, "RELEASE_YES=")
			require.Error(t, err, "no answer to the prompt cancels the release:\n%s", prompt)
			require.NotContains(t, prompt, "s3cr3t", "the userinfo of the origin stays out of the output")
			if tc.refusal != "" {
				require.Contains(t, prompt, tc.refusal)
				require.NotContains(t, prompt, "confirm release")
				require.Empty(t, git(t, s.repo, "tag", "-l"), "a refused release leaves no tag")
			} else {
				require.Contains(t, prompt, "⚠ confirm release v9.9.9 from main@")
				require.Contains(t, prompt, " → "+tc.image+"{,-migrate}:v9.9.9? [y/N]", "the prompt names the full path of the images")

				out, err := s.release(t, "GIT_COMMITTER_NAME=archtest", "GIT_COMMITTER_EMAIL=archtest@example.com")
				require.NoError(t, err, out)
				args := readFiles(t, s.bin, "docker.args")["docker.args"]
				require.Contains(t, args, " -t "+tc.image+":v9.9.9 .\n", "the api image goes to the path of the repo")
				require.Contains(t, args, " -t "+tc.image+"-migrate:v9.9.9 .\n", "the migrate image goes next to it")
				require.Contains(t, out, tc.tagVar+" → v9.9.9")
				require.Contains(t, out, "make bump SVC="+tc.svc+" TAG=v9.9.9")
				require.NotContains(t, out, "s3cr3t")
			}

			config := t.TempDir()
			host, _, _ := strings.Cut(tc.image, "/")
			writeFiles(t, config, map[string]string{"config.json": `{"auths":{"` + host + `":{}}}`})
			out, _ := runEnv(t, s.repo, []string{"PATH=" + s.bin + string(os.PathListSeparator) + os.Getenv("PATH"), "DOCKER_CONFIG=" + config},
				"bash", "scripts/tools/doctor.sh")
			require.NotContains(t, out, "s3cr3t")
			if tc.image == "" {
				require.Contains(t, out, "WARN : docker: путь образа из origin не выводится")
				return
			}
			require.Contains(t, out, "OK   : docker auth: креды для "+host+" присутствуют", "doctor looks for the login of the registry of the image")
		})
	}
}
