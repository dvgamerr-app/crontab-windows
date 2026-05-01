package app

import (
	"context"

	"github.com/pkg/errors"
)

// Run launches the cron scheduler and blocks until ctx is canceled.
func Run(ctx context.Context, svcName, sha1ver string, opts Options) error {
	logger, err := NewLogger(opts)
	if err != nil {
		return errors.Wrap(err, "new logger")
	}
	defer logger.Close()

	s, err := setup(svcName, sha1ver, logger)
	if err != nil {
		return errors.Wrap(err, "setup")
	}

	_ = s

	return runScheduler(ctx, opts, logger)
}
