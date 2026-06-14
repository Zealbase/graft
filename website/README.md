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

## Docs versioning

Released docs are versioned with Docusaurus. The current/in-progress docs live
in `docs/`; each frozen release is snapshotted under `versioned_docs/` with its
sidebar in `versioned_sidebars/` and listed in `versions.json`.

Cut a new version when you ship a release:

```bash
pnpm docusaurus docs:version <version>   # e.g. 0.0.6
```

This copies the current `docs/` into `versioned_docs/version-<version>/`. Commit
the generated `versioned_docs/`, `versioned_sidebars/`, and `versions.json` —
the deploy build ships every committed version, and the version dropdown in the
navbar lets readers switch. Keep editing `docs/` for the next release; existing
versions stay frozen unless you intentionally edit their snapshot.

## Deployment

The site is hosted on **AWS S3 + CloudFront** (default `*.cloudfront.net`
domain, no custom domain). Infrastructure is defined in
[`infra/docs-site.cfn.yaml`](../infra/docs-site.cfn.yaml) (CloudFormation) and
deploys automatically via [`.github/workflows/deploy-docs.yml`](../.github/workflows/deploy-docs.yml)
on every push to the `docs` branch.

Live URL: https://ddiyw5xqx0hu3.cloudfront.net/

### Manual deploy

```bash
pnpm build
aws s3 sync build/ s3://graft-docs-site-101541766996/ --delete --region ap-south-2
aws cloudfront create-invalidation --distribution-id E1QVJYMRGE45D1 --paths "/*"
```

### Provisioning / updating infra

```bash
aws cloudformation deploy \
  --region ap-south-2 \
  --stack-name graft-docs-site \
  --template-file infra/docs-site.cfn.yaml \
  --no-fail-on-empty-changeset
```

The stack is **strictly S3 + CloudFront** (private bucket via Origin Access
Control, caching enabled, default CloudFront cert). No Route53, ACM, WAF,
Shield, or logging buckets — nothing that adds cost beyond free-tier-eligible
S3 storage and CloudFront usage.

The GitHub Actions workflow reads AWS credentials from repository **secrets**
(`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) and never hardcodes keys.
Optional repository **variables** (`AWS_REGION`, `DOCS_BUCKET`,
`DOCS_DISTRIBUTION_ID`) override the provisioned defaults.
