#!/bin/sh
# Runs the Linux container tests in podman. Arguments go to the harness,
# for example -only systemd-debian12 or -list. See linux_test/README.md.
set -eu
cd "$(dirname "$0")/linux_test/harness"
exec go run . "$@"
