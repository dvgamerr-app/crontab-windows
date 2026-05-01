package app

import (
	"context"

	"github.com/pkg/errors"
)

// Run launches the cron scheduler and blocks until ctx is canceled.
func Run(ctx context.Context, svcName, sha1ver string, opts Options) error {
	s, err := setup(svcName, sha1ver)
	if err != nil {
		return errors.Wrap(err, "setup")
	}

	_ = s

	return RunScheduler(ctx, opts)
}
