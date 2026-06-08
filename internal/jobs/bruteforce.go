package jobs

import (
	"context"
	"time"

	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/timerkit"
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
