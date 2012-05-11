// Package service provides a simple way to create a system service.
// Currently only supports Windows.
package service

// Creates a new service. name is the internal name
// and should not contain spaces. Display name is the pretty print
// name. The description is an arbitrary string used to describe the
// service.
func NewService(name, displayName, description string) Service {
	return newService(name, displayName, description)
}

// Represents a generic way to interact with the system's service.
type Service interface {
	// Installs this service on the system.  May return an
	// error if this service is already installed.
	Install() error

	// Removes this service from the system.  May return an
	// error if this service is not already installed.
	Remove() error

	// Call quickly after initial entry point.  Does not return until
	// service is ready to stop.  onStart is called when the service is
	// starting, returning an error will fail to start the service.
	// If an error is returned from onStop, the service will still stop.
	// An error passed from onStart or onStop will be returned as
	// an error from Run.
	// Both callbacks should return quickly and not block.
	Run(onStart, onStop func() error) error

	// Basic log functions in the context of the service.
	LogError(format string, a ...interface{}) error
	LogWarning(format string, a ...interface{}) error
	LogInfo(format string, a ...interface{}) error
}
