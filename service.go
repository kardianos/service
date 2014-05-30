// Package service provides a simple way to create a system service.
// Currently supports Windows, Linux/(systemd | Upstart | SysV), and OSX/Launchd.
package service

import "bitbucket.org/kardianos/osext"

// Creates a new service. name is the internal name
// and should not contain spaces. Display name is the pretty print
// name. The description is an arbitrary string used to describe the
// service.
func NewService(name, displayName, description string) (Service, error) {
	return newService(&Config{
		Name:        name,
		DisplayName: displayName,
		Description: description,
	})
}

// Alpha API. Do not yet use.
type Config struct {
	Name, DisplayName, Description string

	UserName  string   // Run as username.
	Arguments []string // Run with arguments.

	DependsOn        []string // Other services that this depends on.
	WorkingDirectory string   // Service working directory.
	ChRoot           string
	UserService      bool // Install as a current user service.

	// System specific parameters.
	KV KeyValue
}

type KeyValue map[string]interface{}

// Bool returns the value of the given name, assuming the value is a boolean.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) bool(name string, defaultValue bool) bool {
	if v, found := kv[name]; found {
		if castValue, is := v.(bool); is {
			return castValue
		}
	}
	return defaultValue
}

// Int returns the value of the given name, assuming the value is an int.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) int(name string, defaultValue int) int {
	if v, found := kv[name]; found {
		if castValue, is := v.(int); is {
			return castValue
		}
	}
	return defaultValue
}

// Int returns the value of the given name, assuming the value is a string.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) string(name string, defaultValue string) string {
	if v, found := kv[name]; found {
		if castValue, is := v.(string); is {
			return castValue
		}
	}
	return defaultValue
}

// Int returns the value of the given name, assuming the value is a float64.
// If the value isn't found or is not of the type, the defaultValue is returned.
func (kv KeyValue) float64(name string, defaultValue float64) float64 {
	if v, found := kv[name]; found {
		if castValue, is := v.(float64); is {
			return castValue
		}
	}
	return defaultValue
}

// Alpha API. Do not yet use.
func NewServiceConfig(c *Config) (Service, error) {
	return newService(c)
}

// Represents a generic way to interact with the system's service.
type Service interface {
	Installer
	Controller
	Runner
	Logger
}

// A Generic way to stop and start a service.
type Runner interface {
	// Call quickly after initial entry point.  Does not return until
	// service is ready to stop.  onStart is called when the service is
	// starting, returning an error will fail to start the service.
	// If an error is returned from onStop, the service will still stop.
	// An error passed from onStart or onStop will be returned as
	// an error from Run.
	// Both callbacks should return quickly and not block.
	Run(onStart, onStop func() error) error
}

// Simple install and remove commands.
type Installer interface {
	// Installs this service on the system.  May return an
	// error if this service is already installed.
	Install() error

	// Removes this service from the system.  May return an
	// error if this service is not already installed.
	Remove() error
}

// A service that implements ServiceController is able to
// start and stop itself.
type Controller interface {
	// Starts this service on the system.
	Start() error

	// Stops this service on the system.
	Stop() error
}

// A service that implements ServiceLogger can perform simple system logging.
type Logger interface {
	// Basic log functions in the context of the service.
	Error(format string, a ...interface{}) error
	Warning(format string, a ...interface{}) error
	Info(format string, a ...interface{}) error
}

// Depreciated. Use osext.Executable instead.
// Returns the full path of the running executable
// as reported by the system. Includes the executable
// image name.
func GetExePath() (exePath string, err error) {
	return osext.Executable()
}
