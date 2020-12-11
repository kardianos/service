
all: sysv systemd upstart openrc clean

# compile `go test` binary statically
test:
	@CGO_ENABLED=0 go test -installsuffix netgo -a -c ..

clean:
	-rm service.test

sysv: test
	@echo sysv
	@cp service.test sysv/
	@docker build -q --tag="service.test.sysv" sysv
	@-docker run service.test.sysv
	@-docker rm $(shell docker ps -l -q)
	@-docker rmi -f service.test.sysv
	@-rm sysv/service.test

systemd: test
	@echo systemd
	@cp service.test systemd/
	@docker build -q --tag="service.test.systemd" systemd
	@-docker run --privileged -v /sys/fs/cgroup:/sys/fs/cgroup:ro service.test.systemd
	@-docker rm $(shell docker ps -l -q)
	@-docker rmi -f service.test.systemd
	@-rm systemd/service.test

upstart: test
	@echo upstart
	@cp service.test upstart/
	@docker build -q --tag="service.test.upstart" upstart
	@-docker run service.test.upstart
	@-docker rm $(shell docker ps -l -q)
	@-docker rmi -f service.test.upstart
	@-rm upstart/service.test

openrc: test
	@echo openrc
	@cp service.test openrc/
	@docker build -q --tag="service.test.openrc" openrc
	@-docker run service.test.openrc
	@-docker rm $(shell docker ps -l -q)
	@-docker rmi -f service.test.openrc
	@-rm openrc/service.test