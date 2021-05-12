#!/usr/bin/env bash

set -e

if ! git remote -v | grep https://gitlab.cee.redhat.com/cnf/cnf-gotests.git; then
  echo "upstream cnf/cnf-gotests is not connected"
  exit 1
fi

git remote update --prune
CURRENT_BRANCH=$(git branch | grep \* | cut -d ' ' -f2)
git checkout tests_image
if which docker; then
  docker build --no-cache -f cnf-gotests/Dockerfile -t cnf-gotests-client .
else
  podman build --no-cache -f cnf-gotests/Dockerfile -t cnf-gotests-client .
fi
git checkout $CURRENT_BRANCH
