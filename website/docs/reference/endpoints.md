---
sidebar_position: 6
title: Endpoints
description: All outbound network endpoints contacted by graft at runtime — for firewall allowlisting and air-gapped environments.
---

# Endpoints

Outbound network endpoints contacted by graft at runtime.

## `graft update` / `graft update --check`

| Endpoint | Protocol | Purpose |
|----------|----------|---------|
| `https://api.github.com/repos/Shaik-Sirajuddin/graft/releases/latest` | HTTPS GET | Fetch the latest release tag to compare against the running binary version. |

This is the only endpoint contacted at runtime. The request has a 10-second timeout. If it fails, `graft update` returns an error and does not modify the binary.

## `graft catalog verify`

No network access. The catalog is embedded in the binary at compile time; `verify` recomputes hashes offline.

## Sync and agent commands

No outbound network. Sync, validate, status, and config commands operate entirely on local files and the local sqlite database.

## Skill commands

No outbound network. Skill operations resolve paths and create symlinks on the local filesystem.

## Build-time model data

Some provider packages resolve model lists from `models.dev` at build time (not at runtime) when generating the embedded catalog. This is a developer-facing concern, not a user-facing one — end users never contact `models.dev` at runtime.

## Firewall / proxy notes

For `graft update` to work, `api.github.com` must be reachable on port 443. In air-gapped environments, use `graft update --check` to confirm you are up to date and install new binaries manually.

## Related

- [Install — upgrade](../getting-started/install.md#upgrade)
- [CLI reference — graft update](./cli.md#graft-update)
- [CLI reference — graft catalog verify](./cli.md#graft-catalog-verify)
