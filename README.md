# cnf-gotests

## Overview

The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) is the downstream CNF test framework.
The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) uses an auxiliary resource for network testing - [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) 

## cnf-gotests

The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) is designed to test an OCP cluster with pre-installed CNF components such as:

* Machine config pool to define/collect configurations for labeled nodes
* PTP operator
* SR-IOV operator
* Performance Addon Operator
* SCTP via machine config

NOTICE: The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) removes existing configuration such as PtpConfig, SR-IOV configs.

*More tests you can find in the upstream project - [cnf-tests](https://github.com/openshift-kni/cnf-features-deploy/tree/master/cnf-tests)*

### Recommended environment

#### Network suite:

![pic](https://i.imgur.com/0jPXMdc.png)

* Bare metal server
* 2 SR-IOV NICs
* Switch
* Jumbo frame support and configuration

#### Available features

The list of available features:

* *SR-IOV*
* *PTP*
* *VRF*

#### Environment variables

* `FEATURES` - select the feature you are going to test
* `REPORT_DIR_NAME` - path to general report (default `report/`)
* `REPORTER_ERROR_OUTPUT` - path to test failure report for troubleshooting (default `failed_tests.logs.txt`)
* `NETWORK_TEST_CONTAINER_IMAGE` - path where to download the container image of [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) (default `docker-registry.upshift.redhat.com/cnf-gotests/cnf-gotests-client:latest`)
* `CNF_INTERFACES_LIST` - select the SR-IOV interfaces used in the tests. Multiple interfaces can be selected as needed (ex. SR-IOV suite requires 2 interfaces). 

#### Preconfiguration

1. `GOPROXY="https://goproxy.io,direct"` - configure Global Proxy for Go Modules
2. `make install` - download and install all required dependencies

#### Running the tests

Choose the variant that suits you best:

* `make test-features` - will only run tests for the features that were defined in the `FEATURES` variable 
* `make test-all` - will run the test suite for all features

## How to run

Below is an e2e flow example:

1. Clone the project to your local folder - `git clone https://gitlab.cee.redhat.com/cnf/cnf-gotests.git`

2. Change the folder to the project folder - `cd cnf-gotest`

3. Configure Global Proxy via environment variable - `export GOPROXY="https://goproxy.io,direct"`

4. Download and install needed dependencies - `make install`

5. Select SR-IOV supported interfaces - `export CNF_INTERFACES_LIST=ens1f0,ens1f1`

6. Run all tests - `make test-all`

