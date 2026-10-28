#!/usr/bin/env python3
# TestNet neo-go stall detector. Restarts ftb-node ONLY if its height is frozen AND it is
# behind the network (peers ahead) for ~10 min. Peer-aware, so a quiet-network lull at the
# tip never needlessly restarts the node that serves the live dApp.
import json, subprocess, urllib.request, sys
RPC="http://127.0.0.1:20332"; SVC="ftb-node"; STATE="/run/ftb-node-testnet.stallcheck"
THRESH=2; SLACK=50
def rpc(method, params=None):
    data=json.dumps({"jsonrpc":"2.0","id":1,"method":method,"params":params or []}).encode()
    req=urllib.request.Request(RPC, data=data, headers={"Content-Type":"application/json"})
    with urllib.request.urlopen(req, timeout=8) as r: return json.load(r).get("result")
try:
    h=int(rpc("getblockcount"))
except Exception as e:
    print("stallcheck: RPC no height (node down/restarting) - no action:", e); sys.exit(0)
tip=h
try:
    peers=rpc("getpeers") or {}
    hs=[int(p.get("lastknownheight") or 0) for p in peers.get("connected",[])]
    if hs: tip=max(tip, max(hs))
except Exception: pass
last=None; cnt=0
try:
    with open(STATE) as f:
        parts=f.read().split(); last=int(parts[0]); cnt=int(parts[1])
except Exception: pass
if h != last:
    open(STATE,"w").write("%d 0" % h); print("stallcheck: height=%d advancing (ok)" % h); sys.exit(0)
if (tip - h) <= SLACK:
    open(STATE,"w").write("%d 0" % h); print("stallcheck: height=%d unchanged but at tip (peers=%d) - ok" % (h,tip)); sys.exit(0)
cnt+=1; open(STATE,"w").write("%d %d" % (h,cnt))
print("stallcheck: height=%d FROZEN and behind peers=%d, %dmin (count=%d/%d)" % (h,tip,cnt*5,cnt,THRESH))
if cnt>=THRESH:
    print("stallcheck: STALL at height=%d (peers at %d) - restarting %s" % (h,tip,SVC))
    subprocess.run(["systemctl","restart",SVC], timeout=30)
    open(STATE,"w").write("%d 0" % h)
sys.exit(0)
