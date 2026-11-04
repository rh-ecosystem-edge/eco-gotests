#!/usr/bin/env bash

set -euo pipefail

GOPATH="${GOPATH:-~/go}"
PATH="${PATH}:${GOPATH}/bin"
readonly TEST_DIR="./tests"

# Check that ECO_TEST_FEATURES environment variable has been set
if [[ -z "${ECO_TEST_FEATURES:-}" ]]; then
    echo "ECO_TEST_FEATURES environment variable is undefined"
    exit 1
fi

# Set feature_dirs to top-level test directory when "all" feature provided
if [[ "${ECO_TEST_FEATURES}" == "all" ]]; then
    feature_dirs=("${TEST_DIR}")
else
    # Read ECO_TEST_FEATURES into an array of features, whitespace-delimited.
    # Newlines are converted to spaces to ensure a newline-delimited list of
    # features is properly parsed.
    read -r -a features <<< "${ECO_TEST_FEATURES//$'\n'/ }"

    # feature_dirs will contain the full list of paths to provide to the ginkgo
    # command.
    feature_dirs=()

    # For each feature, find all directories matching the feature and add them
    # to the discovered_features array.
    for feature in "${features[@]}"; do
        # Strip leading "./" and trailing "/" from the feature.
        feature="${feature#./}"
        feature="${feature%/}"

        # discovered_features will contain the list of directories matching this
        # feature. It is eventually added to the feature_dirs array.
        discovered_features=()

        # We use `find` to get a NUL-delimited list of directories under
        # TEST_DIR, excluding /internal/ directories.
        while IFS= read -r -d '' directory; do
            # Strip the TEST_DIR prefix from the directory.
            relative_directory="${directory#"${TEST_DIR}/"}"

            # If this test directory matches the feature name or the feature
            # name is a subdirectory of this test directory, add it to the
            # discovered_features array.
            if [[ "${relative_directory}" == "${feature}" ||
                  "${relative_directory}" == */"${feature}" ]]; then
                discovered_features+=("${directory}")

                if [[ "${ECO_VERBOSE_SCRIPT:-}" == "true" ]]; then
                    echo "Found directory: ${directory} matching feature: ${feature}"
                fi
            fi
        done < <(find "${TEST_DIR}" -type d -not -path '*/internal/*' -print0 2>/dev/null)

        # If any features were found, add them to the feature_dirs array.
        if ((${#discovered_features[@]})); then
            feature_dirs+=("${discovered_features[@]}")
        elif [[ "${ECO_VERBOSE_SCRIPT:-}" == "true" ]]; then
            echo "Could not find any feature directories matching ${feature}"
        fi
    done

    if [[ "${#feature_dirs[@]}" -eq 0 ]]; then
        echo "Could not find any feature directories for provided features: ${ECO_TEST_FEATURES}"
        exit 1
    fi

    if [[ "${ECO_VERBOSE_SCRIPT:-}" == "true" ]]; then
        echo "Found feature directories:"
        for directory in "${feature_dirs[@]}"; do
            echo "${directory}"
        done
    fi
fi


# Build ginkgo command as an array of strings.
cmd=(ginkgo -timeout="${ECO_TEST_TIMEOUT:-24h}" --keep-going --require-suite -r)

if [[ "${ECO_TEST_VERBOSE:-}" == "true" ]]; then
    cmd+=(-vv)
fi

if [[ "${ECO_TEST_TRACE:-}" == "true" ]]; then
    cmd+=(--trace)
fi

if [[ -n "${ECO_TEST_LABELS:-}" ]]; then
    cmd+=("--label-filter=${ECO_TEST_LABELS}")
fi

# Add user args before feature dirs. This allows users to pass flags directly
# to the ginkgo command. For example, `./scripts/test-runner.sh --dry-run` will
# run Ginkgo in dry-run mode.
cmd+=("$@" "${feature_dirs[@]}")

# Execute ginkgo command
printf 'Executing:'
printf ' %q' "${cmd[@]}"
printf '\n'
exec "${cmd[@]}"
