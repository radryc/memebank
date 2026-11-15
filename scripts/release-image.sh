#!/usr/bin/env bash
# Build, push and pin the memebank image used by the Guardian partition.
#
#   ./scripts/release-image.sh [tag]
#
# The default tag is the current git short SHA. The script updates
# guardian/partitions/memebank/intents/images.yaml; commit and push that change
# and the MonoFS auto-refresh + Guardian repo-import watcher redeploys the app.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-$(git -C "$ROOT" rev-parse --short HEAD)}"
REGISTRY="${MEMEBANK_REGISTRY:-registry.strata.local:5000}"
REF="${REGISTRY}/memebank:${TAG}"
INTENT="${ROOT}/guardian/partitions/memebank/intents/images.yaml"

echo "→ building ${REF}"
docker build --build-arg VERSION="${TAG}" -t "${REF}" "${ROOT}"

echo "→ pushing ${REF}"
docker push "${REF}"

if command -v kind >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -qx strata; then
  echo "→ loading ${REF} into kind cluster 'strata'"
  kind load docker-image --name strata "${REF}"
fi

python3 - "${INTENT}" "${TAG}" "${REF}" <<'PY'
import re
import sys

path, tag, ref = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, encoding="utf-8") as fh:
    text = fh.read()
text = re.sub(r'version:\s*"[^"]*"', f'version: "{tag}"', text, count=1)
text = re.sub(r'sourceImage:\s*\S+', f'sourceImage: {ref}', text, count=1)
with open(path, "w", encoding="utf-8") as fh:
    fh.write(text)
PY

echo "→ updated ${INTENT}"
echo "  commit and push it; the monorepo watcher will redeploy the partition"
