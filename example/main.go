package main

import (
	"fmt"
	"os"
	"../../service"
)

func main() {
	var displayName = "Go Service Test2"
	var ws = service.NewService("GoServiceTest2", displayName)
	
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
			fmt.Printf("Service \"%s\" removed.", displayName)
		}
		return
	}
	err := ws.Run(func() error {
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
