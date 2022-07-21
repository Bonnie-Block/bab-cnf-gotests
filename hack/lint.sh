#!/usr/bin/env bash
set -e

. $(dirname "$0")/common.sh

if which golangci-lint; then
	echo "golint installed"
else
	echo "Downloading golint tool"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.47.2
fi

golangci-lint run -v
