package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"git.stit.tech/stit-infra/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-infra/go-backend-template/internal/domain/auth/infra/postgres/db"
	"git.stit.tech/stit-infra/golibinfra/pgkit"
)

type Repository struct {
	q *db.Queries
}

func NewRepository(database *pgkit.DB) auth.SessionRepository {
	return &Repository{q: db.New(database)}
}

func (r *Repository) Insert(ctx context.Context, jti, userID string, expiresAt time.Time) error {
	jtiUUID, err := pgkit.ParseUUID(jti)
	if err != nil {
		return fmt.Errorf("auth repo: invalid jti: %w", err)
	}
	uid, err := pgkit.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("auth repo: invalid user_id: %w", err)
	}
	if err := r.q.InsertRefreshSession(ctx, db.InsertRefreshSessionParams{
		Jti:       jtiUUID,
		UserID:    uid,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("auth repo: insert refresh session: %w", err)
	}
	return nil
}

func (r *Repository) GetByJTI(ctx context.Context, jti string) (*auth.RefreshSession, error) {
	jtiUUID, err := pgkit.ParseUUID(jti)
	if err != nil {
		return nil, auth.ErrSessionNotFound
	}
	row, err := r.q.GetRefreshSession(ctx, jtiUUID)
	if pgkit.IsNoRows(err) {
		return nil, auth.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth repo: get refresh session: %w", err)
	}
	return &auth.RefreshSession{
		JTI:           pgkit.UUIDString(row.Jti),
		UserID:        pgkit.UUIDString(row.UserID),
		IssuedAt:      row.IssuedAt.Time,
		ExpiresAt:     row.ExpiresAt.Time,
		UsedAt:        pgkit.NullableTime(row.UsedAt),
		ReplacedByJTI: pgkit.UUIDString(row.ReplacedByJti),
	}, nil
}

func (r *Repository) MarkUsed(ctx context.Context, jti, replacedByJTI string) error {
	jtiUUID, err := pgkit.ParseUUID(jti)
	if err != nil {
		return auth.ErrSessionNotFound
	}
	var replacedBy pgtype.UUID
	if replacedByJTI != "" {
		replacedBy, err = pgkit.ParseUUID(replacedByJTI)
		if err != nil {
			return fmt.Errorf("auth repo: invalid replaced_by_jti: %w", err)
		}
	}
	if err := r.q.MarkRefreshSessionUsed(ctx, db.MarkRefreshSessionUsedParams{
		Jti:           jtiUUID,
		ReplacedByJti: replacedBy,
	}); err != nil {
		return fmt.Errorf("auth repo: mark refresh session used: %w", err)
	}
	return nil
}
