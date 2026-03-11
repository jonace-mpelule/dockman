# Release Workflow

This project uses a simple Makefile-driven release flow that records the version in a `VERSION` file, creates a git commit, tags the release, and publishes to GitHub.

## Prerequisites
- Ensure your local branch has the changes you want to release.
- Ensure the working tree is clean (no uncommitted changes).

## Step 1: Run Tests (Recommended)
```bash
go test ./...
```

## Step 2: Build (Optional)
```bash
make build
```
This creates multi-platform binaries in `dist/` for Linux, macOS, and Windows (amd64/arm64 where applicable).

## Step 3: Set Version (Commit + Tag)
This writes the `VERSION` file, commits it, and creates an annotated git tag.

```bash
make set-version VERSION=v0.2.0
```

What this does:
- Writes `VERSION` with the provided value.
- Adds and commits `VERSION` with message `chore: release v0.2.0`.
- Creates an annotated tag `v0.2.0`.

## Step 4: Publish
Pushes your current branch and the tag to GitHub.

```bash
make publish VERSION=v0.2.0
```

What this does:
- Verifies the working tree is clean.
- Pushes the current branch to `origin`.
- Pushes the tag `v0.2.0` to `origin`.

## Full One-by-One Example
```bash
# 1) test

go test ./...

# 2) build

make build

# 3) set version (commit + tag)

make set-version VERSION=v0.2.0

# 4) publish

make publish VERSION=v0.2.0
```

## Notes
- `make build` embeds the current git tag (or `VERSION` if set) into the binary via `-ldflags` so `dockman --version` prints the release version.
- If you attempt to set the same version twice, `make set-version` will fail with `VERSION unchanged; aborting.`
- `make publish` requires a clean working tree to prevent accidental publishing of uncommitted changes.
