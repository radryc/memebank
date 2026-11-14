# memebank Guardian partition

This directory is the repo-authored deployment definition for
<https://github.com/radryc/memebank>.

Guardian's repo-import loop reads `guardian/partitions/memebank` through MonoFS
(`/.repos/github.com/radryc/memebank`). MonoFS auto-refreshes the git repository,
so a push to `main` that changes this directory is imported and reconciled
automatically.

## Deploy

The repository and partition are registered once:

```bash
monofs-admin ingest --router 127.0.0.1:9090 \
  --source https://github.com/radryc/memebank --ref main \
  --fetch-type git --replicate-data --token "$MONOFS_TOKEN"

guardianctl --monofs-router 127.0.0.1:9090 --monofs-token "$MONOFS_TOKEN" \
  repo add --name memebank --pusher k8s-main --partition memebank \
  --source https://github.com/radryc/memebank.git
```

The partition targets the `k8s-main` pusher and exposes the app through
`lb-edge` on external port **8088** (service annotations in
`payloads/memebank.k8s.yaml`).

## Image updates

After changing application code, run:

```bash
./scripts/release-image.sh
```

It builds and pushes `registry.strata.local:5000/memebank:<git-sha>` and updates
`intents/images.yaml`. Commit and push that change; the watcher rebuilds the
Guardian partition and rolls the deployment.
