# testcmd-test

## Overview

The testcmd-test is a test suite for the [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) utility.  
In order to test testcmd, we use 2 podman containers.  
1 client and 1 server. (*not all protocols use the server side*)  
Every testcmd use case is executed on these containers as exec sessions.

**This test requires root permissions on the testing environment, because the podman containers used are privileged, and use a bridge created during the test run.**

### The testcmd.test binary is built using:

`make testcmd-test`

### Run the testcmd-test using the following command:

`sudo cnf-gotests/bin/testcmd.test`
