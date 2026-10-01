package outcome_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
)

func TestResult_String(t *testing.T) {
	t.Parallel()
	require.Equal(t, "expired", outcome.Expired.String())
	require.Equal(t, "duplicate", outcome.New("duplicate").String())
}
