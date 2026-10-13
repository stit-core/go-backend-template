package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReleaseImage: путь образа — <host>/<org>/<репо> из origin: у сервисов одной орги свои пути, и релиз
// второго не перезаписывает образ первого. Confirm-промпт называет путь целиком, handoff — имя сервиса для
// хаба (<SVC>_TAG, make bump SVC=<репо>); origin, из которого путь не вывести, — отказ до тега. Шаги гейта,
// preflight и docker здесь — заглушки (newReleaseSandbox): docker записывает свои аргументы.
func TestReleaseImage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		origin string
		image  string // "" — отказ
		svc    string
		tagVar string
	}{
		{origin: "git@git.example.org:acme/own.git", image: "git.example.org/acme/own", svc: "own", tagVar: "OWN_TAG"},
		{origin: "https://git.example.org/acme/ext", image: "git.example.org/acme/ext", svc: "ext", tagVar: "EXT_TAG"},
		{origin: "git@git.example.org:acme/backend.git", image: "git.example.org/acme/backend", svc: "backend", tagVar: "BACKEND_TAG"},
		{
			origin: "ssh://git@git.example.org:2222/acme/Billing-API.git/", image: "git.example.org/acme/billing-api",
			svc: "billing-api", tagVar: "BILLING_API_TAG",
		},
		{origin: "/srv/git/own.git"},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			t.Parallel()
			s := newReleaseSandbox(t, "true", map[string]string{"scripts/tools/e2e.sh": "exit 0\n"})
			git(t, s.repo, "remote", "set-url", "origin", tc.origin)
			require.NoError(t, os.WriteFile(filepath.Join(s.bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$0.args\"\n"), 0o700))
			govulncheck, err := exec.LookPath("true")
			require.NoError(t, err)
			require.NoError(t, os.Symlink(govulncheck, filepath.Join(s.bin, "govulncheck")))

			prompt, err := s.release(t, "RELEASE_YES=")
			require.Error(t, err, "no answer to the prompt cancels the release:\n%s", prompt)
			if tc.image == "" {
				require.Contains(t, prompt, "release: путь образа берётся из origin")
				require.NotContains(t, prompt, "confirm release")
				require.Empty(t, git(t, s.repo, "tag", "-l"), "a release without an image path leaves no tag")
				return
			}
			require.Contains(t, prompt, "⚠ confirm release v9.9.9 from main@")
			require.Contains(t, prompt, " → "+tc.image+"{,-migrate}:v9.9.9? [y/N]", "the prompt names the full path of the images")

			out, err := s.release(t, "GIT_COMMITTER_NAME=archtest", "GIT_COMMITTER_EMAIL=archtest@example.com")
			require.NoError(t, err, out)
			args := readFiles(t, s.bin, "docker.args")["docker.args"]
			require.Contains(t, args, " -t "+tc.image+":v9.9.9 .\n", "the api image goes to the path of the repo")
			require.Contains(t, args, " -t "+tc.image+"-migrate:v9.9.9 .\n", "the migrate image goes next to it")
			require.Contains(t, out, tc.tagVar+" → v9.9.9")
			require.Contains(t, out, "make bump SVC="+tc.svc+" TAG=v9.9.9")
		})
	}
}
