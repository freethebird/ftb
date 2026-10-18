#!/usr/bin/env bash
# Push the FTB static front-end to the IPFS node, pin it, publish IPNS.
# Run FROM your Mac:  ./ftb-push-ipfs.sh <ssh-target>   e.g. ./ftb-push-ipfs.sh root@203.0.113.10
set -euo pipefail

HOST="${1:?usage: ftb-push-ipfs.sh <user>@<vm-ip>}"
WEB_DIR="$(cd "$(dirname "$0")/../../web" && pwd)"
STAGE="$(mktemp -d)/ftb-web"
trap 'rm -rf "$(dirname "$STAGE")"' EXIT

REPO_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
COMMIT="$(git -C "$REPO_DIR" rev-parse --short HEAD 2>/dev/null || echo '?')"
STAMP="$(date '+%Y-%m-%d %H:%M')"
STAMP_ISO="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

echo "==> Ensuring IPNS key 'ftb' exists"
ssh "$HOST" 'sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs key list -l | awk "\$2==\"ftb\"" | grep -q . || sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs key gen ftb --type=ed25519'
IPNS="$(ssh "$HOST" 'sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs key list -l' | awk '$2=="ftb" {print $1}' | tr -d '[:space:]')"

echo "==> Staging front-end from: $WEB_DIR"
# -L resolves symlinks (web/proofs.json -> ../../proofs.json) so IPFS gets real files.
# A stale local beat.json never rides along — the stamp below is the only one.
# archive/ (frozen pre-split designs + their builders) is reference weight, not
# runtime: it stays in the repo but never gets pinned (WEB-04).
rsync -aL --exclude '.DS_Store' --exclude 'beat.json' --exclude 'archive/' \
  --exclude '__pycache__/' "$WEB_DIR/" "$STAGE/"

# Version stamp: the front end's mirror counter compares this file across origins —
# same commit on two nodes = both serve this exact build.
printf '{"project":"ftb","commit":"%s","published":"%s","ipns":"%s"}\n' \
  "$COMMIT" "$STAMP_ISO" "$IPNS" > "$STAGE/beat.json"

echo "==> Copying to $HOST"
ssh "$HOST" 'rm -rf /tmp/ftb-web && mkdir -p /tmp/ftb-web'
scp -qr "$STAGE/." "$HOST:/tmp/ftb-web/"
ssh "$HOST" 'chown -R ipfs:ipfs /tmp/ftb-web'

echo "==> Adding to IPFS and pinning"
ROOT_CID="$(ssh "$HOST" "sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs add -r -Q --pin /tmp/ftb-web | tr -d '[:space:]'")"
echo "    Root CID: $ROOT_CID"

echo "==> Publishing IPNS name (this can take a minute or two)"
ssh "$HOST" "sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs name publish --key=ftb --lifetime=720h --ttl=15m /ipfs/$ROOT_CID"
ssh "$HOST" 'rm -rf /tmp/ftb-web'

echo "==> Appending to deploy log"
LOG="$(cd "$(dirname "$0")" && pwd)/DEPLOYS.md"
printf '| auto | %s | `%s` | `%s` | Deployed via ftb-push-ipfs.sh |\n' "$STAMP" "$ROOT_CID" "$COMMIT" >> "$LOG"
echo "    logged: $STAMP  $ROOT_CID  $COMMIT"

echo
echo "=============================================================="
echo " FTB front-end is live at https://freethebird.co"
echo "   ipfs://$ROOT_CID"
echo "   ipns://$IPNS  (stable name, just repointed)"
echo " Deploy logged to ftb/ops/ipfs/DEPLOYS.md"
echo "=============================================================="
