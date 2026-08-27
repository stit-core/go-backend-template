// White-box (package nats): the unit is the unexported subscription callback
// onUserRegistered — the exported path (Register) needs a live JetStream bus
// (rule tests-pipeline: white-box package allowed for private functions).
package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats/mocks"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

func testEvent() UserRegisteredEvent {
	return UserRegisteredEvent{
		ID:    "3f1d2c4b-5a6e-4f70-8b91-0c2d3e4f5a6b",
		Email: "user@example.com",
		At:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestOnUserRegistered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ucErr error
	}{
		{name: "valid payload issues verification", ucErr: nil},
		{name: "usecase error propagates for redelivery", ucErr: errors.New("issue failed")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := testEvent()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().IssueEmailVerification(mock.Anything, e.ID, e.Email).Return(tc.ucErr).Once()

			h := NewHandler(uc, logkit.Nop())
			err := h.onUserRegistered(context.Background(), e)

			if tc.ucErr != nil {
				require.ErrorIs(t, err, tc.ucErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOnUserRegistered_DuplicateDelivery(t *testing.T) {
	t.Parallel()

	e := testEvent()
	uc := mocks.NewMockUsecase(t)
	// Dedup lives in the usecase (ON CONFLICT no-op): on redelivery the
	// handler must call it again, not short-circuit.
	uc.EXPECT().IssueEmailVerification(mock.Anything, e.ID, e.Email).Return(nil).Times(2)

	h := NewHandler(uc, logkit.Nop())
	require.NoError(t, h.onUserRegistered(context.Background(), e))
	require.NoError(t, h.onUserRegistered(context.Background(), e))
}
