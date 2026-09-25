---
name: unit-tests
description: Use this skill when writing and running unit tests. Does not apply to Ginkgo tests.
---

eco-gotests has two types of tests: unit tests and Ginkgo tests. Unit tests always have an `/internal/` directory in the path.

Unit tests are written using the `testify` package, not Ginkgo. They do not use `testing.T` methods for assertions, instead using the `testify` package.

Unit tests may be run using the `make test` command. More targeted unit tests may be run with `UNIT_TEST=true go test -v -tags=unit_test <path to package>`. You must set `UNIT_TEST=true` and add the `unit_test` tag, otherwise tests will be skipped or you will hit initialization errors.

All unit tests should have a `//go:build unit_test` directive at the top of the file. Never add `!unit_test` to the build tags.
