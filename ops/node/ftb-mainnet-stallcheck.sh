#!/usr/bin/env bash
RPC="http://127.0.0.1:10332"
STATE="/run/ftb-node-mainnet.stallcheck"
SVC="ftb-node-mainnet"
THRESH=2   # consecutive 5-min checks w/o progress before restart (~10 min)
H=$(curl -s -m 8 -X POST "$RPC" -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"getblockcount","params":[]}' | grep -oE '"result":[0-9]+' | cut -d: -f2)
if [ -z "$H" ]; then echo "stallcheck: RPC no height (node down/restarting) - no action"; exit 0; fi
read -r LAST CNT < "$STATE" 2>/dev/null; CNT=${CNT:-0}
if [ "$H" != "$LAST" ]; then echo "$H 0" > "$STATE"; echo "stallcheck: height=$H advancing (ok)"; exit 0; fi
CNT=$((CNT+1)); echo "$H $CNT" > "$STATE"; echo "stallcheck: height=$H frozen $((CNT*5))min (count=$CNT/$THRESH)"
if [ "$CNT" -ge "$THRESH" ]; then echo "stallcheck: STALL at height=$H - restarting $SVC"; systemctl restart "$SVC"; echo "$H 0" > "$STATE"; fi
exit 0
