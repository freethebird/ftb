#!/usr/bin/env bash
# Push the FTB static front-end to the IPFS node, pin it, publish IPNS.
# Run FROM your Mac:  ./ftb-push-ipfs.sh <ssh-target>   e.g. ./ftb-push-ipfs.sh root@203.0.113.10
set -euo pipefail

HOST="${1:?usage: ftb-push-ipfs.sh <user>@<vm-ip>}"
WEB_DIR="$(cd "$(dirname "$0")/../../web" && pwd)"
STAGE="$(mktemp -d)/ftb-web"
trap 'rm -rf "$(dirname "$STAGE")"' EXIT

echo "==> Staging front-end from: $WEB_DIR"
# -L resolves symlinks (web/proofs.json -> ../../proofs.json) so IPFS gets real files.
rsync -aL --exclude '.DS_Store' "$WEB_DIR/" "$STAGE/"

echo "==> Copying to $HOST"
ssh "$HOST" 'rm -rf /tmp/ftb-web && mkdir -p /tmp/ftb-web'
scp -qr "$STAGE/." "$HOST:/tmp/ftb-web/"
ssh "$HOST" 'chown -R ipfs:ipfs /tmp/ftb-web'

echo "==> Adding to IPFS and pinning"
ROOT_CID="$(ssh "$HOST" "sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs add -r -Q --pin /tmp/ftb-web | tr -d '[:space:]'")"
echo "    Root CID: $ROOT_CID"

echo "==> Ensuring IPNS key 'ftb' exists"
ssh "$HOST" 'sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs key list -l | awk "\$2==\"ftb\"" | grep -q . || sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs key gen ftb --type=ed25519'
IPNS="$(ssh "$HOST" 'sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs key list -l' | awk '$2=="ftb" {print $1}' | tr -d '[:space:]')"

echo "==> Publishing IPNS name (this can take a minute or two)"
ssh "$HOST" "sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs name publish --key=ftb --lifetime=720h --ttl=15m /ipfs/$ROOT_CID"
ssh "$HOST" 'rm -rf /tmp/ftb-web'

echo "==> Appending to deploy log"
LOG="$(cd "$(dirname "$0")" && pwd)/DEPLOYS.md"
REPO_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
COMMIT="$(git -C "$REPO_DIR" log --format='%h' -1 2>/dev/null || echo '?')"
STAMP="$(date '+%Y-%m-%d %H:%M')"
printf '| auto | %s | `%s` | `%s` | Deployed via ftb-push-ipfs.sh |\n' "$STAMP" "$ROOT_CID" "$COMMIT" >> "$LOG"
echo "    logged: $STAMP  $ROOT_CID  $COMMIT"

echo
echo "=============================================================="
echo " FTB front-end is live at https://freethebird.co"
echo "   ipfs://$ROOT_CID"
echo "   ipns://$IPNS  (stable name, just repointed)"
echo " Deploy logged to ftb/ops/ipfs/DEPLOYS.md"
echo "=============================================================="
