package stdservice

import "fmt"

type ConsoleLogger struct{}

func (ConsoleLogger) LogError(format string, a ...interface{}) error {
	fmt.Printf(format, a...)
	return nil
}
func (ConsoleLogger) LogWarning(format string, a ...interface{}) error {
	fmt.Printf(format, a...)
	return nil
}
func (ConsoleLogger) LogInfo(format string, a ...interface{}) error {
	fmt.Printf(format, a...)
	return nil
}
