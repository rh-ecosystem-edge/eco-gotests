#!/usr/bin/env bash

set -euo pipefail

# Use explicit local package directories for the change under test.
if (( $# == 0 )); then
  set -- ./tests/internal/klogfilter
fi

shopt -s nullglob
repo_root="$(pwd -P)"
packages=()
gofmt_files=()
for package in "$@"; do
  if [[ ${package} != ./* || ! -d ${package} ]]; then
    printf 'Expected a local Go package directory, got %s\n' "${package}" >&2
    exit 2
  fi

  resolved="$(realpath -e -- "${package}")"
  if [[ "${resolved}" != "$repo_root" && "${resolved}" != "$repo_root/"* ]]; then
    printf 'Package directory resolves outside the repository: %s\n' "${package}" >&2
    exit 2
  fi
  if [[ "${resolved}" != "$(realpath -s -- "${package}")" ]]; then
    printf 'Symlinked package directories are unsupported: %s\n' "${package}" >&2
    exit 2
  fi

  package=".${resolved#"$repo_root"}"
  package_files=("${package}"/*.go)
  if (( ${#package_files[@]} == 0 )); then
    printf 'No Go files in %s\n' "${package}" >&2
    exit 2
  fi
  for file in "${package_files[@]}"; do
    if [[ -L "${file}" ]]; then
      printf 'Symlinked Go files are unsupported: %s\n' "${file}" >&2
      exit 2
    fi
  done
  packages+=("${package}")
  gofmt_files+=("${package_files[@]}")
done

export GOTOOLCHAIN=local UNIT_TEST=true

required_toolchain=""
if [[ -f go.mod ]]; then
  required_toolchain="$(awk '$1 == "toolchain" { print $2; exit }' go.mod)"
fi

if [[ -n "${required_toolchain}" ]]; then
  current_go="$(go version | awk '{print $3}')"
  if [[ "${current_go}" != "${required_toolchain}" ]]; then
    printf 'Fullsend validation requires %s; found %s. Use a code image with the repository toolchain preinstalled.\n' \
      "${required_toolchain}" "${current_go}" >&2
    exit 2
  fi
fi

gofmt_output="$(gofmt -l "${gofmt_files[@]}")"
if [[ -n "${gofmt_output}" ]]; then
  printf 'gofmt would modify:\n%s\n' "${gofmt_output}" >&2
  exit 1
fi

go vet -tags=unit_test "${packages[@]}"

test_log="$(mktemp)"
trap 'rm -f -- "$test_log"' EXIT

test_status=0
go test -json -count=1 -tags=unit_test "${packages[@]}" > "$test_log" || test_status=$?
jq -j '.Output // empty' "$test_log"

test_counts="$(jq -s '
  reduce (.[] | select(.Package != null)) as $event ({};
    .[$event.Package] += (
      if $event.Test != null and ($event.Action == "pass" or $event.Action == "fail")
      then 1 else 0 end
    )
  )' "$test_log")"
jq -r 'to_entries[] | if .value == 0
  then "\(.key): zero tests completed"
  else "\(.key): \(.value) test/subtest results"
  end' <<< "$test_counts"

if (( test_status != 0 )); then
  exit "$test_status"
fi

if ! jq -e 'any(.[]; . > 0)' <<< "$test_counts" > /dev/null; then
  printf 'No unit tests completed in the selected packages.\n' >&2
  exit 1
fi
