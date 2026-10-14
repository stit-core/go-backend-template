package postgres

import (
	"context"
	"fmt"
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

func (r *Repository) UpsertPasswordReset(ctx context.Context, id, userID, tokenHash string, requestedAt, expiresAt time.Time) (bool, error) {
	rid, err := pgkit.ParseUUID(id)
	if err != nil {
		return false, fmt.Errorf("verification repo: invalid password reset id: %w", err)
	}
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return false, fmt.Errorf("verification repo: invalid user id: %w", err)
	}
	_, err = r.q.UpsertPasswordReset(ctx, db.UpsertPasswordResetParams{
		ID:          rid,
		UserID:      uid,
		TokenHash:   tokenHash,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		RequestedAt: pgtype.Timestamptz{Time: requestedAt, Valid: true},
	})
	if pgkit.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verification repo: upsert password reset: %w", err)
	}
	return true, nil
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
	return used(r.q.MarkPasswordResetUsed(ctx, uid))
}

func (r *Repository) RevokePasswordResets(ctx context.Context, userID string) error {
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("verification repo: invalid user id: %w", err)
	}
	if err := r.q.RevokePasswordResets(ctx, uid); err != nil {
		return fmt.Errorf("verification repo: revoke password resets: %w", err)
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
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return verification.ErrInvalidVerificationToken
	}
	return used(r.q.MarkEmailVerificationUsed(ctx, uid))
}

// used — итог условного погашения: ни одной строки — токен уже погашен, в том числе параллельным
// запросом, который успел раньше.
func used(rows int64, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("verification repo: mark token used: %w", err)
	case rows == 0:
		return verification.ErrTokenUsed
	}
	return nil
}
