# AGENTS.md

## How I Work In This Repository

This project is a Go Windows service that provides a Linux-style `crontab` command on Windows. When I make changes here, I work from the existing service and scheduler behavior instead of replacing the project shape unnecessarily.

## Working Principles

- Read the current code before changing it. Service behavior, CLI behavior, and scheduler behavior are coupled enough that changes should be made with the existing flow in view.
- Keep changes scoped to the requested behavior. Avoid broad refactors unless they are needed to make the requested behavior reliable.
- Do not revert user changes. The worktree may already contain edits, so treat existing modifications as user-owned unless explicitly told otherwise.
- Prefer Go standard library behavior and the repo's existing patterns before adding new abstractions.
- Use `gofmt` after editing Go files.
- Do not add tests unless the user explicitly approves test writing.

## Current Runtime Model

- The installable binary lives under `cmd/crontab`.
- Running `crontab` with no command should install the Windows service.
- Installing should also attempt to start the service, wait for it to reach `svc.Running`, query the final state, and log whether it is running.
- Service startup runs the cron scheduler until the service is stopped or shut down.
- The scheduler reloads the configured crontab file every minute before running due jobs.
- Jobs run through `cmd.exe /C` so Windows shell commands and built-in behavior work as expected.
- Job environment should include the machine environment so commands such as `curl.exe` can be found through `PATH`.

## Logging

- Use `github.com/gookit/slog` for logging.
- Use the shared logger constructor in `app.NewLogger`.
- Do not add new direct uses of the standard `log` package, Windows `eventlog`, or `svc/debug` logging unless the design is intentionally changed.
- Important service lifecycle events should be logged: install requested, install succeeded or failed, start succeeded or failed, state verification, scheduler start/stop, job start/output/failure/success.

## Verification

Before saying work is complete, run the relevant checks:

```powershell
go build ./cmd/crontab
go test ./...
go install ./cmd/crontab
```

For scheduler behavior, prefer a temporary `--home`, `--file`, and `--log` path so the user's real `%USERPROFILE%\.crontab` is not modified.

Installing or controlling the Windows service may require an Administrator shell. If SCM operations fail with `Access is denied.`, report that as an environment permission issue, not as proof that the service code is broken.

## Cleanup

- Remove temporary directories created for manual verification.
- Do not stop or remove an existing `crontab` Windows service unless the user explicitly asks for it.
- Keep generated binaries and temporary logs out of commits unless the user asks for artifacts.
