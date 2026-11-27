# systemd 219: AllUsers Install must refuse it. Runs only on a cgroup v1 host;
# elsewhere the refusal is covered by unit tests only. The base image
# already has systemd, and its repositories are gone, so nothing is installed.
FROM docker.io/library/centos:7
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
