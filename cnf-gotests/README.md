# Testcmd

## Overview

The Pods used by [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) check the network connectivity between them with [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) containers.
[testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) is based on a [docker container image](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/blob/master/cnf-gotests/Dockerfile) which is built using the following command:

`make testcmd-image`

It is also possible to build the [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) binary using the command:

`make testcmd-bin`

## Supported Protocols

#### IPV4

* **icmp**
* **sctp**
* **tcp**
* **udp**
  * **multicast**
  * **broadcast**

#### IPV6

* **icmp**
* **sctp**
* **tcp**
* **udp**
  * **multicast**

## Flags

* **listen** - insert this flag in order to run server 
* **interface** - insert this flag to specify the interface you want to use(Examples: ens33/eth0/net1)
* **multicast** - insert this flag in order to run a udp **multicast** server
* **broadcast** - insert this flag in order to run a udp **broadcast** server
* **protocol** -  protocol name (Options: tcp/udp/icmp/sctp)
* **mtu** - MTU size. Any integer number in range 50-9000 (deafult 1450)
* **server** - destination IPv4/IPv6 address  
* **port** - port number. Any integer number in range 1-65534 (default 80)
* **negative** - insert this flag if **no** connectivity is expected
