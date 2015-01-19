# service (BETA)
service will install / un-install, start / stop, and run a program as a service (daemon).
Currently supports Windows XP+, Linux/(systemd | Upstart | SysV), and OSX/Launchd.

Windows controls services by setting up callbacks that is non-trivial. This
is very different then other systems. This package provides the same API
despite the substantial differences.
It also can be used to detect how a program is called, from an interactive
terminal or from a service manager.

## TODO
Need to test the Interactive test for the following platforms:
 * SysV
 * systemd system and user service
 * Launchd system and user service

The following items need to be done:
 * Determine if systemd has a launchd equivalent user service.
 * Fix Interactive test for user services.
 * Document that windows doesn't have a user service.
 * Document that upstart's user service definition changes over versions. Do not support.
 * For Linux platforms and Launchd add:
   - UserName
   - Arguments
   - WorkingDirectory
   - ChRoot
 * Determine the best way to document the Config.Option values per platform.
   - Should some of the current parameters like "ChRoot" and "WorkingDirectory" move to "Option" map?

