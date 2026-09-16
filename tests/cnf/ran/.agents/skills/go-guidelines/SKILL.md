---
name: go-guidelines
description: Use this skill when writing or review Go code to ensure it is modern and idiomatic.
---

# go-guidelines

Modernization and style guidelines for Go code.

## modernization

- do not use `interface{}` now that `any` exists
- use `for i := range 6` syntax instead of `for i := 0; i < 6; i++`
- write doc comments for all functions, including unexported ones. do not write comments for `TestXXX` functions in `_test.go` files, however
- avoid overly abbreviated variable names, except for very common conventions. `i` for index is okay. but say `options` instead of `opts`, for example. `idx` should be avoided, similarly.
- loop variable capturing is solved now. each iteration receives its own variable, rather than it being shared
- do not use `ptr.To` or similar helpers anymore. `new("mystring")` and `new(42)` are valid Go now
- use generic versions of stdlib functions where available. `slices` and `maps` are useful stdlib libraries here

## style

- include a comment for regexp usage explaining what a matching string is expected to look like
- default to using testify for unit tests
- avoid complex trees of unexported functions. if a function is only called once or twice or is only a few lines long, it can often be inlined. this is not a hard rule, but something to keep in mind
- when using Gomega, always include a description for assertions. instead of `Expect(err).ToNot(HaveOccurred())`, do something like `Expect(err).ToNot(HaveOccurred(), "Failed to pull resource %s", resourceName)`
- add `//go:build unit_test` to unit test files. use `make test` to run tests
