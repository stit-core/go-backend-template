package tokens_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/pkg/tokens"
)

func TestGenerate_ProducesURLSafeToken(t *testing.T) {
	t.Parallel()

	got, err := tokens.Generate()

	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Regexp(t, "^[0-9a-f]{64}$", got, "token must be lowercase hex, safe for URLs and cookies")
}

func TestGenerate_UniquePerCall(t *testing.T) {
	t.Parallel()

	first, err := tokens.Generate()
	require.NoError(t, err)

	second, err := tokens.Generate()
	require.NoError(t, err)

	require.NotEqual(t, first, second)
}

func TestHash(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		token string
		want  string
	}{
		{
			name:  "empty input",
			token: "",
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:  "known sha256 vector",
			token: "abc",
			want:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tokens.Hash(tc.token)

			require.Equal(t, tc.want, got)
			require.Len(t, got, 64)
			require.Equal(t, got, tokens.Hash(tc.token), "hash must be deterministic")
		})
	}
}

func TestHash_DiffersForDifferentInputs(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, tokens.Hash("token-a"), tokens.Hash("token-b"))
}
