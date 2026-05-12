package jobs

import (
	"context"
	"time"

	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/timerkit"
)

const cleanupBruteforceSQL = `
DELETE FROM bruteforce_attempts
WHERE (locked_until IS NULL OR locked_until < now())
  AND last_attempt < now() - INTERVAL '24 hours'
`

func cleanupBruteforce(ctx context.Context, db *pgkit.DB) error {
	_, err := db.Exec(ctx, cleanupBruteforceSQL)
	return err
}

func NewBruteforceCleanup(db *pgkit.DB) timerkit.Job {
	return timerkit.Job{
		Name:     "bruteforce.cleanup",
		Interval: 5 * time.Minute,
		Run: func(ctx context.Context) error {
			return cleanupBruteforce(ctx, db)
		},
	}
}
