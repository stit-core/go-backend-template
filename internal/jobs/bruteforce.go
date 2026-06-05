package jobs

import (
	"context"
	"time"

	"git.sftbmb.ru/sftbmb-core/golibinfra/bruteforcekit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/timerkit"
)

func NewBruteforceCleanup(g bruteforcekit.Guard) timerkit.Job {
	return timerkit.Job{
		Name:     "bruteforce.cleanup",
		Interval: 5 * time.Minute,
		Run: func(ctx context.Context) error {
			return g.Cleanup(ctx)
		},
	}
}
