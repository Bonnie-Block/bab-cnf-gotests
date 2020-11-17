#!/usr/bin/env bash
GOPATH="${GOPATH:-~/go}"
export PATH=$PATH:$GOPATH/bin

function run_tests {
    case $1 in
        all)
            echo "#### Run all tests ####"
            ginkgo -v --keepGoing -requireSuite -r test/
            ;;
        features)
            if [ -z "$FEATURES" ]; then {
                echo "FEATURES env var is empty. Please export FEATURES"
                exit 1
            } fi
            echo "#### Run feature tests: ${FEATURES} ####"
            for feature in ${FEATURES}
                do
                    for dir in test/*/*; do
                    if [[ $dir != *"util"* ]] && [[ $dir == *"${feature}"* ]]; then {
                        command+=" "$dir
                    } fi
                    done
                done
            ginkgo -v --keepGoing -requireSuite $command
            ;;
        *)
        echo "Unknown case"
        exit 1
        ;;
    esac
}

run_tests ${1}
