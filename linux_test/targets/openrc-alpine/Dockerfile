# OpenRC under busybox init, as Alpine boots it.
FROM docker.io/library/alpine:3.20
# A container has no ttys, so busybox would respawn the gettys forever.
# rc_logger gives /var/log/rc.log for the failure logs.
RUN apk add --no-cache openrc \
 && sed -i '/getty/d' /etc/inittab \
 && sed -i 's/^#\?rc_logger=.*/rc_logger="YES"/' /etc/rc.conf
COPY booted /etc/init.d/booted
RUN chmod 0755 /etc/init.d/booted && rc-update add booted default
CMD ["/sbin/init"]
