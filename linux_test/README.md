# Linux container tests

The harness in `harness/` boots systemd, OpenRC and sysvinit in podman containers and runs the service's end-to-end tests inside them. 
Those tests install and start services and create users. 
Containers are deleted upon completion.

## Running

### Requirements

* go 1.20 or later
* podman

```
./container-test.sh                                    # every target
./container-test.sh -only systemd-debian12             # one target
./container-test.sh -suite lifecycle -run Platform     # part of one suite
./container-test.sh -list                              # targets and suites
```

A suite is marked `Best Effort` when execution is unreliable. If the image fails to boot, or requires cgroup v1, then the suite is skipped.
Test output goes to `linux_test/out/<target>/<suite>.log`.
Image build output to `linux_test/out/<target>/build.log`.

| Flag | Meaning |
|---|---|
| `-only a,b` | run only these targets |
| `-suite name` | run only this suite |
| `-run regexp` | replace each suite's test pattern |
| `-timeout 10m` | time limit for each suite's tests |
| `-keep` | leave a failed container running |
| `-pull` | pull the base images again and rebuild |
| `-cleanup` | remove containers left by a crashed run, then exit |
| `-list` | print the targets and suites, then exit |


## Suites

| Suite | Build tags | Tests |
|---|---|---|
| `unit` | none | the library's unit tests, under a booted init |
| `lifecycle` | `su` | miscellaneous lifecycle tests |
| `allusers` | `systemd_container` | the systemd AllUsers |

## Debugging a failure

If you need to look around inside the container, run the target again with `-keep`. The summary then prints the command for a shell in the failed container, such as:

```
podman exec -it kardianos-service-test-systemd-debian12-lifecycle-12345 sh
```

## Adding a target

1. Create `linux_test/targets/<name>/Dockerfile`. 
2. Some init systems report success before the boot has finished. If that's the case, have the image write `/run/booted` from a boot script that runs afterwards.  See `targets/openrc-alpine` and `targets/sysv-debian12` fot an example.
3. Add the target to `targets` in `harness/targets.go`. Mark it `BestEffort` if it may not boot on every host, or `RequiresCgroupV1` if it can't boot on cgroup v2.
4. Run `(cd linux_test/harness && go test ./...)`.

A new init system also needs an entry in `harness/initsys.go`.
