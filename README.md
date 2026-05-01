# Crontab for Windows Service

`crontab.exe` provides a Linux-style per-user crontab file on Windows and runs it from a Windows service.

## Install

```powershell
go install github.com/dvgamerr/crontab/cmd/crontab@latest
```

Running `crontab` with no arguments installs the Windows service for the current user's `.crontab`.

```powershell
crontab
crontab start
```

The service runs automatically on boot after it is installed.

## Commands

```powershell
crontab -e
crontab -l
crontab -r
crontab -u <user> -e
crontab install
crontab start
crontab stop
crontab remove
crontab debug
```

`crontab -e` opens `%USERPROFILE%\.crontab` in `%VISUAL%`, `%EDITOR%`, or Notepad.

## Crontab Format

```text
* * * * * curl https://www.google.com
- - - - -
| | | | |
| | | | +----- Day of the Week (0 - 6, Sunday=0 or 7)
| | | +------- Month (1 - 12)
| | +--------- Day of the Month (1 - 31)
| +----------- Hour (0 - 23)
+------------- Minute (0 - 59)
```

The parser supports `*`, lists, ranges, and steps such as `*/5`, `1,15`, and `9-17`.

## Logs

By default logs are written to:

```text
%ProgramData%\crontab\crontab.log
```

The scheduler reloads the crontab file every minute before running due jobs. Commands run through `cmd.exe /C` with the machine environment loaded so commands such as `curl` can be found through `PATH`.
