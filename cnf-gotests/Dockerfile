FROM golang:1.13 AS builder
WORKDIR /go/src/gitlab.cee.redhat.com/cnf/cnf-gotests
COPY . .
RUN make test-bin

# TODO: Remove this step once PR https://github.com/nmap/nmap/pull/2033 will be merged
FROM centos:7 AS compiler
RUN yum install -y make gcc gcc-c++ git sshd openssh-server openssh-clients
RUN git clone https://github.com/kononovn/nmap.git
WORKDIR "/nmap"
RUN ./configure
RUN make
RUN make install

FROM centos:7
COPY --from=builder /go/src/gitlab.cee.redhat.com/cnf/cnf-gotests/cnf-gotests/bin/testcmd /usr/bin/testcmd
COPY --from=compiler /nmap/nping/nping /usr/bin/nping
RUN yum install -y iproute ethtool ping httpd

CMD ["/usr/bin/testcmd"]
