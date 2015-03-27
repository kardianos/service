# service (BETA)
service will install / un-install, start / stop, and run a program as a service (daemon).
Currently supports Windows XP+, Linux/(systemd | Upstart | SysV), and OSX/Launchd.

Windows controls services by setting up callbacks that is non-trivial. This
is very different then other systems. This package provides the same API
despite the substantial differences.
It also can be used to detect how a program is called, from an interactive
terminal or from a service manager.

## TODO
 * OS X when running as a UserService Interactive will not be accurate.
 * Determine if UserService should remain in main configuration.
 * Hook up Dependencies field for Linux systems and Launchd.
