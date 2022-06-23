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
* MetalLB operator

Optional:
* Sriov-fec operator
* RAN DU profile

NOTICE: The [cnf-gotests](https://gitlab.cee.redhat.com/cnf/cnf-gotests) removes existing configuration such as
PtpConfig, SR-IOV, SriovFecClusterConfig configs .

*More tests you can find in the upstream project
- [cnf-tests](https://github.com/openshift-kni/cnf-features-deploy/tree/master/cnf-tests)*

### Recommended environment

#### Network suite:

![pic](https://i.imgur.com/0jPXMdc.png)

* Bare metal server
* 2 SR-IOV NICs
* Switch
* Jumbo frame support and configuration

#### RAN suite:
* Bare metal server
* Real-time kernel

#### Available features

The list of available features:

##### Available features for Network suite
* *SR-IOV*
* *PTP*
* *CNI*
* *ACC100*
* *CNF-TESTS*
* *MetalLB*

##### Available features for RAN suite
* *CPU*

#### Environment variables

##### Common environment variables
* `FEATURES` - select the feature you are going to test
* `REPORT_DIR_NAME` - path to general report (default `report/`)
* `REPORTER_ERROR_OUTPUT` - path to test failure report for troubleshooting (default `failed_tests.logs.txt`)

##### Common network environment variables
* `NETWORK_TEST_CONTAINER_IMAGE` - path where to download the container image
  of [testcmd](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/tree/master/cnf-gotests) (
  default `quay.io/ocp-edge-qe/cnf-gotests-client:latest`)
* `CNF_INTERFACES_LIST` - select the interfaces used in the tests. Multiple interfaces can be selected as
  needed:
  * SR-IOV suite requires 2 interfaces
  * CNF-TESTS suite requires 1 interface
  * CNI suite requires 2 interfaces

##### Common RAN environment variables
* `CNF_TEST_IMAGE` - path to generic container image used in RAN tests
* `STRESSNG_TEST_IMAGE` - path to StressNg container image
* `OSLAT_TEST_IMAGE` - path to Oslat container image

##### SR-IOV suite environment variables:
* `SRIOV_OPERATOR_NAMESPACE` - select the namespace were sriov-network-operator installed. Default openshift-sriov-network-operator
* `CNF_GOTESTS_SRIOV_SMOKE` - If this variable is set to true then sriov suite will be running in smoke mode. Default value false. Allowed value: `export CNF_GOTESTS_SRIOV_SMOKE="true"`

##### MetalLB suite environment variables:
* `METALLB_ADDR_LIST` - is used to create the metalLB L2 and BGP service address pool. These addresses are specific to the Helix TLV lab. Addresses include both IPv4 and IPv6 for Single Stack and Dual Stack deployments. If no IP environmental variable is present all MetalLB test cases are skipped. 
* `FRR_IMAGE` - path where to download the frr image (default `quay.io/ocp-edge-qe/frr:stable_7.5`)

##### CNF-TESTS suite environment variables:
* `DPDK_IMAGE_VERSION` - select the name of dpdk image.
* `CNF_IMAGE_VERSION` - select the name of cnf-tests container image. 
* `CONTAINER_REPO` - select the image registry

##### CPU suite environment variables:
* `RAN_WORKLOAD_DURATION` - duration for RAN CPU workload test. e.g., 10m, 12h

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

5. Select interfaces - `export CNF_INTERFACES_LIST=ens1f0,ens1f1`

6. Select the correct MetalLB IP address list `METALLB_ADDR_LIST` depending on which Helix TLV lab cluster is being used for testing.

  Cluster-2 and Cluster-3: `export METALLB_ADDR_LIST="10.46.55.131,10.46.55.132"`
  Cluster-7 `export METALLB_ADDR_LIST="10.46.56.131,10.46.56.132"`

7. Run all tests - `make test-all`

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

## How to run RAN tests

Below is an e2e flow example for RAN cpu test:

1. Make sure oc is installed

2. Set default test image - `export CNF_TEST_IMAGE=quay.io/ocp-edge-qe/cnf-gotests-client:latest`

3. Set stress-ng test image - `STRESSNG_TEST_IMAGE=quay.io/ocp-edge-qe/stress-ng:2.0`

4. Set oslat test image - `export OSLAT_TEST_IMAGE=quay.io/ocp-edge-qe/oslat:latest`

5. Set process-exporter image - `export PROCESS_EXPORTER_IMAGE=quay.io/ocp-edge-qe/process-exporter:ppid-2`

6. Set workload test duration - `export RAN_WORKLOAD_DURATION=12h`

7. Set ran tests feature - `export FEATURES=cpu`

8. Export KUBECONFIG - `export KUBECONFIG=/path/to/kubeconfig`

9. Run feature tests - `make test-features`

## Conventions

### Project structure

```text
├── cnf-gotests                   # cnf-gotests-client container image dependencies 
│   ├── test
│   └── testcmd                   # testcmd utility  
├── config                        # Config files
├── hack                          # Makefile Scripts 
├── test                          # Test features folder
│   ├── accelerator               # Test suites for accelerator features
│   │   ├── acc100
│   │   └── netacceleratorhelper
│   ├── cnf-tests                 # Test suites for cnf-test image features
│   │   └── discovery
│   ├── helper                    # Common test functions 
│   ├── network                   # Test suites for network features
│   │   ├── bfd
│   │   ├── ptp
│   │   ├── nethelper             # Common network test functions
│   │   ├── netparameters         # Common network parameters
│   │   └── cni
│   ├── parameters                # Common parameters
│   ├── ran
│   │   ├── cpu
│   │   ├── kpi
│   │   ├── ranhelper             # Common RAN parameters
│   │   └── workloadpartitioning
│   └── util                      # Common utils functions. These utils are based on Kubernetes api calls
│       ├── client
│       └── utils
└── vendor                        # Dependencies folder 

```

### Committing new code
#### The following is a step-by-step example of forking workflow:
1. A developer forks the [cnf-gotests] https://gitlab.cee.redhat.com/cnf/cnf-gotests project
2. A new local feature branch is created
3. The developer makes changes on the new branch.
4. New commits are created for the changes.
5. The branch gets pushed to the developer's own server-side copy.
6. Changes are tested.
7. The developer opens a pull request(PR) from the new branch to the cnf-gotests.
8. The pull request gets approved for merge and is merged into the cnf-gotests.

**Note:** Dependencies residing in the vendor directory will be seperated to a commit from the code commit

### Test Configuration
The testing repository has several resources for a test to get its arguments, as additional credentials,
namespaces names, and timeout definitions.

The main file loading the configuration is located in [./config/config.go](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/blob/master/config/config.yaml)
This file is read by this source code: https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/blob/master/test/util/config/config.go where a configuration structure is defined. 

Additional parameters are to be added under the test package in a subdir suffixed by parameters like:

[test/ran/cpu/rancpuparameters/rancpuparameters.go](https://gitlab.cee.redhat.com/cnf/cnf-gotests/-/blob/master/test/ran/cpu/rancpuparameters/rancpuparameters.go)

### Code conventions
#### Lint
Push requested are tested in a pipeline with golangci-lint. It is advised to add [Golangci-lint integration](https://golangci-lint.run/usage/integrations/) to your development editor.

#### Errors in helper functions
In the case of a helper function that encounters an unexpected response, the function should log an error with the function name the error happened to assist with later test failure analysis.
If the function can not resolve this error, it should return an error to the caller function, which should take action due to the error.

## Troubleshooting
### When running `ginkgo` I get this error: `flag provided but not defined: -ginkgo.timeout`

Reason: You installed ginkgo version 2+, and this repository supports only version 1.x
Fix: go install github.com/onsi/ginkgo/ginkgo@v1.16.5

### Error using k8s.io during development of new tests
Reason: Go version is below 1.17, and the k8s.io dependencies need go 1.17+
Fix: Download latest go https://go.dev/dl/ and open it in your home dir and add path to it: export PATH=~/go/bin:$PATH to your ~/.bashrc file and source this file.