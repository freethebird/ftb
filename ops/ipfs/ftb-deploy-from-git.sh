#!/usr/bin/env bash
#
# ftb-deploy-from-git.sh — deploy the FTB front end to IPFS/IPNS FROM the canonical git.
#
# Runs ON the ftb-node (1984), NOT from a laptop. The published git is the single source
# of truth; this box is the deployer. Any collaborator (the Mac included) just `git push`es
# to the repo — nobody rsyncs a front end from their own machine. This supersedes the
# Mac-push model in ops/ipfs/ftb-push-ipfs.sh and matches the governance rule "the front
# end follows the vote": for MainNet, deploy a governance-tagged ref, never an ad-hoc push.
#
#   ./ftb-deploy-from-git.sh [git-ref]      # default: origin/main
#   ./ftb-deploy-from-git.sh gov-7          # deploy the ref a vote ratified
#
# One-time setup on the node (see the design doc):
#   - read-only deploy key at /etc/ftb/deploy_key, its .pub added to the repo's Deploy Keys
#   - this script at /usr/local/bin/ftb-deploy-from-git.sh (run as root)
#
set -euo pipefail

REPO_DIR="${FTB_REPO_DIR:-/srv/ftb-src}"
REMOTE="${FTB_REMOTE:-git@github.com:freethebird/ftb.git}"
REF="${1:-origin/main}"
IPFS_PATH="${IPFS_PATH:-/var/lib/ipfs}"
IPFS_KEY="${IPFS_KEY:-ftb}"
DEPLOY_LOG="${FTB_DEPLOY_LOG:-/var/log/ftb-deploys.md}"   # node-local; repo tree is read-only here
export GIT_SSH_COMMAND="ssh -i /etc/ftb/deploy_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"

log(){ echo ">> $*"; }
asipfs(){ sudo -u ipfs env IPFS_PATH="$IPFS_PATH" ipfs --api=/ip4/127.0.0.1/tcp/5001 "$@"; }

# 1. Sync the canonical git via the read-only deploy key
if [ ! -d "$REPO_DIR/.git" ]; then
  log "cloning $REMOTE -> $REPO_DIR"
  git clone "$REMOTE" "$REPO_DIR"
fi
cd "$REPO_DIR"
git fetch --prune origin
git reset --hard "$REF"
COMMIT="$(git rev-parse --short HEAD)"
log "deploying commit $COMMIT ($REF)"

# 2. Stage web/ (same exclusions + symlink resolution as the proven push script)
STAGE_PARENT="$(mktemp -d)"; STAGE="$STAGE_PARENT/ftb-web"; mkdir -p "$STAGE"
trap 'rm -rf "$STAGE_PARENT"' EXIT
rsync -aL --exclude '.DS_Store' --exclude 'beat.json' --exclude 'archive/' --exclude '__pycache__/' \
  "$REPO_DIR/web/" "$STAGE/"

# config.js is git-ignored (filled config, never committed); materialize it from the
# tracked profile so the git-deploy ships a working config. TestNet follows main;
# a gov-tagged (MainNet) ref uses the mainnet profile.
CFG_PROFILE=testnet
case "$REF" in gov-*|release*|*mainnet*) CFG_PROFILE=mainnet ;; esac
cp "$STAGE/config.$CFG_PROFILE.js" "$STAGE/config.js"
log "config.js <- config.$CFG_PROFILE.js"

# version stamp (the front-end mirror counter compares this across origins)
IPNS="$(asipfs key list -l | awk -v k="$IPFS_KEY" '$2==k{print $1}')"
[ -n "$IPNS" ] || { echo "FATAL: IPNS key '$IPFS_KEY' not found in the node keystore" >&2; exit 1; }
printf '{"project":"ftb","commit":"%s","published":"%s","ipns":"%s"}\n' \
  "$COMMIT" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$IPNS" > "$STAGE/beat.json"
chown -R ipfs:ipfs "$STAGE_PARENT"

# 2b. GATE - publish-time floor (ftb-deploy-gate.sh). With `set -e`, a failing
#     gate aborts HERE, before anything is published, so the live site is untouched.
/usr/local/bin/ftb-deploy-gate.sh "$STAGE" "$CFG_PROFILE"
log "gate: PASS - publishing"

# 3. Add + pin + republish IPNS to the 'ftb' key (default CID version, matching the live site)
ROOT_CID="$(asipfs add -r -Q --pin "$STAGE" | tr -d '[:space:]')"
log "root CID: $ROOT_CID"
asipfs name publish --key="$IPFS_KEY" --lifetime=720h --ttl=15m "/ipfs/$ROOT_CID"

# 4. Clear the gateway's in-memory IPNS cache, then verify
systemctl restart ipfs && sleep 5 && systemctl is-active ipfs >/dev/null
sleep 3
curl -sS -o /dev/null -w "   https://freethebird.co -> HTTP %{http_code}\n" https://freethebird.co || true

# 5. Log (node-local — the repo working tree is reset on every deploy)
mkdir -p "$(dirname "$DEPLOY_LOG")"
printf '| git | %s | `%s` | `%s` | ftb-deploy-from-git (%s) |\n' \
  "$(date '+%Y-%m-%d %H:%M')" "$ROOT_CID" "$COMMIT" "$REF" >> "$DEPLOY_LOG"

log "done — $COMMIT live at https://freethebird.co  (IPNS $IPNS)"
