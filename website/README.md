# Website

This site is built with [Docusaurus](https://docusaurus.io/), a modern static site generator.

The package manager for this site is **pnpm** (lockfile: `pnpm-lock.yaml`). Do not use npm or yarn — they would create competing lockfiles. A recent **Node.js** (>=20) must be installed; Docusaurus executes on Node.

## Installation

```bash
pnpm install
```

The first install allows postinstall build scripts declared in `pnpm-workspace.yaml`
(`allowBuilds`) — `@swc/core` (used by `@docusaurus/faster`) and `core-js`.

## Local development

```bash
pnpm start
```

Starts a local dev server and opens a browser window. Most changes reload live.

## Build

```bash
pnpm run build
```

Generates static content into the `build` directory, servable by any static host.
`onBrokenLinks` and `onBrokenMarkdownLinks` are set to `throw`, so a successful
build also confirms there are no broken internal links.

## Serve a production build

```bash
pnpm run serve
```

## Deployment

Hosting is **not yet bound** — `url`/`baseUrl` in `docusaurus.config.ts` are
placeholders, and no deploy workflow is committed. Set them at deploy time.
