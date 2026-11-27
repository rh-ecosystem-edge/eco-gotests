#!/usr/bin/env bash

set -euo pipefail
export GOTOOLCHAIN=local

validator="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/fullsend-agent-validation.sh"
fixture="$(mktemp -d)"
trap 'rm -rf -- "$fixture"' EXIT
mkdir "$fixture/repo" "$fixture/outside"
cd -- "$fixture/repo"

go mod init example.com/validation
mkdir passing compileonly failing unformatted vetfailure
printf 'package compileonly\n' > compileonly/compileonly.go
printf 'package outside\n' > ../outside/outside.go
ln -s ../outside outside-link
ln -s passing passing-link
ln -s ../../outside/outside.go compileonly/linked.go
printf 'package unformatted\n\nfunc Example(){ }\n' > unformatted/unformatted.go

cat > passing/passing_test.go <<'GO'
//go:build unit_test

package passing

import "testing"

func TestPass(t *testing.T) {}
GO

cat > failing/failing_test.go <<'GO'
//go:build unit_test

package failing

import "testing"

func TestFail(t *testing.T) {
	t.Fatal("intentional fixture failure")
}
GO

cat > vetfailure/vetfailure_test.go <<'GO'
//go:build unit_test

package vetfailure

import (
	"fmt"
	"testing"
)

func TestInvalidFormat(t *testing.T) {
	fmt.Printf("%d", "not an integer")
}
GO

check() {
  local expected_status="$1"
  local expected_message="$2"
  shift 2

  local output
  local status=0
  output="$(bash "$validator" "$@" 2>&1)" || status=$?

  if [[ "$status" != "$expected_status" || "$output" != *"$expected_message"* ]]; then
    printf 'Expected exit %s and diagnostic %s; got exit %s:\n%s\n' \
      "$expected_status" "$expected_message" "$status" "$output" >&2
    exit 1
  fi
}

check 2 'Package directory resolves outside the repository' ./../outside
check 2 'Package directory resolves outside the repository' ./outside-link
check 2 'Symlinked package directories are unsupported' ./passing-link
check 2 'Symlinked Go files are unsupported' ./compileonly
unlink compileonly/linked.go

check 0 'example.com/validation/passing: 1 test/subtest results' ./passing
check 0 'example.com/validation/compileonly: zero tests completed' ./passing ./compileonly
check 1 'No unit tests completed in the selected packages.' ./compileonly
check 1 'intentional fixture failure' ./failing
check 1 $'gofmt would modify:\n./unformatted/unformatted.go' ./unformatted
check 1 'fmt.Printf format %d has arg' ./vetfailure

go mod edit -toolchain=go1.99.0
check 2 'Fullsend validation requires go1.99.0' ./passing

printf 'Fullsend validation regression tests passed.\n'
