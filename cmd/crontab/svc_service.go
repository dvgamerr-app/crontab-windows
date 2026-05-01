package main

import (
	"context"
	"time"

	"github.com/dvgamerr-app/crontab-windows/app"
	"github.com/gookit/slog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
)

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- svcLauncher(ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for {
		select {
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				slog.Error(errors.Wrap(err, "svcLauncher").Error())
			}
			break loop
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				cancel()
				select {
				case err := <-done:
					if err != nil {
						slog.Error(errors.Wrap(err, "svcLauncher").Error())
					}
				case <-time.After(10 * time.Second):
					slog.Warn("timeout waiting for scheduler shutdown")
				}
				break loop
			default:
				slog.Errorf("unexpected control request #%d", c)
			}
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}

func runService(name string, isDebug bool) {
	opts := serviceOptionsFromArgs()
	logger, err := app.NewLogger(opts)
	if err != nil {
		return
	}
	defer logger.Close()
	slog.Configure(func(sl *slog.SugaredLogger) {
		sl.Logger = logger
	})

	slog.Infof("%s: starting", name)
	run := svc.Run
	if isDebug {
		run = svc.Run
	}
	err = run(name, &myservice{})
	if err != nil {
		slog.Errorf("%s service failed: %v", name, err)
		return
	}
	slog.Infof("%s: stopped", name)
}
