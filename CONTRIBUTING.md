# Contributing

## Setup

Go is pinned in `mise.toml`. With [mise](https://mise.jdx.dev/) installed,
run Go through it so you get that version:

```bash
mise install
mise exec -- go build ./...
mise run dev                   # start a dev server with this checkout
mise exec -- go test -race ./...
```

## Before you open a pull request

```bash
mise exec -- gofmt -l .        # must print nothing
mise exec -- go vet ./...
mise exec -- go mod tidy -diff
mise exec -- go test -race ./... -count=1
mise run build
```

CI runs the same checks on every pull request.

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
