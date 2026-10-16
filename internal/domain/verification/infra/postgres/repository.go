package postgres

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/postgres/db"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type Repository struct {
	q *db.Queries
}

func NewRepository(database *pgkit.DB) verification.Repository {
	return &Repository{q: db.New(database)}
}

func (r *Repository) UpsertPasswordReset(ctx context.Context, issue verification.PasswordResetIssue, pause time.Duration) (bool, error) {
	rid, err := pgkit.ParseUUID(issue.RequestID)
	if err != nil {
		return false, fmt.Errorf("verification repo: invalid password reset id: %w", err)
	}
	uid, err := pgkit.ParseUUID(issue.UserID)
	if err != nil {
		return false, fmt.Errorf("verification repo: invalid user id: %w", err)
	}
	_, err = r.q.UpsertPasswordReset(ctx, db.UpsertPasswordResetParams{
		ID:          rid,
		UserID:      uid,
		TokenHash:   issue.TokenHash,
		ExpiresAt:   pgtype.Timestamptz{Time: issue.ExpiresAt, Valid: true},
		UserVersion: int32(issue.UserVersion),
		RequestedAt: pgtype.Timestamptz{Time: issue.RequestedAt, Valid: true},
		MailPause:   pgtype.Interval{Microseconds: pause.Microseconds(), Valid: true},
	})
	if pgkit.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verification repo: upsert password reset: %w", err)
	}
	return true, nil
}

func (r *Repository) TryLockPasswordResets(ctx context.Context, userID string) (bool, error) {
	locked, err := r.q.TryLockPasswordResets(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("verification repo: lock password resets: %w", err)
	}
	return locked, nil
}

func (r *Repository) MarkPasswordResetSent(ctx context.Context, id string) error {
	rid, err := pgkit.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("verification repo: invalid password reset id: %w", err)
	}
	if err := r.q.MarkPasswordResetSent(ctx, rid); err != nil {
		return fmt.Errorf("verification repo: mark password reset sent: %w", err)
	}
	return nil
}

func (r *Repository) GetPasswordReset(ctx context.Context, tokenHash string) (*verification.PasswordResetToken, error) {
	row, err := r.q.GetPasswordReset(ctx, tokenHash)
	if pgkit.IsNoRows(err) {
		return nil, verification.ErrInvalidResetToken
	}
	if err != nil {
		return nil, fmt.Errorf("verification repo: get password reset: %w", err)
	}
	pr := &verification.PasswordResetToken{
		ID:        pgkit.UUIDString(row.ID),
		UserID:    pgkit.UUIDString(row.UserID),
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    pgkit.NullableTime(row.UsedAt),
	}
	if row.UserVersion != nil {
		pr.UserVersion = int(*row.UserVersion)
	}
	return pr, nil
}

func (r *Repository) RedeemPasswordReset(ctx context.Context, id, userID string) error {
	rid, err := pgkit.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("verification repo: invalid password reset id: %w", err)
	}
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("verification repo: invalid user id: %w", err)
	}
	redeemed, err := r.q.RedeemPasswordResets(ctx, uid)
	if err != nil {
		return fmt.Errorf("verification repo: redeem password reset: %w", err)
	}
	if !slices.Contains(redeemed, rid) {
		return verification.ErrTokenUsed
	}
	return nil
}

func (r *Repository) UpsertEmailVerification(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (string, bool, error) {
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return "", false, fmt.Errorf("verification repo: invalid user id: %w", err)
	}
	id, err := r.q.UpsertEmailVerification(ctx, db.UpsertEmailVerificationParams{
		UserID:    uid,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if pgkit.IsNoRows(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("verification repo: upsert email verification: %w", err)
	}
	return pgkit.UUIDString(id), true, nil
}

func (r *Repository) MarkEmailVerificationSent(ctx context.Context, id string) error {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("verification repo: invalid email verification id: %w", err)
	}
	if err := r.q.MarkEmailVerificationSent(ctx, uid); err != nil {
		return fmt.Errorf("verification repo: mark email verification sent: %w", err)
	}
	return nil
}

func (r *Repository) GetEmailVerification(ctx context.Context, tokenHash string) (*verification.EmailVerificationToken, error) {
	row, err := r.q.GetEmailVerification(ctx, tokenHash)
	if pgkit.IsNoRows(err) {
		return nil, verification.ErrInvalidVerificationToken
	}
	if err != nil {
		return nil, fmt.Errorf("verification repo: get email verification: %w", err)
	}
	return &verification.EmailVerificationToken{
		ID:        pgkit.UUIDString(row.ID),
		UserID:    pgkit.UUIDString(row.UserID),
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    pgkit.NullableTime(row.UsedAt),
	}, nil
}

func (r *Repository) MarkEmailVerificationUsed(ctx context.Context, id string) error {
	vid, err := pgkit.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("verification repo: invalid email verification id: %w", err)
	}
	rows, err := r.q.MarkEmailVerificationUsed(ctx, vid)
	switch {
	case err != nil:
		return fmt.Errorf("verification repo: mark email verification used: %w", err)
	case rows == 0:
		return verification.ErrTokenUsed // погашена раньше, в том числе параллельным запросом
	}
	return nil
}

func (r *Repository) DeleteExpiredLinks(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.q.DeleteExpiredLinks(ctx, pgtype.Timestamptz{Time: before, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("verification repo: delete expired links: %w", err)
	}
	return n, nil
}
