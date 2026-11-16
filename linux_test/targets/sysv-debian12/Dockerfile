# sysvinit on Debian 12. Best effort: Debian no longer supports sysvinit as
# its default init.
FROM docker.io/library/debian:12
# With systemctl present, detection could pick systemd, so the build fails if
# the systemd package comes in. A container has no ttys, so init would
# respawn the gettys forever.
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      sysvinit-core sysv-rc initscripts procps \
 && rm -rf /var/lib/apt/lists/* \
 && ! command -v systemctl \
 && sed -i '/getty/d' /etc/inittab
COPY booted /etc/init.d/booted
RUN chmod 0755 /etc/init.d/booted && update-rc.d booted defaults
CMD ["/sbin/init"]
