//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/golibinfra/ptrkit"
	"git.stit.tech/stit-core/golibinfra/testkit"
)

func setup(t *testing.T) user.Repository {
	t.Helper()
	fx := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	return userpg.NewRepository(fx.DB(t))
}

// Адрес уникален без учёта регистра среди не удалённых: удалённый его освобождает.
func TestInsert(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo, pgseed.WithEmail("Alice@Example.com"))
	require.NotEmpty(t, u.ID)
	require.Equal(t, 1, u.Version)
	require.False(t, u.CreatedAt.IsZero())
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u, got, "Insert fills the user as it is stored")

	for _, email := range []string{"Alice@Example.com", "alice@example.com"} {
		require.ErrorIs(t, repo.Insert(ctx, pgseed.NewUser(pgseed.WithEmail(email))), user.ErrAlreadyExists, email)
	}
	require.NoError(t, repo.Delete(ctx, u.ID))
	again := pgseed.InsertUser(t, repo, pgseed.WithEmail("alice@example.com"))
	require.NotEqual(t, u.ID, again.ID)
}

func TestGetByID(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo, pgseed.WithRole(identity.RoleAdmin))
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u, got)

	// id приходит из пути запроса: чужой формат — тот же «нет такого», а не сбой БД.
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		_, err = repo.GetByID(ctx, id)
		require.ErrorIs(t, err, user.ErrNotFound, id)
	}
}

// Чтение под блокировкой видит того же пользователя, что GetByID, и до конца своей транзакции держит его
// строку: чужая запись в неё ждёт (здесь — упирается в lock_timeout), после коммита — проходит.
func TestGetByIDForShare(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	repo := userpg.NewRepository(pg.DB(t))
	ctx := context.Background()
	u := pgseed.InsertUser(t, repo)
	deleted := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.Delete(ctx, deleted.ID))

	require.NoError(t, pg.Transactor(t).WithTx(ctx, func(txCtx context.Context) error {
		got, err := repo.GetByIDForShare(txCtx, u.ID)
		require.NoError(t, err)
		require.Equal(t, u, got)

		writer, err := pg.Pool(t).Begin(ctx)
		require.NoError(t, err)
		defer func() { require.NoError(t, writer.Rollback(ctx)) }()
		_, err = writer.Exec(ctx, `SET LOCAL lock_timeout = '100ms'`)
		require.NoError(t, err)
		_, err = writer.Exec(ctx, `UPDATE users SET name = 'Other' WHERE id = $1`, u.ID)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		require.Equal(t, "55P03", pgErr.Code, "a write into the locked row waits for the reading transaction")
		return nil
	}))
	_, err := repo.UpdatePassword(ctx, u.ID, "hash:new", u.Version)
	require.NoError(t, err, "the row is free once the reading transaction ends")

	for _, id := range []string{deleted.ID, "00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		_, err := repo.GetByIDForShare(ctx, id)
		require.ErrorIs(t, err, user.ErrNotFound, id)
	}
}

// Адрес ищется без учёта регистра — как его держит уникальный индекс.
func TestGetByEmail(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo, pgseed.WithEmail("Alice@Example.com"))
	for _, email := range []string{"Alice@Example.com", "alice@example.com", "ALICE@EXAMPLE.COM"} {
		got, err := repo.GetByEmail(ctx, email)
		require.NoError(t, err)
		require.Equal(t, u, got, email)
	}
	_, err := repo.GetByEmail(ctx, "bob@example.com")
	require.ErrorIs(t, err, user.ErrNotFound)
}

// Список и счётчик видят одних и тех же — не удалённых, с фильтром статуса; список — новые первыми.
func TestList(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	client := pgseed.InsertUser(t, repo, pgseed.WithRole(identity.RoleClient))
	admin := pgseed.InsertUser(t, repo, pgseed.WithRole(identity.RoleAdmin))
	blocked := pgseed.InsertUser(t, repo, pgseed.WithStatus(user.StatusBlocked))
	require.NoError(t, repo.Delete(ctx, pgseed.InsertUser(t, repo).ID))

	tests := []struct {
		name   string
		filter user.ListFilter
		want   []*user.User
	}{
		{"all but deleted", user.ListFilter{}, []*user.User{blocked, admin, client}},
		{"active", user.ListFilter{Status: ptrkit.Of(user.StatusActive)}, []*user.User{admin, client}},
		{"blocked", user.ListFilter{Status: ptrkit.Of(user.StatusBlocked)}, []*user.User{blocked}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			list, err := repo.List(ctx, tc.filter, paging.Page{Number: 1, Size: paging.MaxSize})
			require.NoError(t, err)
			require.Equal(t, tc.want, list)
			n, err := repo.Count(ctx, tc.filter)
			require.NoError(t, err)
			require.Equal(t, len(tc.want), n)
		})
	}

	t.Run("pages", func(t *testing.T) {
		first, err := repo.List(ctx, user.ListFilter{}, paging.Page{Number: 1, Size: 2})
		require.NoError(t, err)
		require.Equal(t, []*user.User{blocked, admin}, first)
		second, err := repo.List(ctx, user.ListFilter{}, paging.Page{Number: 2, Size: 2})
		require.NoError(t, err)
		require.Equal(t, []*user.User{client}, second)
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	u.Name, u.Status = "Renamed", user.StatusBlocked
	got, err := repo.Update(ctx, u)
	require.NoError(t, err)
	require.Equal(t, u.Version+1, got.Version)

	stored, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "Renamed", stored.Name, "name and status are written together")
	require.Equal(t, user.StatusBlocked, stored.Status, "name and status are written together")
	require.Equal(t, got.Version, stored.Version)
}

func TestUpdate_StaleVersion(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	updates := []struct {
		name   string
		update func(u *user.User) (*user.User, error)
	}{
		{"name and status", func(u *user.User) (*user.User, error) {
			return repo.Update(ctx, &user.User{ID: u.ID, Name: "renamed", Status: user.StatusBlocked, Version: u.Version})
		}},
		{"password", func(u *user.User) (*user.User, error) {
			return repo.UpdatePassword(ctx, u.ID, "new-hash", u.Version)
		}},
	}
	for _, tc := range updates {
		t.Run(tc.name, func(t *testing.T) {
			u := pgseed.InsertUser(t, repo)

			got, err := tc.update(u)
			require.NoError(t, err)
			require.Equal(t, u.Version+1, got.Version)

			_, err = tc.update(u)
			require.ErrorIs(t, err, user.ErrConcurrentUpdate)
		})
	}
}

func TestUpdatePassword(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	got, err := repo.UpdatePassword(ctx, u.ID, "hash:new", u.Version)
	require.NoError(t, err)
	require.Equal(t, u.Version+1, got.Version)

	stored, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "hash:new", stored.PasswordHash)
	require.Equal(t, got.Version, stored.Version)
	require.Equal(t, u.Name, stored.Name, "only the password changes")
}

// Удаление без версии: повтор — ErrNotFound, из одновременных удалений проходит ровно одно.
func TestDelete(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.Delete(ctx, u.ID))
	_, err := repo.GetByID(ctx, u.ID)
	require.ErrorIs(t, err, user.ErrNotFound)
	require.ErrorIs(t, repo.Delete(ctx, u.ID), user.ErrNotFound)
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		require.ErrorIs(t, repo.Delete(ctx, id), user.ErrNotFound, id)
	}

	raced := pgseed.InsertUser(t, repo)
	const n = 8
	errs := make(chan error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			<-start
			errs <- repo.Delete(ctx, raced.ID)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	deleted := 0
	for err := range errs {
		if err == nil {
			deleted++
			continue
		}
		require.ErrorIs(t, err, user.ErrNotFound)
	}
	require.Equal(t, 1, deleted, "exactly one of the concurrent deletes deletes")
}

func TestSetEmailVerified(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.SetEmailVerified(ctx, u.ID))
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, got.EmailVerified)

	require.ErrorIs(t, repo.SetEmailVerified(ctx, "00000000-0000-0000-0000-000000000000"), user.ErrNotFound)
}

// Удалённый пользователь снаружи не существует: чтение его не находит, запись не трогает.
func TestDeletedUserIsUntouched(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	repo := userpg.NewRepository(pg.DB(t))
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.Delete(ctx, u.ID))
	deletedVersion := u.Version + 1

	_, err := repo.GetByID(ctx, u.ID)
	require.ErrorIs(t, err, user.ErrNotFound)
	_, err = repo.GetByEmail(ctx, u.Email)
	require.ErrorIs(t, err, user.ErrNotFound)
	list, err := repo.List(ctx, user.ListFilter{}, paging.Page{Number: 1, Size: paging.MaxSize})
	require.NoError(t, err)
	require.Empty(t, list)

	_, err = repo.Update(ctx, &user.User{ID: u.ID, Name: "revived", Status: user.StatusActive, Version: deletedVersion})
	require.ErrorIs(t, err, user.ErrConcurrentUpdate)
	_, err = repo.UpdatePassword(ctx, u.ID, "new-hash", deletedVersion)
	require.ErrorIs(t, err, user.ErrConcurrentUpdate)
	require.ErrorIs(t, repo.SetEmailVerified(ctx, u.ID), user.ErrNotFound)

	var (
		name, status, hash string
		version            int
		verified           bool
	)
	require.NoError(t, pg.Pool(t).QueryRow(ctx,
		`SELECT name, status, password_hash, version, email_verified FROM users WHERE id = $1`, u.ID).
		Scan(&name, &status, &hash, &version, &verified))
	require.Equal(t, []any{u.Name, "deleted", u.PasswordHash, deletedVersion, false},
		[]any{name, status, hash, version, verified})
}

// Отказы, до SQL не дошедшие или пришедшие от драйвера: битый id записи — тот же «нет такого», что у
// несуществующего, а сбой БД — ошибка с причиной, не sentinel домена (недоступная база не выглядит как «нет
// такого», «уже есть» или устаревшая версия: API ответит 500, а не 404 или 409).
func TestRepository_Failures(t *testing.T) {
	t.Parallel()
	repo := setup(t)

	t.Run("malformed id is not found", func(t *testing.T) {
		ctx := context.Background()
		_, err := repo.Update(ctx, &user.User{ID: "not-a-uuid", Name: "x", Status: user.StatusActive, Version: 1})
		require.ErrorIs(t, err, user.ErrNotFound, "Update")
		_, err = repo.UpdatePassword(ctx, "not-a-uuid", "hash", 1)
		require.ErrorIs(t, err, user.ErrNotFound, "UpdatePassword")
		require.ErrorIs(t, repo.SetEmailVerified(ctx, "not-a-uuid"), user.ErrNotFound, "SetEmailVerified")
	})

	t.Run("database failure keeps its cause", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		const id = "00000000-0000-0000-0000-000000000001"
		calls := map[string]func() error{
			"Insert":          func() error { return repo.Insert(ctx, pgseed.NewUser()) },
			"GetByID":         func() error { _, err := repo.GetByID(ctx, id); return err },
			"GetByIDForShare": func() error { _, err := repo.GetByIDForShare(ctx, id); return err },
			"GetByEmail":      func() error { _, err := repo.GetByEmail(ctx, "a@b.com"); return err },
			"Update": func() error {
				_, err := repo.Update(ctx, &user.User{ID: id, Name: "x", Status: user.StatusActive, Version: 1})
				return err
			},
			"Delete":           func() error { return repo.Delete(ctx, id) },
			"UpdatePassword":   func() error { _, err := repo.UpdatePassword(ctx, id, "hash", 1); return err },
			"SetEmailVerified": func() error { return repo.SetEmailVerified(ctx, id) },
			"List": func() error {
				_, err := repo.List(ctx, user.ListFilter{}, paging.Page{Number: 1, Size: paging.DefaultSize})
				return err
			},
			"Count": func() error { _, err := repo.Count(ctx, user.ListFilter{}); return err },
		}
		for name, call := range calls {
			err := call()
			require.ErrorIs(t, err, context.Canceled, name)
			for _, sentinel := range []error{user.ErrNotFound, user.ErrAlreadyExists, user.ErrConcurrentUpdate} {
				require.NotErrorIs(t, err, sentinel, name)
			}
		}
	})
}
