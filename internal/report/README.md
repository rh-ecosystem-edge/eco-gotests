# ginkgo report generator

Generate HTML reports for the tree of Ginkgo specs. [Available on GitHub pages.]

[Available on GitHub pages.]: https://rh-ecosystem-edge.github.io/eco-gotests/report

## Usage

```
go run ./internal/report [flags]
```

Documentation may be viewed using the following command:

```
go doc ./internal/report
```

### Examples

For viewing the tree for a single branch while printing all logs:

```
go run ./internal/report -v 100 -b <branch name>
```

For viewing the trees for all branches without printing any logs:

```
go run ./internal/report -b 'main release-*'
```

For viewing the trees and generating an html report:

```
go run ./internal/report -b main -o <report output directory>
```

## Developing

### Architecture

Although this consists entirely of a single Go package, it generally treats each file as its own package when it comes to exported vs unexported values. Unexported values are generally meant to be used in the file they are defined whereas exported values are meant for reuse by other files.

For this purpose, the program is split into the following files:

* `cache.go`: Contains the Cache type and manages the cache directory. This allows the program to only do a Ginkgo dry run when either the program source or the branch is updated.
* `command.go`: Wrapper around local commands, such as various git and ginkgo commands.
* `main.go`: Entrypoint for the program that has the doc comment, handles command line flags, and orchestrates report caching and generation.
* `sum.go`: Generates a SHA-256 sum of the program source code used for validating cache. This guarantees that invalid cache formats will not be loaded.
* `template.go`: Configs and functions for generating reports based on `report_template.html` and `tree_template.html`.
* `tree.go`: Defines the SuiteTree type representing the tree of specs in `tests/`.
* `report_template.html`: Template for the main page of a report listing the branches and revisions included therein.
* `tree_template.html`: Template for a single branch that contains a tree of all the specs.

### Program flow

1. Flags are parsed.
1. If help flag specified, help is printed and program exits.
1. If clean flag specified, cache is cleaned and program exits.
1. Trees are generated based on the branch flag.
    1. If branch flag nonempty, attempt to get trees for all branches matching the patterns. Trees not present in the cache get cloned and have a dry run performed.
    1. If branch flag empty, attempt to get trees from the repo in the current directory. Cache is checked for the current directory and a clone and dry run is performed if necessary.
    1. Once updated, the cache is saved before any processing of the trees.
    1. Trees are trimmed and sorted to clean them up for displaying.
1. Trees are printed to stdout.
1. If output flag nonempty, the generated tree map is used to fill in the templates.

### GitHub workflow

On push to the main or release branches, the GitHub workflow runs with this flow:

1. Setup environment, including checking out the default branch and installing go and ginkgo.
1. Attempt to restore cache.
1. Run the program on main and release branches. A report is generated and the current action URL provided.
1. The generated report is uploaded as an artifact in the format necessary for deploying to pages.
1. Attempt to save cache.
1. Deploy the generated pages artifact.

With this workflow, the entire report is generated on every run, ensuring all branch reports stay up to date with the latest template. A dry run is only performed for the branch that changed or all branches if the source code changes. Since changes to the report program are much rarer than other changes to the branches, the total runtime should be about the same as just the dry run step in the existing Makefile CI.
