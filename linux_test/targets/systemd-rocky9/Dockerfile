# systemd 252 (RHEL 9). Its GNOME (40) starts graphical-session.target, unlike
# Rocky 8's. The unmask is kept in case the base image masks logind as Rocky 8's does.
FROM quay.io/rockylinux/rockylinux:9
RUN dnf install -y systemd dbus util-linux shadow-utils \
 && dnf clean all \
 && systemctl unmask systemd-logind.service
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
