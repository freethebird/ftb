#!/usr/bin/env bash
# ftb-autodeploy.sh — watch origin/live, run the GATED deploy when it advances.
# Idempotent; a commit that fails the gate is tried once, flagged, and not retried
# until live moves again. The live site is never touched on a gate failure.
set -uo pipefail
REPO_DIR="${FTB_REPO_DIR:-/srv/ftb-src}"
BRANCH="${FTB_LIVE_BRANCH:-live}"
STATE_DIR=/var/lib/ftb-deploy
STATE="$STATE_DIR/live-sha"; FAILED="$STATE_DIR/failed-sha"; MARKER="$STATE_DIR/gate-failed"
export GIT_SSH_COMMAND="ssh -i /etc/ftb/deploy_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"
mkdir -p "$STATE_DIR"
cd "$REPO_DIR" 2>/dev/null || { echo "autodeploy: no repo at $REPO_DIR" >&2; exit 1; }
git fetch --prune origin >/dev/null 2>&1 || { echo "autodeploy: fetch failed (transient)" >&2; exit 0; }
REMOTE="$(git rev-parse "origin/$BRANCH" 2>/dev/null || true)"
[ -n "$REMOTE" ] || { echo "autodeploy: origin/$BRANCH not found" >&2; exit 0; }
HAVE="$(cat "$STATE" 2>/dev/null || true)"; LASTFAIL="$(cat "$FAILED" 2>/dev/null || true)"
if [ "$REMOTE" = "$HAVE" ]; then echo "autodeploy: origin/$BRANCH unchanged (${REMOTE:0:7}) — nothing to do"; exit 0; fi
if [ "$REMOTE" = "$LASTFAIL" ]; then echo "autodeploy: ${REMOTE:0:7} already failed the gate — waiting for a fix"; exit 0; fi
echo "autodeploy: origin/$BRANCH -> ${REMOTE:0:7} (was ${HAVE:0:7})"
if /usr/local/bin/ftb-deploy-from-git.sh "origin/$BRANCH"; then
  echo "$REMOTE" > "$STATE"; rm -f "$FAILED" "$MARKER"; echo "autodeploy: ${REMOTE:0:7} is live"
else
  rc=$?; echo "$REMOTE" > "$FAILED"
  printf '%s  origin/%s @ %s FAILED the deploy gate (rc=%s) — LIVE SITE UNCHANGED\n' "$(date -u +%FT%TZ)" "$BRANCH" "${REMOTE:0:7}" "$rc" | tee "$MARKER" >&2
  echo "autodeploy: ${REMOTE:0:7} blocked by the gate — flagged; no retry until $BRANCH moves" >&2
  exit 0
fi
