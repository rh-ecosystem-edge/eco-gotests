
BASE_IMG ?= eco-gotests
BASE_TAG ?= latest

GO_PACKAGES=$(shell go list ./... | grep -v vendor)
.PHONY: vet lint deps-update sync-eco-goinfra install-ginkgo build-docker-image-ran-du install test coverage-html run-internal-pkg-unit-tests run-ran-pkg-unit-tests run-system-tests-pkg-unit-tests
vet:
	go vet ${GO_PACKAGES}

lint:
	@echo "Running go lint"
	scripts/golangci-lint.sh

deps-update:
	go mod tidy && \
	go mod vendor

# ECO_GOINFRA_BRANCH: optional branch to sync from (e.g. release-4.20). Empty = default branch.
ECO_GOINFRA_BRANCH ?=
sync-eco-goinfra:
ifneq ($(ECO_GOINFRA_BRANCH),)
	go get github.com/rh-ecosystem-edge/eco-goinfra@$(ECO_GOINFRA_BRANCH)
else
	go get github.com/rh-ecosystem-edge/eco-goinfra
endif
	go mod tidy
	go mod vendor

install-ginkgo:
	scripts/install-ginkgo.sh

build-docker-image:
	@echo "Building docker image"
	podman build -t "${BASE_IMG}:${BASE_TAG}" -f Dockerfile

build-docker-image-ran-du: build-docker-image
	@echo "Building docker image for RAN DU tests"
	podman build --build-arg=BASE_IMG="${BASE_IMG}" --build-arg=BASE_TAG="${BASE_TAG}" -t eco-gotests-ran-du:latest -f images/system-tests/ran-du/Dockerfile

install: deps-update install-ginkgo
	@echo "Installing needed dependencies"

run-tests:
	@echo "Executing eco-gotests test-runner script"
	scripts/test-runner.sh

run-internal-pkg-unit-tests:
	@echo "Executing eco-gotests internal package unit tests"
	UNIT_TEST=true go test -v -tags=unit_test ./tests/internal/...

run-ran-pkg-unit-tests:
	@echo "Executing eco-gotests RAN package unit tests"
	UNIT_TEST=true go test -v -tags=unit_test ./tests/cnf/ran/internal/... ./tests/cnf/ran/.../internal/...

run-system-tests-pkg-unit-tests:
	@echo "Executing eco-gotests internal package unit tests"
	UNIT_TEST=true go test -v -tags=unit_test ./tests/system-tests/internal/... ./tests/system-tests/.../internal/...

# Rather than run each unit test target individually, run all unit tests in one go.
test:
	@echo "Executing eco-gotests unit tests"
	UNIT_TEST=true go test -v -tags=unit_test ./tests/internal/... ./tests/.../internal/...

coverage-html: test
	go tool cover -html cover.out
