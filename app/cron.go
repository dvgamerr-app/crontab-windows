package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

const CrontabFileName = ".crontab"

type Options struct {
	CrontabPath string
	HomeDir     string
	LogPath     string
}

type Job struct {
	Line     int
	Schedule Schedule
	Command  string
	Env      map[string]string
}

type Schedule struct {
	Minute [60]bool
	Hour   [24]bool
	Dom    [32]bool
	Month  [13]bool
	Dow    [7]bool

	DomStar bool
	DowStar bool
}

type CronFile struct {
	Jobs []Job
	Env  map[string]string
}

type fileLogger struct {
	path string
	mu   sync.Mutex
}

type envEntry struct {
	key string
	val string
}

func DefaultOptions() Options {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return Options{
		CrontabPath: CrontabPathForHome(home),
		HomeDir:     home,
		LogPath:     DefaultLogPath(),
	}
}

func CrontabPathForHome(home string) string {
	return filepath.Join(home, CrontabFileName)
}

func DefaultLogPath() string {
	if programData := os.Getenv("ProgramData"); programData != "" {
		return filepath.Join(programData, "crontab", "crontab.log")
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, "crontab.log")
	}
	return filepath.Join(".", "crontab.log")
}

func RunScheduler(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	logger := &fileLogger{path: opts.LogPath}
	logger.Printf("scheduler started crontab=%q home=%q goos=%s", opts.CrontabPath, opts.HomeDir, runtime.GOOS)
	defer logger.Printf("scheduler stopped")

	next := nextMinute(time.Now())
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-timer.C:
			runDue(ctx, opts, logger, now.Truncate(time.Minute))
			next = nextMinute(time.Now())
			timer.Reset(time.Until(next))
		}
	}
}

func ParseCrontab(path string) (CronFile, []error) {
	f, err := os.Open(path)
	if err != nil {
		return CronFile{Env: map[string]string{}}, []error{err}
	}
	defer f.Close()

	cf := CronFile{Env: map[string]string{}}
	var errs []error
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if k, v, ok := parseEnvLine(raw); ok {
			cf.Env[k] = v
			continue
		}

		fields := strings.Fields(raw)
		if len(fields) < 6 {
			errs = append(errs, fmt.Errorf("%s:%d: expected 5 schedule fields and a command", path, lineNo))
			continue
		}
		sched, err := ParseSchedule(fields[:5])
		if err != nil {
			errs = append(errs, fmt.Errorf("%s:%d: %w", path, lineNo, err))
			continue
		}
		cf.Jobs = append(cf.Jobs, Job{
			Line:     lineNo,
			Schedule: sched,
			Command:  strings.Join(fields[5:], " "),
			Env:      cloneEnv(cf.Env),
		})
	}
	if err := scanner.Err(); err != nil {
		errs = append(errs, err)
	}
	return cf, errs
}

func ParseSchedule(fields []string) (Schedule, error) {
	if len(fields) != 5 {
		return Schedule{}, fmt.Errorf("expected 5 schedule fields")
	}
	var s Schedule
	if err := parseField(fields[0], 0, 59, nil, func(v int) { s.Minute[v] = true }); err != nil {
		return Schedule{}, fmt.Errorf("minute: %w", err)
	}
	if err := parseField(fields[1], 0, 23, nil, func(v int) { s.Hour[v] = true }); err != nil {
		return Schedule{}, fmt.Errorf("hour: %w", err)
	}
	s.DomStar = fields[2] == "*"
	if err := parseField(fields[2], 1, 31, nil, func(v int) { s.Dom[v] = true }); err != nil {
		return Schedule{}, fmt.Errorf("day of month: %w", err)
	}
	if err := parseField(fields[3], 1, 12, nil, func(v int) { s.Month[v] = true }); err != nil {
		return Schedule{}, fmt.Errorf("month: %w", err)
	}
	s.DowStar = fields[4] == "*"
	if err := parseField(fields[4], 0, 7, map[int]int{7: 0}, func(v int) { s.Dow[v] = true }); err != nil {
		return Schedule{}, fmt.Errorf("day of week: %w", err)
	}
	return s, nil
}

func (s Schedule) Matches(t time.Time) bool {
	if !s.Minute[t.Minute()] || !s.Hour[t.Hour()] || !s.Month[int(t.Month())] {
		return false
	}

	day := t.Day()
	dow := int(t.Weekday())
	domMatch := s.Dom[day]
	dowMatch := s.Dow[dow]
	if !s.DomStar && !s.DowStar {
		return domMatch || dowMatch
	}
	return domMatch && dowMatch
}

func ResolveUserHome(userName string) (string, error) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return "", fmt.Errorf("empty user")
	}
	if currentHome, _ := os.UserHomeDir(); currentHome != "" {
		currentUser := os.Getenv("USERNAME")
		if strings.EqualFold(userName, currentUser) {
			return currentHome, nil
		}
	}

	userName = strings.ReplaceAll(userName, "/", "\\")
	parts := strings.Split(userName, "\\")
	userName = parts[len(parts)-1]
	if userName == "" {
		return "", fmt.Errorf("empty user")
	}

	systemDrive := os.Getenv("SystemDrive")
	if systemDrive == "" {
		systemDrive = "C:"
	}
	home := filepath.Join(systemDrive+`\`, "Users", userName)
	if st, err := os.Stat(home); err != nil {
		return "", fmt.Errorf("user home %q not found: %w", home, err)
	} else if !st.IsDir() {
		return "", fmt.Errorf("user home %q is not a directory", home)
	}
	return home, nil
}

func MergedEnvironment(extra map[string]string) []string {
	env := map[string]envEntry{}
	set := func(k, v string) {
		if k == "" {
			return
		}
		env[strings.ToUpper(k)] = envEntry{key: k, val: v}
	}
	for _, pair := range os.Environ() {
		if i := strings.Index(pair, "="); i > 0 {
			set(pair[:i], pair[i+1:])
		}
	}

	machine := readRegistryEnvironment(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
	currentPath := valueFor(env, "PATH")
	for k, v := range machine {
		if strings.EqualFold(k, "Path") || strings.EqualFold(k, "PATH") {
			if currentPath != "" && !strings.Contains(strings.ToLower(currentPath), strings.ToLower(v)) {
				v = v + ";" + currentPath
			}
		}
		set(k, v)
	}

	for k, v := range extra {
		if strings.EqualFold(k, "Path") || strings.EqualFold(k, "PATH") {
			if existing := valueFor(env, "PATH"); existing != "" {
				v = v + ";" + existing
			}
			set("Path", v)
			continue
		}
		set(k, v)
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		e := env[k]
		out = append(out, e.key+"="+e.val)
	}
	return out
}

func runDue(ctx context.Context, opts Options, logger *fileLogger, at time.Time) {
	cf, errs := ParseCrontab(opts.CrontabPath)
	if len(errs) > 0 {
		if len(errs) == 1 && os.IsNotExist(errs[0]) {
			logger.Printf("no crontab file at %q", opts.CrontabPath)
			return
		}
		for _, err := range errs {
			logger.Printf("crontab parse error: %v", err)
		}
	}
	for _, job := range cf.Jobs {
		if job.Schedule.Matches(at) {
			job := job
			go runJob(ctx, opts, logger, job, at)
		}
	}
}

func runJob(ctx context.Context, opts Options, logger *fileLogger, job Job, at time.Time) {
	start := time.Now()
	logger.Printf("line %d start at=%s command=%q", job.Line, at.Format(time.RFC3339), job.Command)

	shell := os.Getenv("ComSpec")
	if shell == "" {
		shell = "cmd.exe"
	}
	cmd := exec.CommandContext(ctx, shell, "/C", job.Command)
	cmd.Dir = opts.HomeDir
	cmd.Env = MergedEnvironment(job.Env)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if output != "" {
		logger.Printf("line %d output:\n%s", job.Line, output)
	}
	if err != nil {
		logger.Printf("line %d failed after %s: %v", job.Line, time.Since(start).Round(time.Millisecond), err)
		return
	}
	logger.Printf("line %d finished after %s", job.Line, time.Since(start).Round(time.Millisecond))
}

func parseField(expr string, min, max int, aliases map[int]int, set func(int)) error {
	if expr == "" {
		return fmt.Errorf("empty field")
	}
	for _, part := range strings.Split(expr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty list item")
		}
		rangePart := part
		step := 1
		if strings.Contains(part, "/") {
			pieces := strings.Split(part, "/")
			if len(pieces) != 2 || pieces[1] == "" {
				return fmt.Errorf("invalid step %q", part)
			}
			rangePart = pieces[0]
			parsedStep, err := strconv.Atoi(pieces[1])
			if err != nil || parsedStep <= 0 {
				return fmt.Errorf("invalid step %q", pieces[1])
			}
			step = parsedStep
		}

		start, end := min, max
		if rangePart != "*" {
			if strings.Contains(rangePart, "-") {
				pieces := strings.Split(rangePart, "-")
				if len(pieces) != 2 {
					return fmt.Errorf("invalid range %q", rangePart)
				}
				var err error
				start, err = strconv.Atoi(pieces[0])
				if err != nil {
					return fmt.Errorf("invalid number %q", pieces[0])
				}
				end, err = strconv.Atoi(pieces[1])
				if err != nil {
					return fmt.Errorf("invalid number %q", pieces[1])
				}
			} else {
				value, err := strconv.Atoi(rangePart)
				if err != nil {
					return fmt.Errorf("invalid number %q", rangePart)
				}
				start, end = value, value
			}
		}
		if start < min || start > max || end < min || end > max || start > end {
			return fmt.Errorf("value %d-%d outside %d-%d", start, end, min, max)
		}
		for v := start; v <= end; v += step {
			if aliases != nil {
				if alias, ok := aliases[v]; ok {
					set(alias)
					continue
				}
			}
			set(v)
		}
	}
	return nil
}

var envLinePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func parseEnvLine(line string) (string, string, bool) {
	if !envLinePattern.MatchString(line) {
		return "", "", false
	}
	parts := strings.SplitN(line, "=", 2)
	return parts[0], strings.Trim(parts[1], `"`), true
}

func cloneEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeOptions(opts Options) Options {
	defaults := DefaultOptions()
	if opts.HomeDir == "" {
		opts.HomeDir = defaults.HomeDir
	}
	if opts.CrontabPath == "" {
		opts.CrontabPath = CrontabPathForHome(opts.HomeDir)
	}
	if opts.LogPath == "" {
		opts.LogPath = defaults.LogPath
	}
	return opts
}

func nextMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute).Add(time.Minute)
}

func (l *fileLogger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "log mkdir failed: %v\n", err)
		return
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log open failed: %v\n", err)
		return
	}
	defer f.Close()

	line := fmt.Sprintf(format, args...)
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
}

func readRegistryEnvironment(root registry.Key, path string) map[string]string {
	out := map[string]string{}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return out
	}
	defer k.Close()

	names, err := k.ReadValueNames(0)
	if err != nil {
		return out
	}
	for _, name := range names {
		value, valueType, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		if valueType == registry.EXPAND_SZ {
			if expanded, err := registry.ExpandString(value); err == nil {
				value = expanded
			}
		}
		out[name] = value
	}
	return out
}

func valueFor(env map[string]envEntry, key string) string {
	if e, ok := env[strings.ToUpper(key)]; ok {
		return e.val
	}
	return ""
}
