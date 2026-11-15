# memebank Guardian partition

This directory is the repo-authored deployment definition for
<https://github.com/radryc/memebank>.

Guardian's repo-import loop reads `guardian/partitions/memebank` through MonoFS
(`/.repos/github.com/radryc/memebank`). MonoFS auto-refreshes the git repository,
so a push to `main` that changes this directory is imported and reconciled
automatically.

## Deploy

The partition is auto-discovered from the ingested repository. No Guardian
repo registration is required:

```bash
monofs-admin ingest --router 127.0.0.1:9090 \
  --source https://github.com/radryc/memebank --ref main \
  --fetch-type git --replicate-data --token "$MONOFS_TOKEN"
```

Guardian's repo-import watcher scans `/.repos` for `guardian/partitions/**` and
imports/reconciles new or changed bundles automatically. MonoFS auto-refresh
pulls future `main` commits, so after the first ingest a `git push` is enough.

The partition targets the `k8s-main` pusher and exposes the app through
`lb-edge` on external port **8088** (service annotations in
`payloads/memebank.k8s.yaml`).

## Access

The partition owns its RBAC in [`access.yaml`](access.yaml). Guardian imports
this file with the partition bundle, validates that every grant is scoped to
`memebank`, and merges it into the compiled access policy automatically. The
current grant makes `radryc@example.com` an admin on the partition, its logs and
traces, and the `github.com/*/memebank` repository:

```yaml
version: 1
users:
  radryc@example.com:
    partitions: [memebank]
    logs: [memebank]
    traces: [memebank]
    monofs: [github.com/*/memebank]
    role: admin
```

Repo-owned ACLs are deliberately narrow. Imports, defaults, selectors, quotas,
`aws_bindings`, clients, deny rules, wildcards, other partitions, and monofs
paths outside this repository are rejected by the importer. The platform ACL
remains the authority for platform-wide grants and defaults.

GitHub review ownership is declared in [`.github/CODEOWNERS`](../../../.github/CODEOWNERS)
(`* @radryc`). The `owner` label in `config.yaml` is informational.

## Image updates

After changing application code, run:

```bash
./scripts/release-image.sh
```

It builds and pushes `registry.strata.local:5000/memebank:<git-sha>` and updates
`intents/images.yaml`. Commit and push that change; the watcher rebuilds the
Guardian partition and rolls the deployment.
