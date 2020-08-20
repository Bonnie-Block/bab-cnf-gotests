# Export GO111MODULE=on to enable project to be built from within GOPATH/src
export GO111MODULE=on

govet:
	@echo "Running go vet"
	# Disabling GO111MODULE just for go vet execution
	GO111MODULE=off go vet gitlab.cee.redhat.com/cnf/cnf-gotests/test...

golint:
	@echo "Running go lint"
	hack/lint.sh

deps-update:
	go mod tidy && \
	go mod vendor

gofmt:
	@echo "Running gofmt"
	gofmt -s -l `find . -path ./vendor -prune -o -type f -name '*.go' -print`

test-all:
	./hack/run-tests.sh all

test-features:
	FEATURES="$(FEATURES)" ./hack/run-tests.sh features 
