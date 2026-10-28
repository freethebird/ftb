#!/usr/bin/env python3
import json, time, subprocess, urllib.request, os, tempfile
OUT = "/var/lib/ftb-status/status.json"
SLACK = 50
def rpc(port, method, params=None):
    data = json.dumps({"jsonrpc":"2.0","id":1,"method":method,"params":params or []}).encode()
    req = urllib.request.Request("http://127.0.0.1:%d" % port, data=data, headers={"Content-Type":"application/json"})
    with urllib.request.urlopen(req, timeout=8) as r:
        return json.load(r).get("result")
def node_status(port):
    try:
        h = int(rpc(port, "getblockcount"))
    except Exception:
        return {"status":"down"}
    tip = h
    try:
        peers = rpc(port, "getpeers") or {}
        hs = [int(p.get("lastknownheight") or 0) for p in peers.get("connected", [])]
        if hs: tip = max(tip, max(hs))
    except Exception:
        pass
    if h >= tip - SLACK:
        return {"status":"synced","height":h,"network":tip,"pct":100.0}
    return {"status":"syncing","height":h,"network":tip,"pct":round(h/tip*100,1) if tip else 0.0}
def watchdog_active():
    try:
        r = subprocess.run(["systemctl","is-active","ftb-node-mainnet-watchdog.timer"], capture_output=True, text=True, timeout=5)
        return r.stdout.strip() == "active"
    except Exception:
        return False
doc = {"updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
       "nodes": {"testnet": node_status(20332), "mainnet": node_status(10332)},
       "watchdog": {"active": watchdog_active()}}
os.makedirs(os.path.dirname(OUT), exist_ok=True)
fd, tmp = tempfile.mkstemp(dir=os.path.dirname(OUT))
with os.fdopen(fd, "w") as f: json.dump(doc, f, separators=(",",":"))
os.chmod(tmp, 0o644)
os.replace(tmp, OUT)
print(json.dumps(doc, indent=2))
