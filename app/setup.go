package app

import "github.com/gookit/slog"

// if setup returns an error, the service doesn't start
func setup(svcName, sha1ver string, logger *slog.Logger) (server, error) {
	var s server

	// did we get a full SHA1?
	if len(sha1ver) == 40 {
		sha1ver = sha1ver[0:7]
	}

	if sha1ver == "" {
		sha1ver = "dev"
	}

	if logger != nil {
		logger.Infof("%s: setup (%s)", svcName, sha1ver)
	}

	// read configuration
	// configure more logging

	return s, nil
}
