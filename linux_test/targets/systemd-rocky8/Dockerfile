# systemd 239 (RHEL 8): StartAll and StopAll use runuser. Best effort: it may
# not boot on cgroup v2. The base image masks logind, which linger needs.
FROM quay.io/rockylinux/rockylinux:8
RUN dnf install -y systemd dbus util-linux shadow-utils \
 && dnf clean all \
 && systemctl unmask systemd-logind.service
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
