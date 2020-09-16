# service [![GoDoc](https://godoc.org/github.com/kardianos/service?status.svg)](https://godoc.org/github.com/kardianos/service)

service will install / un-install, start / stop, and run a program as a service (daemon).
Currently supports Windows XP+, Linux/(systemd | Upstart | SysV), and OSX/Launchd.

Windows controls services by setting up callbacks that is non-trivial. This
is very different then other systems. This package provides the same API
despite the substantial differences.
It also can be used to detect how a program is called, from an interactive
terminal or from a service manager.

An optional argument can be passed to the windows service on startup for a GRPC port and this 
port is made available to the GRPC client. This is provided because of the limitations of providing
the port number to a windows service and allows the GRPC channel to be created.

## BUGS
 * Dependencies field is not implemented for Linux systems and Launchd.
 * OS X when running as a UserService Interactive will not be accurate.
