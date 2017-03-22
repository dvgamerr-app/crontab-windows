package app

import (
	"fmt"
	"log"
)

// if setup returns an error, the service doesn't start
func setup(svcName, sha1ver string) (server, error) {
	var s server

	// did we get a full SHA1?
	if len(sha1ver) == 40 {
		sha1ver = sha1ver[0:7]
	}

	if sha1ver == "" {
		sha1ver = "dev"
	}

	// Note: any logging here goes to Windows App Log
	// I suggest you setup local logging
	log.Println(1, fmt.Sprintf("%s: setup (%s)", svcName, sha1ver))

	// read configuration
	// configure more logging

	return s, nil
}
