package verification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-infra/golibinfra/clockkit"
	"git.stit.tech/stit-infra/golibinfra/logkit"
	"git.stit.tech/stit-infra/golibinfra/testkit/authntest"
	"git.stit.tech/stit-infra/golibinfra/testkit/eventbus"
	"git.stit.tech/stit-infra/golibinfra/testkit/txkit"
	"git.stit.tech/stit-template/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-template/go-backend-template/internal/domain/verification/mocks"
)

type ucFixture struct {
	uc       *verification.Usecase
	repo     *mocks.MockRepository
	users    *mocks.MockUserReader
	pwSet    *mocks.MockPasswordSetter
	emailSet *mocks.MockEmailVerificationSetter
	mailer   *mocks.MockMailer
	bus      *eventbus.Recorder
}

type ucOpts struct {
	repo     *mocks.MockRepository
	users    *mocks.MockUserReader
	pwSet    *mocks.MockPasswordSetter
	emailSet *mocks.MockEmailVerificationSetter
	mailer   *mocks.MockMailer
}

type ucOption func(*ucOpts)

func withRepo(r *mocks.MockRepository) ucOption  { return func(o *ucOpts) { o.repo = r } }
func withUsers(u *mocks.MockUserReader) ucOption { return func(o *ucOpts) { o.users = u } }

func withPwSet(p *mocks.MockPasswordSetter) ucOption { return func(o *ucOpts) { o.pwSet = p } }

func withEmailSet(e *mocks.MockEmailVerificationSetter) ucOption {
	return func(o *ucOpts) { o.emailSet = e }
}

func withMailer(m *mocks.MockMailer) ucOption { return func(o *ucOpts) { o.mailer = m } }

func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{}
	for _, fn := range opts {
		fn(o)
	}
	if o.repo == nil {
		r := mocks.NewMockRepository(t)
		r.EXPECT().InsertPasswordReset(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		r.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(nil, verification.ErrInvalidResetToken).Maybe()
		r.EXPECT().MarkPasswordResetUsed(mock.Anything, mock.Anything).Return(nil).Maybe()
		r.EXPECT().InsertEmailVerification(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		r.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(nil, verification.ErrInvalidVerificationToken).Maybe()
		r.EXPECT().MarkEmailVerificationUsed(mock.Anything, mock.Anything).Return(nil).Maybe()
		o.repo = r
	}
	if o.users == nil {
		u := mocks.NewMockUserReader(t)
		u.EXPECT().GetByID(mock.Anything, mock.Anything).Return(nil, verification.ErrUserNotFound).Maybe()
		u.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(nil, verification.ErrUserNotFound).Maybe()
		o.users = u
	}
	if o.pwSet == nil {
		p := mocks.NewMockPasswordSetter(t)
		p.EXPECT().UpdatePassword(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		o.pwSet = p
	}
	if o.emailSet == nil {
		e := mocks.NewMockEmailVerificationSetter(t)
		e.EXPECT().MarkEmailVerified(mock.Anything, mock.Anything).Return(nil).Maybe()
		o.emailSet = e
	}
	if o.mailer == nil {
		m := mocks.NewMockMailer(t)
		m.EXPECT().SendPasswordReset(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		m.EXPECT().SendEmailVerification(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		o.mailer = m
	}

	bus := &eventbus.Recorder{}
	uc := verification.NewUsecase(verification.UsecaseDeps{
		Repo:          o.repo,
		Users:         o.users,
		PasswordSet:   o.pwSet,
		EmailVerifier: o.emailSet,
		Hasher:        authntest.DeterministicHasher{},
		Tx:            txkit.Identity{},
		Events:        bus,
		Mailer:        o.mailer,
		Log:           logkit.Nop(),
		Clock:         clockkit.NewFixed(time.Now()),
	})
	return ucFixture{
		uc: uc, repo: o.repo, users: o.users, pwSet: o.pwSet, emailSet: o.emailSet, mailer: o.mailer, bus: bus,
	}
}

func TestIssueEmailVerification_HappyPath(t *testing.T) {
	t.Parallel()
	var sentEmail, sentToken string
	mailer := mocks.NewMockMailer(t)
	mailer.EXPECT().SendEmailVerification(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, email, token string) error {
			sentEmail = email
			sentToken = token
			return nil
		})
	f := newUC(t, withMailer(mailer))

	require.NoError(t, f.uc.IssueEmailVerification(context.Background(), "u1", "a@b.com"))
	require.Equal(t, 1, f.bus.Count())

	ev, ok := f.bus.Last("user.email_verification.requested").(verification.EmailVerificationRequested)
	if !ok {
		ev, ok = f.bus.All()[0].Payload.(verification.EmailVerificationRequested)
	}
	require.True(t, ok, "expected EmailVerificationRequested event")
	require.NotEmpty(t, ev.TokenID)
	require.Equal(t, "a@b.com", sentEmail)
	require.NotEmpty(t, sentToken)
	require.NotEqual(t, ev.TokenID, sentToken, "TokenID must not equal plaintext token")
}

func TestRequestPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(
		&verification.User{ID: "u1", Email: "a@b.com", Active: true}, nil,
	)
	repo := mocks.NewMockRepository(t)
	var capturedHash string
	repo.EXPECT().InsertPasswordReset(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _, h string, _ time.Time) error {
			capturedHash = h
			return nil
		})
	var sentEmail, sentToken string
	mailer := mocks.NewMockMailer(t)
	mailer.EXPECT().SendPasswordReset(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, email, token string) error {
			sentEmail = email
			sentToken = token
			return nil
		})
	f := newUC(t, withRepo(repo), withUsers(users), withMailer(mailer))

	require.NoError(t, f.uc.RequestPasswordReset(context.Background(), "a@b.com"))
	require.Len(t, capturedHash, 64)
	require.Equal(t, "a@b.com", sentEmail)
	require.NotEmpty(t, sentToken)

	ev, ok := f.bus.All()[0].Payload.(verification.PasswordResetRequested)
	require.True(t, ok)
	require.NotEmpty(t, ev.TokenID)
	require.Equal(t, "u1", ev.UserID)
	require.Equal(t, "a@b.com", ev.Email)
	require.NotEqual(t, ev.TokenID, sentToken)
}

func TestRequestPasswordReset_UnknownEmail(t *testing.T) {
	t.Parallel()
	f := newUC(t)
	require.NoError(t, f.uc.RequestPasswordReset(context.Background(), "x@y.com"), "anti-enumeration: must not error")
}

func TestConfirmPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}, nil,
	)
	repo.EXPECT().MarkPasswordResetUsed(mock.Anything, mock.Anything).Return(nil).Maybe()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, mock.Anything).Return(
		&verification.User{ID: "u1", Email: "a@b.com", Active: true, Version: 3}, nil,
	)
	pwSet := mocks.NewMockPasswordSetter(t)
	var lastHash string
	var lastVersion int
	pwSet.EXPECT().UpdatePassword(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _, hash string, version int) error {
			lastHash = hash
			lastVersion = version
			return nil
		})
	f := newUC(t, withRepo(repo), withUsers(users), withPwSet(pwSet))

	require.NoError(t, f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123"))
	require.Equal(t, "hash:newpassword123", lastHash)
	require.Equal(t, 3, lastVersion)
}

func TestConfirmPasswordReset_WeakPassword(t *testing.T) {
	t.Parallel()
	f := newUC(t)
	err := f.uc.ConfirmPasswordReset(context.Background(), "tok", "short")
	require.ErrorIs(t, err, verification.ErrWeakPassword)
}

func TestConfirmPasswordReset_Expired(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: time.Now().Add(-time.Hour)}, nil,
	)
	f := newUC(t, withRepo(repo))
	err := f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken)
}

func TestConfirmEmail_HappyPath(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}, nil,
	)
	repo.EXPECT().MarkEmailVerificationUsed(mock.Anything, mock.Anything).Return(nil).Maybe()
	emailSet := mocks.NewMockEmailVerificationSetter(t)
	var markedID string
	emailSet.EXPECT().MarkEmailVerified(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, userID string) error {
			markedID = userID
			return nil
		})
	f := newUC(t, withRepo(repo), withEmailSet(emailSet))

	require.NoError(t, f.uc.ConfirmEmail(context.Background(), "tok"))
	require.Equal(t, "u1", markedID)
}

func TestConfirmEmail_Expired(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: time.Now().Add(-time.Hour)}, nil,
	)
	f := newUC(t, withRepo(repo))
	err := f.uc.ConfirmEmail(context.Background(), "tok")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
}
