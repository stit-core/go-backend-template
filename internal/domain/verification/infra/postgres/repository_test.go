//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	verifpg "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
	"git.stit.tech/stit-core/golibinfra/idkit"
	"git.stit.tech/stit-core/golibinfra/testkit"
)

// requestedAt — запрос сброса тестов: его ссылка живёт час, до tokenExpiry.
var (
	tokenExpiry = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	requestedAt = tokenExpiry.Add(-time.Hour)
)

// noPause — выпуск без паузы писем: тесты, которым она не важна, выпускают несколько ссылок пользователю подряд.
const noPause = time.Duration(0)

// setup seeds a user because both token tables reference users(id).
func setup(t *testing.T) (verification.Repository, string) {
	t.Helper()
	fx := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	db := fx.DB(t)
	u := pgseed.InsertUser(t, userpg.NewRepository(db))
	return verifpg.NewRepository(db), u.ID
}

// issueReset выпускает новую ссылку сброса пользователя userID с токеном hash и отдаёт id запроса.
func issueReset(t *testing.T, repo verification.Repository, userID, hash string) string {
	t.Helper()
	id := idkit.NewV7()
	issued, err := repo.UpsertPasswordReset(context.Background(), resetIssue(id, userID, hash), noPause)
	require.NoError(t, err)
	require.True(t, issued)
	return id
}

// resetIssue — выпуск ссылки id пользователя userID версии 1 по запросу в requestedAt.
func resetIssue(id, userID, hash string) verification.PasswordResetIssue {
	return verification.PasswordResetIssue{
		RequestID: id, UserID: userID, UserVersion: 1, TokenHash: hash, RequestedAt: requestedAt, ExpiresAt: tokenExpiry,
	}
}

// Ссылка сброса — по id запроса: её находят по хэшу токена, id ссылки — id запроса из события.
func TestUpsertPasswordResetAndGet(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)

	id := issueReset(t, repo, userID, "hash:pr-1")

	got, err := repo.GetPasswordReset(context.Background(), "hash:pr-1")
	require.NoError(t, err)
	require.Equal(t, id, got.ID, "the link is stored under the id of the request")
	require.Equal(t, userID, got.UserID)
	require.Equal(t, 1, got.UserVersion, "the link keeps the version of the account at its mail")
	require.WithinDuration(t, tokenExpiry, got.ExpiresAt, time.Second)
	require.Nil(t, got.UsedAt)
}

// Повтор доставки запроса до отправки письма выпускает новый токен той же ссылки, после отправки или
// погашения — ссылку не трогает.
func TestUpsertPasswordReset_Redelivery(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()
	id := issueReset(t, repo, userID, "hash:pr-1")

	again := resetIssue(id, userID, "hash:pr-2")
	again.UserVersion = 2
	issued, err := repo.UpsertPasswordReset(ctx, again, noPause)
	require.NoError(t, err)
	require.True(t, issued, "an unsent link is issued again")
	_, err = repo.GetPasswordReset(ctx, "hash:pr-1")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken, "the unsent token is replaced")
	got, err := repo.GetPasswordReset(ctx, "hash:pr-2")
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Equal(t, 2, got.UserVersion, "the reissued link keeps the version of the account at its own mail")

	require.NoError(t, repo.MarkPasswordResetSent(ctx, id))
	issued, err = repo.UpsertPasswordReset(ctx, resetIssue(id, userID, "hash:pr-3"), noPause)
	require.NoError(t, err)
	require.False(t, issued, "a sent link is not issued again")
	_, err = repo.GetPasswordReset(ctx, "hash:pr-3")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken)
	_, err = repo.GetPasswordReset(ctx, "hash:pr-2")
	require.NoError(t, err, "the sent token stays valid")

	unsent := issueReset(t, repo, userID, "hash:pr-4")
	require.NoError(t, repo.RedeemPasswordReset(ctx, unsent, userID))
	issued, err = repo.UpsertPasswordReset(ctx, resetIssue(unsent, userID, "hash:pr-5"), noPause)
	require.NoError(t, err)
	require.False(t, issued, "a used link is not issued again")
}

// Пока у пользователя есть ссылка моложе паузы писем, новый запрос ссылки не получает — с каких бы адресов ни шли
// запросы; повтор доставки своего запроса и соседа пауза не задевает, а после неё ссылка выпускается.
func TestUpsertPasswordReset_MailPause(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	db := pg.DB(t)
	users := userpg.NewRepository(db)
	owner, neighbour := pgseed.InsertUser(t, users), pgseed.InsertUser(t, users)
	repo := verifpg.NewRepository(db)
	ctx := context.Background()
	const pause = 5 * time.Minute
	first := idkit.NewV7()
	issue := func(id, userID, hash string) bool {
		t.Helper()
		issued, err := repo.UpsertPasswordReset(ctx, resetIssue(id, userID, hash), pause)
		require.NoError(t, err)
		return issued
	}

	require.True(t, issue(first, owner.ID, "hash:pr-1"))
	require.False(t, issue(idkit.NewV7(), owner.ID, "hash:pr-2"), "a new request within the pause gets no link")
	require.True(t, issue(first, owner.ID, "hash:pr-3"), "the redelivery of the issued request is not paused")
	require.True(t, issue(idkit.NewV7(), neighbour.ID, "hash:pr-n"), "the pause is per account")

	_, err := pg.Pool(t).Exec(ctx, `UPDATE password_resets SET created_at = created_at - interval '6 minutes' WHERE user_id = $1`, owner.ID)
	require.NoError(t, err)
	require.True(t, issue(idkit.NewV7(), owner.ID, "hash:pr-4"), "after the pause a request gets its link")
}

// Job удаляет ссылки сброса и подтверждения адреса, истёкшие раньше before; живые остаются.
func TestDeleteExpiredLinks(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()
	stale := issueReset(t, repo, userID, "hash:pr-stale")
	fresh := resetIssue(idkit.NewV7(), userID, "hash:pr-fresh")
	fresh.ExpiresAt = tokenExpiry.Add(time.Hour)
	issued, err := repo.UpsertPasswordReset(ctx, fresh, noPause)
	require.NoError(t, err)
	require.True(t, issued)
	_, _, err = repo.UpsertEmailVerification(ctx, userID, "hash:ev-stale", tokenExpiry)
	require.NoError(t, err)

	n, err := repo.DeleteExpiredLinks(ctx, tokenExpiry.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	_, err = repo.GetPasswordReset(ctx, "hash:pr-stale")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken, "the expired link %s is deleted", stale)
	_, err = repo.GetEmailVerification(ctx, "hash:ev-stale")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
	_, err = repo.GetPasswordReset(ctx, "hash:pr-fresh")
	require.NoError(t, err, "the live link stays")
}

// Отметка «письмо ушло» — только своей ссылке: неотправленная ссылка другого запроса при повторе доставки
// выпускается снова, а не числится отправленной без письма.
func TestMarkPasswordResetSent_OwnLinkOnly(t *testing.T) {
	t.Parallel()
	db := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations())).DB(t)
	users := userpg.NewRepository(db)
	first, second := pgseed.InsertUser(t, users), pgseed.InsertUser(t, users)
	repo := verifpg.NewRepository(db)
	ctx := context.Background()
	sent, unsent := issueReset(t, repo, first.ID, "hash:pr-1"), issueReset(t, repo, second.ID, "hash:pr-2")

	require.NoError(t, repo.MarkPasswordResetSent(ctx, sent))

	issued, err := repo.UpsertPasswordReset(ctx, resetIssue(unsent, second.ID, "hash:pr-3"), noPause)
	require.NoError(t, err)
	require.True(t, issued, "the link of another request stays unsent: its redelivery mails it")
	issued, err = repo.UpsertPasswordReset(ctx, resetIssue(sent, first.ID, "hash:pr-4"), noPause)
	require.NoError(t, err)
	require.False(t, issued, "the marked link is sent")
}

// Ссылка запроса — его пользователя: событие с id чужой неотправленной ссылки её токен не перевыпускает.
func TestUpsertPasswordReset_OtherUser(t *testing.T) {
	t.Parallel()
	db := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations())).DB(t)
	users := userpg.NewRepository(db)
	owner, other := pgseed.InsertUser(t, users), pgseed.InsertUser(t, users)
	repo := verifpg.NewRepository(db)
	ctx := context.Background()
	id := issueReset(t, repo, owner.ID, "hash:pr-1")

	issued, err := repo.UpsertPasswordReset(ctx, resetIssue(id, other.ID, "hash:pr-2"), noPause)
	require.NoError(t, err)
	require.False(t, issued, "the link of another user is not issued")
	got, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)
	require.Equal(t, owner.ID, got.UserID, "the token of the owner stays")
}

// Подтверждение гасит и запрос в полёте: запрос, после которого пользователь погасил ссылку сброса, ссылки не
// получает, а запрос после погашения и запрос соседа — получают.
func TestUpsertPasswordReset_AfterRedemption(t *testing.T) {
	t.Parallel()
	db := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations())).DB(t)
	users := userpg.NewRepository(db)
	owner, neighbour := pgseed.InsertUser(t, users), pgseed.InsertUser(t, users)
	repo := verifpg.NewRepository(db)
	ctx := context.Background()
	require.NoError(t, repo.RedeemPasswordReset(ctx, issueReset(t, repo, owner.ID, "hash:pr-1"), owner.ID))
	redeemed, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)
	before, after := redeemed.UsedAt.Add(-time.Second), redeemed.UsedAt.Add(time.Second)

	for _, tc := range []struct {
		name, userID, hash string
		requestedAt        time.Time
		issued             bool
	}{
		{"request before the redemption", owner.ID, "hash:pr-2", before, false},
		{"request after the redemption", owner.ID, "hash:pr-3", after, true},
		{"request of the neighbour", neighbour.ID, "hash:pr-n", before, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue := resetIssue(idkit.NewV7(), tc.userID, tc.hash)
			issue.RequestedAt = tc.requestedAt
			issued, err := repo.UpsertPasswordReset(ctx, issue, noPause)
			require.NoError(t, err)
			require.Equal(t, tc.issued, issued)
			_, err = repo.GetPasswordReset(ctx, tc.hash)
			require.Equal(t, tc.issued, err == nil, "the link is stored only when issued")
		})
	}
}

func TestGetPasswordReset_UnknownTokenHash(t *testing.T) {
	t.Parallel()
	repo, _ := setup(t)

	_, err := repo.GetPasswordReset(context.Background(), "hash:absent")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken)
}

// Погашение ссылки гасит все непогашенные ссылки её пользователя одним оператором; ссылки соседа целы. Ссылка,
// погашенная раньше (своим или соседним погашением), — ErrTokenUsed.
func TestRedeemPasswordReset(t *testing.T) {
	t.Parallel()
	db := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations())).DB(t)
	users := userpg.NewRepository(db)
	owner, neighbour := pgseed.InsertUser(t, users), pgseed.InsertUser(t, users)
	repo := verifpg.NewRepository(db)
	ctx := context.Background()
	first, second := issueReset(t, repo, owner.ID, "hash:pr-1"), issueReset(t, repo, owner.ID, "hash:pr-2")
	issueReset(t, repo, neighbour.ID, "hash:pr-n")

	require.NoError(t, repo.RedeemPasswordReset(ctx, first, owner.ID))

	for hash, used := range map[string]bool{"hash:pr-1": true, "hash:pr-2": true, "hash:pr-n": false} {
		got, err := repo.GetPasswordReset(ctx, hash)
		require.NoError(t, err)
		require.Equal(t, used, got.UsedAt != nil, hash)
	}
	require.ErrorIs(t, repo.RedeemPasswordReset(ctx, first, owner.ID), verification.ErrTokenUsed)
	require.ErrorIs(t, repo.RedeemPasswordReset(ctx, second, owner.ID), verification.ErrTokenUsed, "a link redeemed with its neighbour")
}

// Две ссылки одного пользователя гасят одновременно: блокировки строк — в одном порядке, одно погашение
// успешно, второе — ErrTokenUsed, взаимной блокировки (40P01) нет.
func TestRedeemPasswordReset_TwoLinksAtOnce(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	repo := verifpg.NewRepository(pg.DB(t))
	tx := pg.Transactor(t)
	ctx := context.Background()
	userID := pgseed.InsertUser(t, userpg.NewRepository(pg.DB(t))).ID
	links := []string{issueReset(t, repo, userID, "hash:pr-1"), issueReset(t, repo, userID, "hash:pr-2")}

	errs := make(chan error, len(links))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, id := range links {
		wg.Go(func() {
			<-start
			errs <- tx.WithTx(ctx, func(txCtx context.Context) error {
				if err := repo.RedeemPasswordReset(txCtx, id, userID); err != nil {
					return err
				}
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		})
	}
	close(start)
	wg.Wait()
	close(errs)

	var redeemed, used int
	for err := range errs {
		switch {
		case err == nil:
			redeemed++
		case errors.Is(err, verification.ErrTokenUsed):
			used++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, redeemed)
	require.Equal(t, 1, used)
}

// Ожидающее подтверждение пользователя одно: повтор доставки до отправки письма перевыпускает
// токен, после отправки — не трогает.
func TestUpsertEmailVerification_Redelivery(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()

	id, issued, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev-1", tokenExpiry)
	require.NoError(t, err)
	require.True(t, issued)

	reissued, issued, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev-2", tokenExpiry.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, issued, "an unsent verification is issued again")
	require.Equal(t, id, reissued, "the same pending verification")
	_, err = repo.GetEmailVerification(ctx, "hash:ev-1")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken, "the unsent token is replaced")
	got, err := repo.GetEmailVerification(ctx, "hash:ev-2")
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.WithinDuration(t, tokenExpiry.Add(time.Hour), got.ExpiresAt, time.Second)

	require.NoError(t, repo.MarkEmailVerificationSent(ctx, id))
	_, issued, err = repo.UpsertEmailVerification(ctx, userID, "hash:ev-3", tokenExpiry)
	require.NoError(t, err)
	require.False(t, issued, "a sent verification is not issued again")
	_, err = repo.GetEmailVerification(ctx, "hash:ev-3")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
	_, err = repo.GetEmailVerification(ctx, "hash:ev-2")
	require.NoError(t, err, "the sent token stays valid")

	require.NoError(t, repo.MarkEmailVerificationUsed(ctx, id))
	require.ErrorIs(t, repo.MarkEmailVerificationUsed(ctx, id), verification.ErrTokenUsed)
	used, err := repo.GetEmailVerification(ctx, "hash:ev-2")
	require.NoError(t, err)
	require.Equal(t, userID, used.UserID)
	require.NotNil(t, used.UsedAt)
	next, issued, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev-4", tokenExpiry)
	require.NoError(t, err)
	require.True(t, issued, "the key is the pending verification only: a replay after the confirmation is stopped by the subscriber, the address is verified")
	require.NotEqual(t, id, next)
}

// Одновременные погашения одного токена: ровно одно успешное, остальные — ErrTokenUsed. Каждое — в
// своей транзакции, как в usecase; первая держит блокировку строки, остальные ждут её.
func TestMarkUsed_Concurrent(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	repo := verifpg.NewRepository(pg.DB(t))
	tx := pg.Transactor(t)
	ctx := context.Background()
	userID := pgseed.InsertUser(t, userpg.NewRepository(pg.DB(t))).ID

	resetID := issueReset(t, repo, userID, "hash:pr")
	verifyID, _, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev", tokenExpiry)
	require.NoError(t, err)

	tests := []struct {
		name  string
		claim func(ctx context.Context) error
	}{
		{"password reset", func(ctx context.Context) error { return repo.RedeemPasswordReset(ctx, resetID, userID) }},
		{"email verification", func(ctx context.Context) error { return repo.MarkEmailVerificationUsed(ctx, verifyID) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const n = 8
			errs := make(chan error, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range n {
				wg.Go(func() {
					<-start
					errs <- tx.WithTx(ctx, func(txCtx context.Context) error {
						if err := tc.claim(txCtx); err != nil {
							return err
						}
						time.Sleep(50 * time.Millisecond)
						return nil
					})
				})
			}
			close(start)
			wg.Wait()
			close(errs)

			var claimed, used int
			for err := range errs {
				switch {
				case err == nil:
					claimed++
				case errors.Is(err, verification.ErrTokenUsed):
					used++
				default:
					require.NoError(t, err)
				}
			}
			require.Equal(t, 1, claimed, "exactly one claim succeeds")
			require.Equal(t, n-1, used)
		})
	}
}

func TestGetEmailVerification_UnknownTokenHash(t *testing.T) {
	t.Parallel()
	repo, _ := setup(t)

	_, err := repo.GetEmailVerification(context.Background(), "hash:absent")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
}

// Отказы, до SQL не дошедшие или пришедшие от драйвера. Id пользователя и строк приходят из БД и событий: битый —
// ошибка, а не тихий no-op и не sentinel. Сбой БД — ошибка с причиной: база не выглядит негодной ссылкой.
func TestRepository_Failures(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	sentinels := []error{verification.ErrInvalidResetToken, verification.ErrInvalidVerificationToken, verification.ErrTokenUsed}

	t.Run("malformed id", func(t *testing.T) {
		ctx := context.Background()
		calls := map[string]func() error{
			"UpsertPasswordReset bad user": func() error {
				_, err := repo.UpsertPasswordReset(ctx, resetIssue(idkit.NewV7(), "not-a-uuid", "hash:bad-user"), noPause)
				return err
			},
			"UpsertPasswordReset bad request": func() error {
				_, err := repo.UpsertPasswordReset(ctx, resetIssue("not-a-uuid", userID, "hash:bad-request"), noPause)
				return err
			},
			"MarkPasswordResetSent":        func() error { return repo.MarkPasswordResetSent(ctx, "not-a-uuid") },
			"RedeemPasswordReset bad link": func() error { return repo.RedeemPasswordReset(ctx, "not-a-uuid", userID) },
			"RedeemPasswordReset bad user": func() error { return repo.RedeemPasswordReset(ctx, idkit.NewV7(), "not-a-uuid") },
			"UpsertEmailVerification": func() error {
				_, _, err := repo.UpsertEmailVerification(ctx, "not-a-uuid", "hash:bad-user", tokenExpiry)
				return err
			},
			"MarkEmailVerificationSent": func() error { return repo.MarkEmailVerificationSent(ctx, "not-a-uuid") },
			"MarkEmailVerificationUsed": func() error { return repo.MarkEmailVerificationUsed(ctx, "not-a-uuid") },
		}
		for name, call := range calls {
			err := call()
			require.Error(t, err, name)
			for _, sentinel := range sentinels {
				require.NotErrorIs(t, err, sentinel, name)
			}
		}
	})

	t.Run("database failure keeps its cause", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		const id = "00000000-0000-0000-0000-000000000001"
		calls := map[string]func() error{
			"UpsertPasswordReset": func() error {
				_, err := repo.UpsertPasswordReset(ctx, resetIssue(id, userID, "hash:canceled"), noPause)
				return err
			},
			"MarkPasswordResetSent": func() error { return repo.MarkPasswordResetSent(ctx, id) },
			"GetPasswordReset":      func() error { _, err := repo.GetPasswordReset(ctx, "hash:canceled"); return err },
			"RedeemPasswordReset":   func() error { return repo.RedeemPasswordReset(ctx, id, userID) },
			"DeleteExpiredLinks":    func() error { _, err := repo.DeleteExpiredLinks(ctx, tokenExpiry); return err },
			"UpsertEmailVerification": func() error {
				_, _, err := repo.UpsertEmailVerification(ctx, userID, "hash:canceled", tokenExpiry)
				return err
			},
			"MarkEmailVerificationSent": func() error { return repo.MarkEmailVerificationSent(ctx, id) },
			"GetEmailVerification":      func() error { _, err := repo.GetEmailVerification(ctx, "hash:canceled"); return err },
			"MarkEmailVerificationUsed": func() error { return repo.MarkEmailVerificationUsed(ctx, id) },
		}
		for name, call := range calls {
			err := call()
			require.ErrorIs(t, err, context.Canceled, name)
			for _, sentinel := range sentinels {
				require.NotErrorIs(t, err, sentinel, name)
			}
		}
	})
}
