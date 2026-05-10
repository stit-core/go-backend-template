package worker

import (
	"context"

	"git.stit.tech/stit/golibinfra/pgkit"
)

const cleanupBruteforceSQL = `
DELETE FROM bruteforce_attempts
WHERE (locked_until IS NULL OR locked_until < now())
  AND last_attempt < now() - INTERVAL '24 hours'
`

func Cleanup(ctx context.Context, db *pgkit.DB) error {
	_, err := db.Exec(ctx, cleanupBruteforceSQL)
	return err
}
