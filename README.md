# cnf-gotests

## Overview

The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) is the downstream CNF test framework.
The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) uses an auxiliary resource for network testing
- [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests)

## cnf-gotests

The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) is designed to test an OCP cluster with pre-installed
CNF components such as:

Mandatory:
* Machine config pool to define/collect configurations for labeled nodes
* PTP operator
* SR-IOV operator
* Performance Addon Operator
* SCTP via machine config

Optional:
* N3000 operator
* Sriov-fec operator

NOTICE: The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) removes existing configuration such as
PtpConfig, SR-IOV, N3000Cluster, SriovFecClusterConfig configs .

*More tests you can find in the upstream project
- [cnf-tests](https://github.com/openshift-kni/cnf-features-deploy/tree/master/cnf-tests)*

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
* *N3000*
* *ACC100*
* *CNF-TESTS*

#### Environment variables

##### Common environment variables:
* `FEATURES` - select the feature you are going to test
* `REPORT_DIR_NAME` - path to general report (default `report/`)
* `REPORTER_ERROR_OUTPUT` - path to test failure report for troubleshooting (default `failed_tests.logs.txt`)
* `NETWORK_TEST_CONTAINER_IMAGE` - path where to download the container image
  of [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) (
  default `docker-registry.upshift.redhat.com/cnf-gotests/cnf-gotests-client:latest`)
* `CNF_INTERFACES_LIST` - select the SR-IOV interfaces used in the tests. Multiple interfaces can be selected as
  needed: 
    * SR-IOV suite requires 2 interfaces
    * CNF-TESTS suite requires 1 interface

##### SR-IOV suite environment variables:
* `SRIOV_OPERATOR_NAMESPACE` - select the namespace were sriov-network-operator installed. Default openshift-sriov-network-operator
* `CNF_GOTESTS_SRIOV_SMOKE` - If this variable is set to true then sriov suite will be running in smoke mode. Default value false. Allowed value: `export CNF_GOTESTS_SRIOV_SMOKE="true"`

##### CNF-TESTS suite environment variables:
* `DPDK_IMAGE_VERSION` - select the name of dpdk image.
* `CNF_IMAGE_VERSION` - select the name of cnf-tests container image. 
* `CONTAINER_REPO` - select the image registry

#### Pre-configuration

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

## How to run tests for cnf-tests container:

Below is an e2e flow example of testing cnf-tests image:

1. Make sure either podman or docker container engine is installed and running

2. Select a registry were cnf-tests image is stored - `export CONTAINER_REPO=registry-proxy.engineering.redhat.com/rh-osbs`

3. Select the sriov interface which will be used in cnf-tests discovery mode - - `export CNF_INTERFACES_LIST=ens1f0`
   
4. Select cnf-tests image version - `export CNF_IMAGE_VERSION="openshift4-cnf-tests:v4.7.2-4"`

5. Select dpdk-tests image version - `export DPDK_IMAGE_VERSION="dpdk-base:v4.7.2-1"`

6. Export cnf-tests feature - `export FEATURES="cnf-tests"`

7. Export KUBECONFIG - `export KUBECONFIG=/path/to/kubeconfig`

8. Run feature tests - `make test-features`
