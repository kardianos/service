package stdservice

import "fmt"

type ConsoleLogger struct{}

func (ConsoleLogger) Error(format string, a ...interface{}) error {
	fmt.Printf("E: "+format+"\n", a...)
	return nil
}
func (ConsoleLogger) Warning(format string, a ...interface{}) error {
	fmt.Printf("W: "+format+"\n", a...)
	return nil
}
func (ConsoleLogger) Info(format string, a ...interface{}) error {
	fmt.Printf("I: "+format+"\n", a...)
	return nil
}
