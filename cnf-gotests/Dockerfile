FROM golang:1.13 AS builder
WORKDIR /go/src/gitlab.cee.redhat.com/cnf/cnf-gotests
COPY . .
RUN make test-bin

FROM centos:7

COPY --from=builder /go/src/gitlab.cee.redhat.com/cnf/cnf-gotests/cnf-gotests/bin/testcmd /usr/bin/testcmd
RUN yum install -y iproute ethtool ping httpd nmap

CMD ["/usr/bin/testcmd"]
