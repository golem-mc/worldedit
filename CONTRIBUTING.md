# Contributing

## Setup

The toolchain (Go, golangci-lint, gotestsum) is pinned in `mise.toml`. With
[mise](https://mise.jdx.dev/) installed, run Go through it so you get those
versions:

```bash
mise install
mise run build
mise run dev                   # start a dev server with this checkout
```

## Before you open a pull request

Run the cheap checks:

```bash
mise run fmt && mise run fix && mise run tidy
mise run build
mise run dev                   # start a dev server with this checkout
mise run lint
```

`fmt:check`, `fix:check` and `tidy:check` only verify, which is what CI runs.

For tests, run the ones you added or changed, then repeat them to catch flakes:

```bash
mise exec -- go test ./core -run 'TestA|TestB'
mise run test:changed
```

There is no need to run the whole suite locally (`mise run test` and
`mise run check` do); CI runs it on every pull request, alongside the Build,
Lint, Fix, Format and Tidy jobs. Tests have to be cheap, because every pull
request waits for the whole suite. A test that fails only on a busy machine is
a flaky test: fix it rather than retrying it.

## Commit messages and releases

Pull requests are squash-merged with their title as the commit message, and
the title must be a [conventional commit](https://www.conventionalcommits.org/):

```
feat(brushes): add erode preset parameters
fix(storage): preserve entity offsets in Sponge v3 schematics
docs: document rollback database configuration
feat!: change API.NewEditSession signature
```

Use the scope for the part you changed: `core`, `region`, `expr`, `block`,
`storage`, `tools`, or `commands`. A check on each pull request rejects a
title that isn't a conventional commit.

Releases are made by [release-please](https://github.com/googleapis/release-please).
It keeps a release pull request open that bumps the version and updates
`CHANGELOG.md` from the commits on `main`. Merging it tags the release
(`vX.Y.Z`), which is the version servers put in `launcher.json`. Before
1.0.0, `feat` raises the minor version and `fix` the patch version; a
breaking change (`!`) raises the minor version.
