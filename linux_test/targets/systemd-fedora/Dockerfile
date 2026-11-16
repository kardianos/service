# The newest systemd: StartAll and StopAll use systemctl -M user@.
FROM registry.fedoraproject.org/fedora:latest
RUN dnf install -y systemd dbus dbus-broker util-linux shadow-utils \
 && dnf clean all
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
