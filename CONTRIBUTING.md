## Setting up the environment

To set up the repository, run:

```sh
$ ./scripts/bootstrap
$ ./scripts/lint
```

This will install all the required dependencies and build the SDK.

You can also [install go 1.25+ manually](https://go.dev/doc/install).

## Modifying/Adding code

Most of the SDK is generated code. Modifications to code will be persisted between generations, but may
result in merge conflicts between manual patches and changes from the generator. The generator will never
modify the contents of the `lib/` and `examples/` directories.

## Adding and running examples

All files in the `examples/` directory are not modified by the generator and can be freely edited or added to.

```go
# add an example to examples/<your-example>/main.go

package main

func main() {
  // ...
}
```

```sh
$ go run ./examples/<your-example>
```

## Using the repository from source

To use a local version of this library from source in another project, edit the `go.mod` with a replace
directive. This can be done through the CLI with the following:

```sh
$ go mod edit -replace github.com/limrun-inc/go-sdk=/path/to/go-sdk
```

## Running tests

Most tests require you to [set up a mock server](https://github.com/stoplightio/prism) against the OpenAPI spec to run the tests.

```sh
# you will need npm installed
$ npx prism mock path/to/your/openapi.yml
```

```sh
$ ./scripts/test
```

## Formatting

This library uses the standard gofmt code formatter:

```sh
$ ./scripts/format
```

## Publishing and releases

Go modules are published by tag. Merging a commit to `main` with a releasable [Conventional Commits](https://www.conventionalcommits.org)
title (`feat:`, `fix:` and so on) makes [the `Release Please` workflow](https://www.github.com/limrun-inc/go-sdk/actions/workflows/release-please.yml)
open or update a `release: x.y.z` pull request. It bumps the version in `internal/version.go`, `README.md` and the changelog.
Merging that pull request creates the `vX.Y.Z` tag and its GitHub release.

The release then runs [the `Publish Go` workflow](https://www.github.com/limrun-inc/go-sdk/actions/workflows/publish-go.yml). It checks
that the tagged files report the tag's version, attaches a source SBOM to the release, and fetches the version through
`proxy.golang.org` so it appears on pkg.go.dev. If it fails, run it again from the Actions tab with the release tag.
