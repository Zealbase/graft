---
sidebar_position: 2
title: Install
description: Install, verify, upgrade, and uninstall the graft CLI binary on macOS and Windows.
---

# Install

How to install, verify, upgrade, and remove the `graft` binary.

## Install methods

### From source

```bash
go install github.com/Shaik-Sirajuddin/graft/cmd/graft@latest
```

This builds the `graft` binary into your `$GOBIN` (usually `~/go/bin`). Ensure that directory is on your `PATH`.

## Verify

```bash
graft --version
graft --help
```

---

## Upgrade

### Using `graft update`

The easiest way to upgrade an existing installation:

```bash
graft update
```

This checks the GitHub releases API and installs a newer binary if one is available. To check without installing:

```bash
graft update --check
```

`graft update` works outside an initialized workspace — it does not need a `.graft/` directory.

### Manually (from source)

```bash
go install github.com/Shaik-Sirajuddin/graft/cmd/graft@latest
```

---

## Uninstall

Remove the binary from your `$GOBIN`/`PATH`:

```bash
rm "$(command -v graft)"
```

graft also keeps a per-project store at `.graft/` and a global sqlite database. To fully remove graft state from a project, run `graft destroy` first (this removes the workspace row from the global database and the `.graft/` directory while keeping your provider files), then delete `.graft/` if anything remains:

```bash
graft destroy --yes
```

See `graft destroy --help` and the [CLI reference](../reference/cli.md#graft-destroy).

---

## Platform notes

macOS and Windows are both supported. On Windows, symlink creation for skills requires Developer Mode or Administrator privileges. Without those, `graft skill` commands report a capability error rather than silently creating a file copy. See [Skills](../concepts/skills.md).

## Outbound network access

- `graft update` and `graft update --check` contact `api.github.com/repos/Shaik-Sirajuddin/graft/releases/latest`.
- The embedded catalog is offline-first; `graft catalog verify` runs entirely offline.
- Some providers' model resolution may contact `models.dev` at build time (not at runtime).

See [Endpoints](../reference/endpoints.md) for the full list.

## Next

- [Quickstart](./quickstart.md)
- [Core concepts](../concepts/overview.md)

---

## Additional install methods (planned)

### Homebrew (planned)

```bash
brew install graft
```

### npm (planned)

```bash
npm install -g graft
```
