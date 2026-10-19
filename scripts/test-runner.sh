#!/usr/bin/env bash

GOPATH="${GOPATH:-~/go}"
PATH=$PATH:$GOPATH/bin
TEST_DIR="./tests"

# Check that ECO_TEST_FEATURES environment variable has been set
if [[ -z "${ECO_TEST_FEATURES}" ]]; then
    echo "ECO_TEST_FEATURES environment variable is undefined"
    exit 1
fi

# Set feature_dirs to top-level test directory when "all" feature provided
if [[ "${ECO_TEST_FEATURES}" == "all" ]]; then
    feature_dirs=${TEST_DIR}
else
    # Find all test directories matching provided features
    for feature in ${ECO_TEST_FEATURES}; do
        discovered_features=$(find $TEST_DIR -depth -name "${feature}" -not -path '*/internal/*' 2> /dev/null)
        if [[ ! -z $discovered_features ]]; then
            feature_dirs+=" "$discovered_features
        else
            if [[ "${ECO_VERBOSE_SCRIPT}" == "true" ]]; then
                echo "Could not find any feature directories matching ${feature}"
            fi
        fi
    done

    if [[ -z "${feature_dirs}" ]]; then
        echo "Could not find any feature directories for provided features: ${ECO_TEST_FEATURES}"
        exit 1
    fi

    if [[ "${ECO_VERBOSE_SCRIPT}" == "true" ]]; then
        echo "Found feature directories:"
        for directory in $feature_dirs; do printf "$directory\n"; done
    fi
fi


# Build ginkgo command
cmd="ginkgo -timeout=24h --keep-going --require-suite -r"
hwol_suite=false

if [[ "${ECO_TEST_VERBOSE}" == "true" ]]; then
    cmd+=" -vv"
fi

if [[ "${ECO_TEST_TRACE}" == "true" ]]; then
    cmd+=" --trace"
fi

if [[ ! -z "${ECO_TEST_LABELS}" ]]; then
    cmd+=" --label-filter=\"${ECO_TEST_LABELS}\""
fi
# HWOL changes SR-IOV/MCP state and performs a potentially long, synchronized
# foundation wait.  Running it with the repository-wide parallelism leaves
# non-primary Ginkgo workers blocked until their launcher kills them, which
# produces the misleading "parallel process disappeared" failure.  Keep the
# suite serial unless a caller explicitly opts into parallel execution.
if [[ " ${ECO_TEST_FEATURES} " == *" hwol "* ]]; then
    hwol_suite=true
fi

if [[ "$hwol_suite" == "true" && " ${ECO_HWOL_ALLOW_PARALLEL:-false} " != " true "* ]]; then
    cmd+=" $@ --procs=1 $feature_dirs"
else
    cmd+=" $@ $feature_dirs"
fi

# Execute ginkgo command
echo $cmd
set +e
eval $cmd
test_status=$?

# Ginkgo cannot run its after-suite hooks when its process is interrupted. Run
# the same ownership-checked cleanup as a separate process so an exited Ginkgo
# launcher still leaves the HWOL lane in a verified legacy-mode state. Jenkins
# should also invoke this command from an unconditional post step to cover an
# interrupted runner process.
cleanup_status=0
if [[ "$hwol_suite" == "true" ]]; then
    echo "Running independent HWOL post-run cleanup verification"
    go run ./tests/ocp/hwol/cmd/cleanup
    cleanup_status=$?
fi

if [[ $test_status -ne 0 ]]; then
    exit $test_status
fi

exit $cleanup_status
