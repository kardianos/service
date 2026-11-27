// testsvc is the AllUsers service of the container tests. Each copy writes its
// PID to /tmp/testsvc-<user>.<event>. Thus the tests can see which user's copy
// started or stopped.
package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"strconv"

	"github.com/kardianos/service"
)

type program struct{}

func (program) Start(service.Service) error { return mark("started") }
func (program) Stop(service.Service) error  { return mark("stopped") }

func mark(event string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/tmp/testsvc-%s.%s", u.Username, event)
	// The rename prevents a partial PID for a reader.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func main() {
	s, err := service.New(program{}, &service.Config{
		Name:         "testsvc",
		ServiceLevel: service.ServiceLevelAllUsers,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
