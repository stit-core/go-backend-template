package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/verification"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/verification/infra/postgres/db"
	"git.sftbmb.ru/sftbmb-core/golibinfra/pgkit"
)

type Repository struct {
	q *db.Queries
}

func NewRepository(database *pgkit.DB) verification.Repository {
	return &Repository{q: db.New(database)}
}

func (r *Repository) InsertPasswordReset(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return verification.ErrUserNotFound
	}
	if err := r.q.InsertPasswordReset(ctx, db.InsertPasswordResetParams{
		UserID:    uid,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("verification repo: insert password reset: %w", err)
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
	return &verification.PasswordResetToken{
		ID:        pgkit.UUIDString(row.ID),
		UserID:    pgkit.UUIDString(row.UserID),
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    pgkit.NullableTime(row.UsedAt),
	}, nil
}

func (r *Repository) MarkPasswordResetUsed(ctx context.Context, id string) error {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return verification.ErrInvalidResetToken
	}
	if err := r.q.MarkPasswordResetUsed(ctx, uid); err != nil {
		return fmt.Errorf("verification repo: mark password reset used: %w", err)
	}
	return nil
}

func (r *Repository) InsertEmailVerification(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return verification.ErrUserNotFound
	}
	if err := r.q.InsertEmailVerification(ctx, db.InsertEmailVerificationParams{
		UserID:    uid,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("verification repo: insert email verification: %w", err)
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
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return verification.ErrInvalidVerificationToken
	}
	if err := r.q.MarkEmailVerificationUsed(ctx, uid); err != nil {
		return fmt.Errorf("verification repo: mark email verification used: %w", err)
	}
	return nil
}
