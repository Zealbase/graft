---
sidebar_position: 2
title: Install
---

# Install

How to install, verify, upgrade, and remove the `graft` binary.

:::info Planned
Release artifacts (Homebrew tap, npm wrapper, prebuilt binaries) are **planned**. The distribution name is being pre-flight checked on the Go module path, Homebrew, and npm before release. Until then, build from source.
:::

## Install methods

### From source (available)

```bash
go install <module-path>/cmd/graft@latest   # module path set at release
```

This builds the `graft` binary into your `$GOBIN` (usually `~/go/bin`). Ensure that directory is on your `PATH`.

### Homebrew (planned)

```bash
brew install graft
```

### npm (planned)

```bash
npm install -g graft
```

## Verify

```bash
graft --version
graft --help
```

## Upgrade

Re-run your install method. From source:

```bash
go install <module-path>/cmd/graft@latest
```

## Uninstall

Remove the binary from your `$GOBIN`/`PATH`:

```bash
rm "$(command -v graft)"
```

graft also keeps a per-project store at `.graft/` and a global sqlite database. To fully remove graft state from a project, delete its `.graft/` directory. (The global database location is documented in the [FAQ](../reference/faq.md).)

## Next

- [Quickstart](./quickstart.md)
- [Core concepts](../concepts/overview.md)
