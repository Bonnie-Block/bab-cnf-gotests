#!/usr/bin/env bash

set -e
. $(dirname "$0")/common.sh

if ! which go; then
  echo "No go command available"
  exit 1
fi

GOPATH="${GOPATH:-~/go}"
export GOFLAGS="${GOFLAGS:-"-mod=vendor"}"

export PATH=$PATH:$GOPATH/bin

mkdir -p cnf-gotests/bin
go test -c cnf-gotests/test/testcmd_suite_test.go -o ./cnf-gotests/bin/testcmd.test 
