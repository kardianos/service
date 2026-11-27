// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

func isSystemd() bool {
	if _, err := exec.LookPath("rpm-ostree"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	if _, err := os.Stat("/proc/1/comm"); err == nil {
		filerc, err := os.Open("/proc/1/comm")
		if err != nil {
			return false
		}
		defer filerc.Close()

		buf := new(bytes.Buffer)
		buf.ReadFrom(filerc)
		contents := buf.String()

		if strings.Trim(contents, " \r\n") == "systemd" {
			return true
		}
	}
	return false
}

type systemd struct {
	i        Interface
	platform string
	*Config

	level ServiceLevel
}

func newSystemdService(i Interface, platform string, c *Config) (Service, error) {
	level, err := c.userServiceLevel()
	if err != nil {
		return nil, err
	}
	if level == ServiceLevelAllUsers && (c.UserName != "" || c.ChRoot != "" ||
		c.Option.string(optionLogDirectory, "") != "") {
		return nil, errors.New("AllUsers services run as the logged-in user and cannot set UserName, ChRoot or LogDirectory")
	}
	s := &systemd{
		i:        i,
		platform: platform,
		Config:   c,

		level: level,
	}

	return s, nil
}

func (s *systemd) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

func (s *systemd) Platform() string {
	return s.platform
}

func (s *systemd) configPath() (string, error) {
	switch s.level {
	case ServiceLevelUser:
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, ".config/systemd/user", s.unitName()), nil
	case ServiceLevelAllUsers:
		return "/etc/systemd/user/" + s.unitName(), nil
	default:
		return "/etc/systemd/system/" + s.unitName(), nil
	}
}

// installScope is the systemctl flag for enable and disable.
func (s *systemd) installScope() []string {
	switch s.level {
	case ServiceLevelUser:
		return []string{"--user"}
	case ServiceLevelAllUsers:
		return []string{"--global"}
	default:
		return nil
	}
}

// installCommands returns the systemctl calls Install makes after writing the
// unit.
func (s *systemd) installCommands() [][]string {
	return s.enableCommands("enable")
}

// uninstallCommands returns the systemctl calls Uninstall makes. The unit
// file is removed after the first one.
func (s *systemd) uninstallCommands() [][]string {
	return s.enableCommands("disable")
}

func (s *systemd) enableCommands(action string) [][]string {
	cmds := [][]string{append(s.installScope(), action, s.unitName())}
	// --global only edits /etc/systemd/user, and has no manager to reload.
	if s.level != ServiceLevelAllUsers {
		cmds = append(cmds, append(s.installScope(), "daemon-reload"))
	}
	return cmds
}

// minAllUsersVersion is the first systemd with ConditionUser=, which keeps
// AllUsers units out of greeter and other system users' managers
const minAllUsersVersion = 235

func (s *systemd) unitName() string {
	return s.Config.Name + ".service"
}

func (s *systemd) getSystemdVersion() int64 {
	_, out, err := runWithOutput("systemctl", "--version")
	if err != nil {
		return -1
	}

	re := regexp.MustCompile(`systemd ([0-9]+)`)
	matches := re.FindStringSubmatch(out)
	if len(matches) != 2 {
		return -1
	}

	v, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return -1
	}

	return v
}

func (s *systemd) hasOutputFileSupport() bool {
	defaultValue := true
	version := s.getSystemdVersion()
	if version == -1 {
		return defaultValue
	}

	if version < 236 {
		return false
	}

	return defaultValue
}

// systemdTemplate is the built-in template, parsed once at init and reused for
// every install. A custom SystemdScript is parsed per install instead.
var systemdTemplate = mustParse(systemdScript)

func (s *systemd) template() (*tmpl, error) {
	if custom := s.Option.string(optionSystemdScript, ""); custom != "" {
		return parseTemplate(custom)
	}
	return systemdTemplate, nil
}

func (s *systemd) runScope() []string {
	if s.level == ServiceLevelSystem {
		return nil
	}
	return []string{"--user"}
}

func (s *systemd) userContextRequired() error {
	if s.level == ServiceLevelAllUsers && isPrivileged() {
		return ErrUserContextRequired
	}
	return nil
}

func (s *systemd) Install() error {
	if s.level == ServiceLevelAllUsers {
		if !isPrivileged() {
			return ErrRequiresPrivilege
		}
		if v := s.getSystemdVersion(); v != -1 && v < minAllUsersVersion {
			return fmt.Errorf("AllUsers services require systemd %d or later, found %d", minAllUsersVersion, v)
		}
	}
	confPath, err := s.configPath()
	if err != nil {
		return err
	}
	_, err = os.Stat(confPath)
	if err == nil {
		return fmt.Errorf("Init already exists: %s", confPath)
	}

	path, err := s.execPath()
	if err != nil {
		return err
	}
	if s.level == ServiceLevelAllUsers {
		path, err = checkExecutableOwnership(path)
		if err != nil {
			return err
		}
	}

	t, err := s.template()
	if err != nil {
		return err
	}
	out, err := t.render(s.templateData(path), tfs)
	if err != nil {
		return err
	}

	if err := s.writeUnit(confPath, out); err != nil {
		return err
	}
	for _, args := range s.installCommands() {
		if err := run("systemctl", args...); err != nil {
			return err
		}
	}
	if s.level == ServiceLevelAllUsers {
		return s.reloadAll()
	}
	return nil
}

// reloadAll makes running user managers load the new unit, so it starts at
// their next desktop login without waiting for the manager to restart.
func (s *systemd) reloadAll() error {
	users, version, err := s.listAllUsers()
	if err != nil {
		return err
	}
	return forEachUser(users, loginUser.label, func(u loginUser) error {
		if running, err := managerRunning(u); err != nil || !running {
			return err
		}
		return runAsUser(version, u, "daemon-reload")
	})
}

func (s *systemd) writeUnit(path, content string) error {
	flags := os.O_WRONLY | os.O_CREATE
	switch s.level {
	case ServiceLevelUser:
		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return err
		}
	case ServiceLevelAllUsers:
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		flags |= os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return err
	}
	if s.level == ServiceLevelAllUsers {
		// umask can narrow the mode, and every user's manager must read it.
		if err := f.Chmod(0644); err != nil {
			f.Close()
			return err
		}
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// templateData returns the values the unit template is rendered with.
func (s *systemd) templateData(path string) map[string]any {
	c := s.Config

	// Conditions that the mini engine cannot express (and/gt on non-string
	// values) are collapsed here to a present-or-empty string, so the template
	// only needs {{if Key}} truthiness. Everything else is passed through as a
	// string or a []string for {{range}}.
	outputFileSupport := ""
	// AllUsers output goes to each user's journal
	// there is no log directory that is writable by every user
	if s.level != ServiceLevelAllUsers && s.Option.bool(optionLogOutput, optionLogOutputDefault) && s.hasOutputFileSupport() {
		outputFileSupport = "yes"
	}

	limitNOFILE := ""
	if n := s.Option.int(optionLimitNOFILE, optionLimitNOFILEDefault); n > -1 {
		limitNOFILE = strconv.Itoa(n)
	}

	wantedBy, userScope, allUsers := "multi-user.target", "", ""
	switch s.level {
	case ServiceLevelUser:
		wantedBy, userScope = "default.target", "yes"
	case ServiceLevelAllUsers:
		wantedBy, userScope, allUsers = "graphical-session.target", "yes", "yes"
	}

	return map[string]any{
		"Description":       c.Description,
		"Path":              path,
		"Name":              c.Name,
		"Dependencies":      c.Dependencies,
		"Arguments":         c.Arguments,
		"ChRoot":            c.ChRoot,
		"WorkingDirectory":  c.WorkingDirectory,
		"UserName":          c.UserName,
		"ReloadSignal":      s.Option.string(optionReloadSignal, ""),
		"PIDFile":           s.Option.string(optionPIDFile, ""),
		"LogDirectory":      s.Option.string(optionLogDirectory, defaultLogDirectory),
		"OutputFileSupport": outputFileSupport,
		"LimitNOFILE":       limitNOFILE,
		"Restart":           s.Option.string(optionRestart, "always"),
		"SuccessExitStatus": s.Option.string(optionSuccessExitStatus, ""),
		"EnvVars":           envVars(c.EnvVars, func(k, v string) string { return "Environment=" + k + "=" + v }),
		"WantedBy":          wantedBy,
		"UserScope":         userScope,
		"AllUsers":          allUsers,
	}
}

func (s *systemd) Uninstall() error {
	if s.level == ServiceLevelAllUsers && !isPrivileged() {
		return ErrRequiresPrivilege
	}
	cmds := s.uninstallCommands()
	if err := run("systemctl", cmds[0]...); err != nil {
		return err
	}
	cp, err := s.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(cp); err != nil {
		return err
	}
	for _, args := range cmds[1:] {
		if err := run("systemctl", args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *systemd) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}
func (s *systemd) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (s *systemd) Run() (err error) {
	err = s.i.Start(s)
	if err != nil {
		return err
	}

	s.Option.funcSingle(optionRunWait, func() {
		var sigChan = make(chan os.Signal, 3)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
	})()

	return s.i.Stop(s)
}

func (s *systemd) Status() (Status, error) {
	if err := s.userContextRequired(); err != nil {
		return StatusUnknown, err
	}
	_, out, err := s.runWithOutput("is-active", s.unitName())
	switch state := strings.TrimSpace(out); state {
	case "active", "reloading", "refreshing", "activating":
		return StatusRunning, nil
	case "deactivating", "maintenance": // windows impl reports pending stopped as stopped
		return StatusStopped, nil
	// inactive can also mean it's not installed, and older systemd (e.g. 219)
	// prints unknown for any unit that isn't loaded
	// check unit files to disambiguate
	case "inactive", "unknown":
		return s.installedStatus()
	case "failed":
		return StatusUnknown, errors.New("service in failed state")
	// systemctl prints no state when it fails itself, e.g. it can't reach
	// the user's bus over su, sudo or cron
	case "":
		if err == nil {
			err = errors.New("systemctl is-active reported no state")
		}
		return StatusUnknown, err
	default:
		return StatusUnknown, fmt.Errorf("%s is in unexpected state %q", s.unitName(), state)
	}
}

// installedStatus tells a stopped unit apart from one that isn't installed
func (s *systemd) installedStatus() (Status, error) {
	_, out, err := s.runWithOutput("list-unit-files", "-t", "service", s.unitName())
	// some versions exit non-zero when no unit files match, but they still
	// print a listing, so only treat empty output as a failure
	if err != nil && strings.TrimSpace(out) == "" {
		return StatusUnknown, err
	}
	// match the full unit name: s.Name alone can match the listing's header
	// or footer, e.g. a service named "unit"
	if strings.Contains(out, s.unitName()) {
		return StatusStopped, nil
	}
	return StatusUnknown, ErrNotInstalled
}

func (s *systemd) Start() error {
	if err := s.userContextRequired(); err != nil {
		return err
	}
	return s.runAction("start")
}

func (s *systemd) Stop() error {
	if err := s.userContextRequired(); err != nil {
		return err
	}
	return s.runAction("stop")
}

func (s *systemd) Restart() error {
	if err := s.userContextRequired(); err != nil {
		return err
	}
	return s.runAction("restart")
}

func (s *systemd) runWithOutput(args ...string) (int, string, error) {
	return runWithOutput("systemctl", append(s.runScope(), args...)...)
}

func (s *systemd) runAction(action string) error {
	return run("systemctl", append(s.runScope(), action, s.unitName())...)
}

// the exit code of "systemctl stop" when the service isn't loaded
const systemctlNotLoaded = 5

// required for "systemctl -M user@"
const minMachineUserVersion = 248

type loginUser struct {
	uid  uint32
	name string
}

func (u loginUser) label() string {
	return u.name
}

// StartAll starts the service for every logged-in user.
// No-op for already running services.
func (s *systemd) StartAll() error {
	users, version, err := s.listAllUsers()
	if err != nil {
		return err
	}
	return forEachUser(users, loginUser.label, func(u loginUser) error {
		if running, err := managerRunning(u); err != nil || !running {
			return err
		}
		// Picks up a unit file added or changed since the manager started.
		if err := runAsUser(version, u, "daemon-reload"); err != nil {
			return err
		}
		return runAsUser(version, u, "start", s.unitName())
	})
}

// StopAll stops the service for every logged-in user.
// Waits for all commands to finish.
func (s *systemd) StopAll() error {
	users, version, err := s.listAllUsers()
	if err != nil {
		return err
	}
	return forEachUser(users, loginUser.label, func(u loginUser) error {
		if running, err := managerRunning(u); err != nil || !running {
			return err
		}
		stop := systemctlAsUser(version, u, "stop", s.unitName())
		code, _, err := runWithOutput(stop[0], stop[1:]...)
		if code == systemctlNotLoaded {
			return nil
		}
		return err
	})
}

func (s *systemd) listAllUsers() ([]loginUser, int64, error) {
	if s.level != ServiceLevelAllUsers {
		return nil, 0, ErrUnsupportedOperation
	}
	if !isPrivileged() {
		return nil, 0, ErrRequiresPrivilege
	}
	_, out, err := runWithOutput("loginctl", "list-users", "--no-legend")
	if err != nil {
		return nil, 0, err
	}
	return parseLoginctlUsers(out), s.getSystemdVersion(), nil
}

// parses `loginctl list-users --no-legend`
// deduplicates users and elides root
func parseLoginctlUsers(out string) []loginUser {
	var users []loginUser
	seen := map[uint32]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		uid, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil || uid == 0 || seen[uint32(uid)] {
			continue
		}
		seen[uint32(uid)] = true
		users = append(users, loginUser{uid: uint32(uid), name: fields[1]})
	}
	return users
}

// loginctl will report managers that are stopping or gone, so we need to do
// an explicit check
func managerRunning(u loginUser) (bool, error) {
	unit := fmt.Sprintf("user@%d.service", u.uid)
	// is-active exits non-zero for every state except active, reloading and
	// (v257+) refreshing, so check the output
	_, out, err := runWithOutput("systemctl", "is-active", unit)
	switch state := strings.TrimSpace(out); state {
	case "active", "reloading", "refreshing", "activating":
		return true, nil
	case "inactive", "failed", "deactivating", "maintenance":
		return false, nil
	case "":
		if err == nil {
			err = errors.New("no state reported")
		}
		return false, fmt.Errorf("checking %s: %w", unit, err)
	default:
		return false, fmt.Errorf("%s is in unexpected state %q", unit, state)
	}
}

// runs a systemctl command for a particular user
// if systemd is 248+, uses -M
// else, fallback to runuser
func systemctlAsUser(version int64, u loginUser, args ...string) []string {
	if version >= minMachineUserVersion {
		return append([]string{"systemctl", "--user", "-M", u.name + "@"}, args...)
	}
	return append([]string{
		"runuser", "-u", u.name, "--",
		"env", fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", u.uid),
		"systemctl", "--user",
	}, args...)
}

func runAsUser(version int64, u loginUser, args ...string) error {
	argv := systemctlAsUser(version, u, args...)
	return run(argv[0], argv[1:]...)
}

// systemdScript is expanded by the mini template engine (renderTemplate).
// {{if}}/{{range}} operate on plain string / []string values built in Install;
// compound conditions such as "and .LogOutput .HasOutputFileSupport" or
// "gt .LimitNOFILE -1" are precomputed there into a present-or-empty string.
const systemdScript = `[Unit]
Description={{Description}}
ConditionFileIsExecutable={{Path | cmdEscape}}
{{if AllUsers}}ConditionUser=!@system
PartOf=graphical-session.target
After=graphical-session.target
{{end}}{{range Dependencies}}{{.}}
{{end}}
[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart={{Path | cmdEscape}}{{range Arguments}} {{. | cmd}}{{end}}
{{if ChRoot}}RootDirectory={{ChRoot | cmd}}
{{end}}{{if WorkingDirectory}}WorkingDirectory={{WorkingDirectory | cmdEscape}}
{{end}}{{if UserScope}}{{else}}{{if UserName}}User={{UserName}}
{{end}}{{end}}{{if ReloadSignal}}ExecReload=/bin/kill -{{ReloadSignal}} "$MAINPID"
{{end}}{{if PIDFile}}PIDFile={{PIDFile | cmd}}
{{end}}{{if OutputFileSupport}}StandardOutput=file:{{LogDirectory}}/{{Name}}.out
StandardError=file:{{LogDirectory}}/{{Name}}.err
{{end}}{{if LimitNOFILE}}LimitNOFILE={{LimitNOFILE}}
{{end}}{{if Restart}}Restart={{Restart}}
{{end}}{{if SuccessExitStatus}}SuccessExitStatus={{SuccessExitStatus}}
{{end}}RestartSec=120
EnvironmentFile=-/etc/sysconfig/{{Name}}

{{range EnvVars}}{{.}}
{{end}}[Install]
WantedBy={{WantedBy}}
`
