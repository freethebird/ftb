#!/usr/bin/env bash
# ftb-follow.sh — mirror the FTB front-end on any IPFS node.
# Resolves the FTB IPNS name, pins the current release, unpins the superseded one.
# Requires: a running Kubo daemon (ipfs daemon). Safe to run repeatedly, e.g. from cron:
#   */15 * * * *  IPFS_PATH=$HOME/.ipfs /path/to/ftb-follow.sh >> $HOME/.ftb-follow.log 2>&1
set -euo pipefail

IPNS="${1:-k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq}"
STATE_FILE="${FTB_FOLLOW_STATE:-$HOME/.ftb-follow-cid}"

NEW_CID="$(ipfs name resolve --nocache -r "/ipns/$IPNS" | sed 's|^/ipfs/||' | tr -d '[:space:]')"
OLD_CID="$(cat "$STATE_FILE" 2>/dev/null || true)"

if [ "$NEW_CID" = "$OLD_CID" ]; then
  echo "$(date '+%F %T') up to date: $NEW_CID"
  exit 0
fi

echo "$(date '+%F %T') new release: $NEW_CID (was: ${OLD_CID:-none})"
ipfs pin add --progress "/ipfs/$NEW_CID"

if [ -n "$OLD_CID" ] && [ "$OLD_CID" != "$NEW_CID" ]; then
  ipfs pin rm "$OLD_CID" >/dev/null 2>&1 || true
  echo "$(date '+%F %T') unpinned superseded: $OLD_CID"
fi

echo "$NEW_CID" > "$STATE_FILE"
echo "$(date '+%F %T') mirror current: $NEW_CID"
