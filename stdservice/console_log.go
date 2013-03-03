package stdservice

import "fmt"

type ConsoleLogger struct{}

func (ConsoleLogger) Error(format string, a ...interface{}) error {
	fmt.Printf("E: "+format, a...)
	return nil
}
func (ConsoleLogger) Warning(format string, a ...interface{}) error {
	fmt.Printf("W: "+format, a...)
	return nil
}
func (ConsoleLogger) Info(format string, a ...interface{}) error {
	fmt.Printf("I: "+format, a...)
	return nil
}
