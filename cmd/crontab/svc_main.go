//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/dvgamerr-app/crontab-windows/app"
	"github.com/gookit/slog"
	"golang.org/x/sys/windows/svc"
)

type cliOptions struct {
	command string
	user    string
	opts    app.Options
}

func usage(errmsg string) {
	if errmsg != "" {
		fmt.Fprintf(os.Stderr, "%s\n\n", errmsg)
	}
	fmt.Fprintf(os.Stderr,
		"usage: %s [install|remove|start|stop|debug]\n"+
			"       %s -e\n"+
			"       %s -l\n"+
			"       %s -r\n"+
			"       %s -u <user> -e|-l|-r\n",
		os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	os.Exit(2)
}

func main() {
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		slog.Errorf("failed to determine if we are running in an interactive session: %v", err)
		os.Exit(1)
	}
	if !isIntSess {
		runService(svcName, false)
		return
	}

	if err := runCLI(os.Args[1:]); err != nil {
		slog.Error(err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	parsed, err := parseCLI(args)
	if err != nil {
		usage(err.Error())
	}

	switch parsed.command {
	case "":
		return installAndStartService(svcName, svcNameLong, parsed.opts)
	case "install":
		return installAndStartService(svcName, svcNameLong, parsed.opts)
	case "remove":
		return removeService(svcName)
	case "start":
		return installAndStartService(svcName, svcNameLong, parsed.opts)
	case "stop":
		return controlService(svcName, svc.Stop, svc.Stopped)
	case "pause":
		return controlService(svcName, svc.Pause, svc.Paused)
	case "continue":
		return controlService(svcName, svc.Continue, svc.Running)
	case "debug":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return app.Run(ctx, svcName, sha1ver, parsed.opts)
	case "service":
		runService(svcName, false)
		return nil
	case "-e":
		return editCrontab(parsed.opts.CrontabPath)
	case "-l":
		return listCrontab(parsed.opts.CrontabPath)
	case "-r":
		return removeCrontab(parsed.opts.CrontabPath)
	default:
		usage(fmt.Sprintf("invalid command %s", parsed.command))
	}
	return nil
}

func parseCLI(args []string) (cliOptions, error) {
	parsed := cliOptions{opts: app.DefaultOptions()}
	fileExplicit := false
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(args[i])
		switch arg {
		case "-u":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("-u requires a user")
			}
			parsed.user = args[i]
			home, err := app.ResolveUserHome(parsed.user)
			if err != nil {
				return parsed, err
			}
			parsed.opts.HomeDir = home
			parsed.opts.CrontabPath = app.CrontabPathForHome(home)
		case "--home":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--home requires a path")
			}
			parsed.opts.HomeDir = args[i]
			if !fileExplicit {
				parsed.opts.CrontabPath = app.CrontabPathForHome(parsed.opts.HomeDir)
			}
		case "--file":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--file requires a path")
			}
			fileExplicit = true
			parsed.opts.CrontabPath = args[i]
		case "--log":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--log requires a path")
			}
			parsed.opts.LogPath = args[i]
		case "-h", "--help", "help":
			usage("")
		case "-e", "-l", "-r", "install", "remove", "start", "stop", "pause", "continue", "debug", "service":
			if parsed.command != "" {
				return parsed, fmt.Errorf("multiple commands specified")
			}
			parsed.command = arg
		default:
			return parsed, fmt.Errorf("invalid argument %s", args[i])
		}
	}
	if parsed.opts.HomeDir != "" && parsed.opts.CrontabPath == "" {
		parsed.opts.CrontabPath = app.CrontabPathForHome(parsed.opts.HomeDir)
	}
	return parsed, nil
}

func serviceOptionsFromArgs() app.Options {
	parsed, err := parseCLI(os.Args[1:])
	if err != nil {
		return app.DefaultOptions()
	}
	return parsed.opts
}

func editCrontab(path string) error {
	if err := os.MkdirAll(filepathDir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := writeCrontabExample(path); err != nil {
		return err
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	var cmd *exec.Cmd
	if editor == "" {
		if codePath := findVSCode(); codePath != "" {
			cmd = exec.Command(codePath, "--reuse-window", path)
		} else {
			cmd = exec.Command("notepad.exe", path)
		}
	} else {
		shell := os.Getenv("ComSpec")
		if shell == "" {
			shell = "cmd.exe"
		}
		cmd = exec.Command(shell, "/C", editor+" "+quoteCmdArg(path))
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const crontabExample = `# crontab for Windows
#
# Format:
#   minute hour day-of-month month day-of-week command
#
# Fields:
#   minute        0-59
#   hour          0-23
#   day-of-month  1-31
#   month         1-12
#   day-of-week   0-6 (Sunday=0 or 7)
#
# Examples:
#   Run every minute:
# * * * * * curl.exe -I https://www.google.com
#
#   Run every 5 minutes:
# */5 * * * * echo hello
#
#   Run at 09:00 every Monday:
# 0 9 * * 1 powershell.exe -NoProfile -Command "Get-Date"

`

func writeCrontabExample(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Size() > 0 {
		return nil
	}
	return os.WriteFile(path, []byte(crontabExample), 0644)
}

func findVSCode() string {
	for _, name := range []string{"code.cmd", "code.exe", "code"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	candidates := []string{
		joinIfBase(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "bin", "code.cmd"),
		joinIfBase(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "Code.exe"),
		joinIfBase(os.Getenv("ProgramFiles"), "Microsoft VS Code", "bin", "code.cmd"),
		joinIfBase(os.Getenv("ProgramFiles"), "Microsoft VS Code", "Code.exe"),
		joinIfBase(os.Getenv("ProgramFiles(x86)"), "Microsoft VS Code", "bin", "code.cmd"),
		joinIfBase(os.Getenv("ProgramFiles(x86)"), "Microsoft VS Code", "Code.exe"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

func listCrontab(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("no crontab for this user")
	}
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(os.Stdout, f)
	return err
}

func removeCrontab(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no crontab for this user")
		}
		return err
	}
	return nil
}

func filepathDir(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[:i]
	}
	return "."
}

func joinIfBase(base string, parts ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

func quoteCmdArg(arg string) string {
	return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
}
