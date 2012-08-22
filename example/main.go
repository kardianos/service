package main

import (
	"bitbucket.org/kardianos/service"
	"fmt"
	"os"
)

func main() {
	var name = "GoServiceTest"
	var displayName = "Go Service Test"
	var desc = "This is a test Go service.  It is designed to run well."
	var ws, err = service.NewService(name, displayName, desc)

	if err != nil {
		fmt.Printf("%s unable to start: %s", displayName, err)
		return
	}

	if len(os.Args) > 1 {
		var err error
		verb := os.Args[1]
		switch verb {
		case "install":
			err = ws.Install()
			if err != nil {
				fmt.Printf("Failed to install: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" installed.\n", displayName)
		case "remove":
			err = ws.Remove()
			if err != nil {
				fmt.Printf("Failed to remove: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" removed.\n", displayName)
		}
		return
	}
	err = ws.Run(func() error {
		// start
		go doWork()
		ws.LogInfo("I'm Running!")
		return nil
	}, func() error {
		// stop
		stopWork()
		ws.LogInfo("I'm Stopping!")
		return nil
	})
	if err != nil {
		ws.LogError(err.Error())
	}
}

func doWork() {

}
func stopWork() {

}
