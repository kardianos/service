// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxPathSize = 32 * 1024

const (
	version                   = "darwin-launchd"
	defaultDarwinLogDirectory = "/var/log"
)

type darwinSystem struct{}

func (darwinSystem) String() string {
	return version
}

func (darwinSystem) Detect() bool {
	return true
}

func (darwinSystem) Interactive() bool {
	return interactive
}

func (darwinSystem) New(i Interface, c *Config) (Service, error) {
	level, err := c.userServiceLevel()
	if err != nil {
		return nil, err
	}
	if level == ServiceLevelAllUsers && (c.UserName != "" || c.ChRoot != "" ||
		c.Option.string(optionLogDirectory, "") != "" || c.Option.bool(optionSessionCreate, optionSessionCreateDefault)) {
		return nil, errors.New("AllUsers services run as the logged-in user and cannot set UserName, ChRoot, LogDirectory or SessionCreate")
	}
	s := &darwinLaunchdService{
		i:      i,
		Config: c,

		level: level,
	}

	return s, nil
}

func init() {
	ChooseSystem(darwinSystem{})
}

var interactive = false

func init() {
	var err error
	interactive, err = isInteractive()
	if err != nil {
		panic(err)
	}
}

func isInteractive() (bool, error) {
	// TODO: The PPID of Launchd is 1. The PPid of a service process should match launchd's PID.
	return os.Getppid() != 1, nil
}

type darwinLaunchdService struct {
	i Interface
	*Config

	level ServiceLevel
}

func (s *darwinLaunchdService) String() string {
	if len(s.DisplayName) > 0 {
		return s.DisplayName
	}
	return s.Name
}

func (s *darwinLaunchdService) Platform() string {
	return version
}

var userHomeDir = func() (string, error) {
	u, err := user.Current()
	if err == nil {
		return u.HomeDir, nil
	}

	// alternate methods
	homeDir := os.Getenv("HOME") // *nix
	if homeDir == "" {
		return "", errors.New("User home directory not found.")
	}
	return homeDir, nil
}

var darwinLibraryDir = "/Library"

func (s *darwinLaunchdService) getServiceFilePath() (string, error) {
	switch s.level {
	case ServiceLevelUser:
		homeDir, err := userHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + "/Library/LaunchAgents/" + s.Name + ".plist", nil
	case ServiceLevelAllUsers:
		return darwinLibraryDir + "/LaunchAgents/" + s.Name + ".plist", nil
	default:
		return darwinLibraryDir + "/LaunchDaemons/" + s.Name + ".plist", nil
	}
}

// logDir returns the directory for launchd's stdout and stderr files, or ""
// for none.
func (s *darwinLaunchdService) logDir() (string, error) {
	if customDir := s.Option.string(optionLogDirectory, ""); customDir != "" {
		return customDir, nil
	}
	switch s.level {
	case ServiceLevelUser:
		return userHomeDir()
	case ServiceLevelAllUsers:
		// default log location is not writable by users
		// launchd will fail to spawn as a result
		// TODO: sort out a way to redirect logs at runtime in the default case
		return "", nil
	}
	return defaultDarwinLogDirectory, nil
}

func (s *darwinLaunchdService) getLogPaths() (string, string, error) {
	logDir, err := s.logDir()
	if err != nil || logDir == "" {
		return "", "", err
	}
	return s.getLogPath(logDir, "out"), s.getLogPath(logDir, "err"), nil
}

func (s *darwinLaunchdService) getLogPath(logDir, logType string) string {
	return fmt.Sprintf("%s/%s.%s.log", logDir, s.Name, logType)
}

// plistEscaper matches text/template's html escaper, which the launchd plist
// was previously rendered with.
var plistEscaper = strings.NewReplacer(
	`&`, "&amp;",
	`'`, "&#39;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&#34;",
)

// launchdFuncs is the pipeline function map for the launchd plist: html escapes
// a value for XML.
var launchdFuncs = map[string]tmplFunc{
	"html": func(s string) (string, error) { return plistEscaper.Replace(s), nil },
}

var launchdTemplate = mustParse(launchdConfig)

func (s *darwinLaunchdService) template() (*tmpl, error) {
	if custom := s.Option.string(optionLaunchdConfig, ""); custom != "" {
		return parseTemplate(custom)
	}
	return launchdTemplate, nil
}

// launchdBool renders a Go bool as the "true"/"false" element name used in the
// plist (previously the text/template "bool" function).
func launchdBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (s *darwinLaunchdService) Install() error {
	if s.level == ServiceLevelAllUsers && !isPrivileged() {
		return ErrRequiresPrivilege
	}
	confPath, err := s.getServiceFilePath()
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

	if s.level == ServiceLevelUser {
		// Ensure that ~/Library/LaunchAgents exists.
		err = os.MkdirAll(filepath.Dir(confPath), 0700)
		if err != nil {
			return err
		}
	}

	f, err := s.createPlist(confPath)
	if err != nil {
		return err
	}
	defer f.Close()

	stdOutPath, stdErrPath, _ := s.getLogPaths()

	c := s.Config
	data := map[string]any{
		"Name":              c.Name,
		"Path":              path,
		"Arguments":         c.Arguments,
		"ChRoot":            c.ChRoot,
		"UserName":          c.UserName,
		"WorkingDirectory":  c.WorkingDirectory,
		"KeepAlive":         launchdBool(s.Option.bool(optionKeepAlive, optionKeepAliveDefault)),
		"RunAtLoad":         launchdBool(s.Option.bool(optionRunAtLoad, optionRunAtLoadDefault)),
		"SessionCreate":     launchdBool(s.Option.bool(optionSessionCreate, optionSessionCreateDefault)),
		"StandardOutPath":   stdOutPath,
		"StandardErrorPath": stdErrPath,
		// Each entry is the two escaped plist lines for one variable.
		"EnvVars": envVars(c.EnvVars, func(k, v string) string {
			return "\t\t<key>" + plistEscaper.Replace(k) + "</key>\n" +
				"\t\t<string>" + plistEscaper.Replace(v) + "</string>"
		}),
	}

	t, err := s.template()
	if err != nil {
		return err
	}
	out, err := t.render(data, launchdFuncs)
	if err != nil {
		return err
	}
	_, err = f.WriteString(out)
	return err
}

func (s *darwinLaunchdService) createPlist(path string) (*os.File, error) {
	if s.level != ServiceLevelAllUsers {
		return os.Create(path)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	// launchd ignores agent plists that are not 0644
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *darwinLaunchdService) Uninstall() error {
	if s.level == ServiceLevelAllUsers {
		if !isPrivileged() {
			return ErrRequiresPrivilege
		}
		// Running copies are left until each user logs out.
	} else {
		s.Stop()
	}

	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	return os.Remove(confPath)
}

func (s *darwinLaunchdService) userContextRequired() error {
	if s.level == ServiceLevelAllUsers && isPrivileged() {
		return ErrUserContextRequired
	}
	return nil
}

func (s *darwinLaunchdService) Status() (Status, error) {
	if err := s.userContextRequired(); err != nil {
		return StatusUnknown, err
	}
	exitCode, out, err := runWithOutput("launchctl", "list", s.Name)
	if exitCode == 0 && err != nil {
		if !strings.Contains(err.Error(), "failed with stderr") {
			return StatusUnknown, err
		}
	}

	re := regexp.MustCompile(`"PID" = ([0-9]+);`)
	matches := re.FindStringSubmatch(out)
	if len(matches) == 2 {
		return StatusRunning, nil
	}

	confPath, err := s.getServiceFilePath()
	if err != nil {
		return StatusUnknown, err
	}

	if _, err = os.Stat(confPath); err == nil {
		return StatusStopped, nil
	}

	return StatusUnknown, ErrNotInstalled
}

func (s *darwinLaunchdService) Start() error {
	if err := s.userContextRequired(); err != nil {
		return err
	}
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	return s.startIn(s.domain(), confPath)
}

func (s *darwinLaunchdService) domain() string {
	if s.level == ServiceLevelSystem {
		return "system"
	}
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func (s *darwinLaunchdService) startIn(domain, confPath string) error {
	target := domain + "/" + s.Name
	code, _, err := runWithOutput("launchctl", "kickstart", target)
	if code == launchctlNotFound {
		err = run("launchctl", "bootstrap", domain, confPath)
		// we still need to start the service after bootstrapping
		if err == nil {
			err = run("launchctl", "kickstart", target)
		}
	}
	return err
}

func (s *darwinLaunchdService) Stop() error {
	if err := s.userContextRequired(); err != nil {
		return err
	}

	// plist is not guaranteed to exist, as uninstall removes it
	// stop by name instead
	return s.stopIn(s.domain())
}

func (s *darwinLaunchdService) stopIn(domain string) error {
	code, _, err := runWithOutput("launchctl", "bootout", domain+"/"+s.Name)
	if code == launchctlNoSuchProcess {
		return nil
	}
	return err
}

func (s *darwinLaunchdService) Restart() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return s.Start()
}

// launchctl exit codes for a service that isn't loaded in the domain.
const (
	launchctlNoSuchProcess = 3   // bootout: "Boot-out failed: 3: No such process"
	launchctlNotFound      = 113 // kickstart: "Could not find service"
)

// StartAll attempts to start the service for every logged-in GUI user.
func (s *darwinLaunchdService) StartAll() error {
	uids, err := s.allUsers()
	if err != nil {
		return err
	}
	confPath, err := s.getServiceFilePath()
	if err != nil {
		return err
	}
	return forEachUser(uids, guiDomain, func(uid uint32) error {
		return s.startIn(guiDomain(uid), confPath)
	})
}

// StopAll unloads the service from every logged-in GUI user's domain
// Take precedence over KeepAlive.
func (s *darwinLaunchdService) StopAll() error {
	uids, err := s.allUsers()
	if err != nil {
		return err
	}
	return forEachUser(uids, guiDomain, func(uid uint32) error {
		return s.stopIn(guiDomain(uid))
	})
}

func guiDomain(uid uint32) string {
	return fmt.Sprintf("gui/%d", uid)
}

// allUsers checks StartAll and StopAll can run, and returns the uids of the
// logged-in GUI users.
func (s *darwinLaunchdService) allUsers() ([]uint32, error) {
	if s.level != ServiceLevelAllUsers {
		return nil, ErrUnsupportedOperation
	}
	if !isPrivileged() {
		return nil, ErrRequiresPrivilege
	}
	_, out, err := runWithOutput("ps", "-axo", "uid=,comm=")
	if err != nil {
		return nil, err
	}
	return parseGUIUsers(out), nil
}

// parseGUIUsers returns the uids that own a loginwindow process in the output
// of "ps -axo uid=,comm=", without duplicates or root.
func parseGUIUsers(out string) []uint32 {
	var uids []uint32
	seen := map[uint32]bool{}
	for _, line := range strings.Split(out, "\n") {
		uidField, comm, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || filepath.Base(strings.TrimSpace(comm)) != "loginwindow" {
			continue
		}
		uid, err := strconv.ParseUint(uidField, 10, 32)
		if err != nil || uid == 0 || seen[uint32(uid)] {
			continue
		}
		seen[uint32(uid)] = true
		uids = append(uids, uint32(uid))
	}
	return uids
}

func (s *darwinLaunchdService) Run() error {
	err := s.i.Start(s)
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

func (s *darwinLaunchdService) Logger(errs chan<- error) (Logger, error) {
	if interactive {
		return ConsoleLogger, nil
	}
	return s.SystemLogger(errs)
}

func (s *darwinLaunchdService) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

const launchdConfig = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Disabled</key>
	<false/>
	{{if EnvVars}}<key>EnvironmentVariables</key>
	<dict>
{{range EnvVars}}{{.}}
{{end}}	</dict>
	{{end}}<key>KeepAlive</key>
	<{{KeepAlive}}/>
	<key>Label</key>
	<string>{{Name | html}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{Path | html}}</string>
{{range Arguments}}		<string>{{. | html}}</string>
{{end}}	</array>
	{{if ChRoot}}<key>RootDirectory</key>
	<string>{{ChRoot | html}}</string>
	{{end}}<key>RunAtLoad</key>
	<{{RunAtLoad}}/>
	<key>SessionCreate</key>
	<{{SessionCreate}}/>
	{{if StandardErrorPath}}<key>StandardErrorPath</key>
	<string>{{StandardErrorPath | html}}</string>
	{{end}}{{if StandardOutPath}}<key>StandardOutPath</key>
	<string>{{StandardOutPath | html}}</string>
	{{end}}{{if UserName}}<key>UserName</key>
	<string>{{UserName | html}}</string>
	{{end}}{{if WorkingDirectory}}<key>WorkingDirectory</key>
	<string>{{WorkingDirectory | html}}</string>
	{{end}}</dict>
</plist>
`
