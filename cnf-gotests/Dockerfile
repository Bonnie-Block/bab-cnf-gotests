FROM golang:1.13 AS builder
WORKDIR /go/src/gitlab.cee.redhat.com/cnf/cnf-gotests
COPY . .
RUN make client-bin

FROM centos:7

COPY --from=builder /go/src/gitlab.cee.redhat.com/cnf/cnf-gotests/cnf-gotests/bin/clientcmd /usr/bin/clientcmd
RUN yum install -y iproute ethtool ping

CMD ["/usr/bin/clientcmd"]
