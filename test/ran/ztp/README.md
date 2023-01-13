# Ztp Tests

## Test Preconfiguration

Running the ZTP tests requires some configuration to be completed ahead of time.
    1. The following environment variables must be defined:additional environment variables to be configured, namely;
        A. $KUBECONFIG: The kubeconfig path for the spoke. If this is not defined then the tests will be skipped.
        B. $KUBECONFIG_HUB: The kubeconfig path for the hub. If this is not defined then the tests will be skipped.
    2. The ArgoCD policies application must be pre-configured to have the correct git repository, branch, and base folder. The tests will use the base folder and then add the paths to the specific test files
    for each test under ztp-test/ subfolder.

## Running the tests

The ZTP tests can be executed by using the features flag, e.g. `export FEATURES="ztp"` and run `make test-features`
