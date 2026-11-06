package outcome_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
)

func TestResult_String(t *testing.T) {
	t.Parallel()
	// Исходы шаблона — значения метки result: их имена держит этот тест, а не домены, которые их ставят (их снимает prune).
	require.Equal(t, "ok", outcome.OK.String())
	require.Equal(t, "error", outcome.Error.String())
	require.Equal(t, "invalid", outcome.Invalid.String())
	require.Equal(t, "expired", outcome.Expired.String())
	require.Equal(t, "duplicate", outcome.New("duplicate").String())
}
