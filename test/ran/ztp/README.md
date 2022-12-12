# Ztp Tests

## Test Preconfiguration

Running the ZTP tests requires some additional environment variables to be configured, namely;
    1. $ZTP_GIT_REPO: The url of the git repo that should be used for the test. By default this is `http://registry.kni-qe-0.lab.eng.rdu2.redhat.com:3000/kni-qe/ztp-site-configs.git`.
    2. $ZTP_GIT_BRANCH: The name of the git branch that should be used for the test. By default this is `worker-1-4.12`.
    3. $ZTP_GIT_DIR: The path to the folder that contains the test files for ztp. By default this is `ztp-site-configs/policygentemplates/ztp-test`.
    4. $KUBECONFIG: The kubeconfig path for the spoke. If this is not defined then the tests will be skipped.
    5. $KUBECONFIG_HUB: The kubeconfig path for the hub. If this is not defined then the tests will be skipped.

## Running the tests

The ZTP tests can be executed by using the features flag, e.g. `export FEATURES="ztp"` and run `make test-features`
