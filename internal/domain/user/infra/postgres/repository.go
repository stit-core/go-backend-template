package postgres

import (
	"context"
	"fmt"
	"strings"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres/db"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type Repository struct {
	q *db.Queries
}

func NewRepository(database *pgkit.DB) user.Repository {
	return &Repository{q: db.New(database)}
}

func (r *Repository) Insert(ctx context.Context, u *user.User) error {
	row, err := r.q.InsertUser(ctx, db.InsertUserParams{
		Email:         u.Email,
		PasswordHash:  u.PasswordHash,
		Name:          u.Name,
		Status:        string(u.Status),
		Role:          string(u.Role),
		EmailVerified: u.EmailVerified,
	})
	if pgkit.IsUniqueViolation(err) {
		return user.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("user repo: insert: %w", err)
	}
	u.ID = pgkit.UUIDString(row.ID)
	u.Version = int(row.Version)
	u.EmailVerified = row.EmailVerified
	u.CreatedAt = row.CreatedAt.Time
	u.UpdatedAt = row.UpdatedAt.Time
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*user.User, error) {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return nil, user.ErrNotFound
	}
	row, err := r.q.GetUserByID(ctx, uid)
	if pgkit.IsNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: select: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) GetByIDForShare(ctx context.Context, id string) (*user.User, error) {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return nil, user.ErrNotFound
	}
	row, err := r.q.GetUserByIDForShare(ctx, uid)
	if pgkit.IsNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: select for share: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	// NUL Postgres в TEXT не хранит: такого адреса нет, запрос упал бы ошибкой драйвера.
	if strings.ContainsRune(email, 0) {
		return nil, user.ErrNotFound
	}
	row, err := r.q.GetUserByEmail(ctx, email)
	if pgkit.IsNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: select by email: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) GetByEmailForUpdate(ctx context.Context, email string) (*user.User, error) {
	if strings.ContainsRune(email, 0) {
		return nil, user.ErrNotFound
	}
	row, err := r.q.GetUserByEmailForUpdate(ctx, email)
	if pgkit.IsNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: select by email for update: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) Update(ctx context.Context, u *user.User) (*user.User, error) {
	uid, err := pgkit.ParseUUID(u.ID)
	if err != nil {
		return nil, user.ErrNotFound
	}
	row, err := r.q.UpdateUser(ctx, db.UpdateUserParams{
		Name:    u.Name,
		Status:  string(u.Status),
		ID:      uid,
		Version: int32(u.Version),
	})
	if pgkit.IsNoRows(err) {
		return nil, user.ErrConcurrentUpdate
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: update: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return user.ErrNotFound
	}
	rows, err := r.q.DeleteUser(ctx, uid)
	if err != nil {
		return fmt.Errorf("user repo: delete: %w", err)
	}
	if rows == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (r *Repository) UpdatePassword(ctx context.Context, id, passwordHash string, version int) (*user.User, error) {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return nil, user.ErrNotFound
	}
	row, err := r.q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		PasswordHash: passwordHash,
		ID:           uid,
		Version:      int32(version),
	})
	if pgkit.IsNoRows(err) {
		return nil, user.ErrConcurrentUpdate
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: update password: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) Promote(ctx context.Context, id, passwordHash string, version int) (*user.User, error) {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return nil, user.ErrNotFound
	}
	row, err := r.q.PromoteUser(ctx, db.PromoteUserParams{PasswordHash: passwordHash, ID: uid, Version: int32(version)})
	if pgkit.IsNoRows(err) {
		return nil, user.ErrConcurrentUpdate
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: promote: %w", err)
	}
	return rowToEntity(row), nil
}

func (r *Repository) SetEmailVerified(ctx context.Context, id string) error {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return user.ErrNotFound
	}
	rows, err := r.q.SetUserEmailVerified(ctx, uid)
	if err != nil {
		return fmt.Errorf("user repo: set email verified: %w", err)
	}
	if rows == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter user.ListFilter, page paging.Page) ([]*user.User, error) {
	rows, err := r.q.ListUsers(ctx, db.ListUsersParams{
		Limit:  int32(page.Size),
		Offset: int32(page.Offset()),
		Status: statusFilter(filter.Status),
	})
	if err != nil {
		return nil, fmt.Errorf("user repo: list: %w", err)
	}
	out := make([]*user.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, &user.User{
			ID:            pgkit.UUIDString(row.ID),
			Email:         row.Email,
			Name:          row.Name,
			Status:        user.Status(row.Status),
			Role:          identityRole(row.Role),
			Version:       int(row.Version),
			EmailVerified: row.EmailVerified,
			CreatedAt:     row.CreatedAt.Time,
			UpdatedAt:     row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *Repository) Count(ctx context.Context, filter user.ListFilter) (int, error) {
	n, err := r.q.CountUsers(ctx, statusFilter(filter.Status))
	if err != nil {
		return 0, fmt.Errorf("user repo: count: %w", err)
	}
	return int(n), nil
}

func rowToEntity(row db.User) *user.User {
	return &user.User{
		ID:            pgkit.UUIDString(row.ID),
		Email:         row.Email,
		PasswordHash:  row.PasswordHash,
		Name:          row.Name,
		Status:        user.Status(row.Status),
		Role:          identityRole(row.Role),
		Version:       int(row.Version),
		EmailVerified: row.EmailVerified,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func statusFilter(s *user.Status) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}
