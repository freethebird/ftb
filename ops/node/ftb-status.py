#!/usr/bin/env python3
import json, time, subprocess, urllib.request, os, tempfile
OUT = "/var/lib/ftb-status/status.json"
SLACK = 50
IPFS_API = "http://127.0.0.1:5001/api/v0"
IPNS = "k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq"
FINDPROVS_SECS = 30
MIRRORS_TTL = 300                                  # recompute DHT provider count at most every 5 min
MIRRORS_CACHE = "/var/lib/ftb-status/mirrors.cache"
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
def _findprovs_count():
    # Resolve the live front-end release CID, then count distinct DHT providers for it.
    r = subprocess.run(["curl","-s","--max-time","20","-X","POST",
        IPFS_API + "/name/resolve?arg=/ipns/" + IPNS + "&nocache=true&recursive=true"],
        capture_output=True, text=True, timeout=25)
    cid = json.loads(r.stdout or "{}").get("Path","").replace("/ipfs/","")
    if not cid:
        return None
    r = subprocess.run(["curl","-s","--max-time",str(FINDPROVS_SECS),"-X","POST",
        IPFS_API + "/routing/findprovs?arg=" + cid + "&num-providers=200"],
        capture_output=True, text=True, timeout=FINDPROVS_SECS + 5)
    peers = set()
    for line in r.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            ev = json.loads(line)
        except Exception:
            continue
        if ev.get("Type") == 4:
            for resp in (ev.get("Responses") or []):
                pid = resp.get("ID")
                if pid:
                    peers.add(pid)
    return len(peers)
def ipfs_mirrors():
    # Distinct peers currently PROVIDING the live front-end release on the IPFS DHT.
    # Every node that pins and announces the release self-registers, so this is the real
    # mirror count. Cached to MIRRORS_CACHE and refreshed at most every MIRRORS_TTL seconds
    # so the 1-min timer does not walk the DHT on every tick; on a transient failure the
    # last good value is reused rather than dropping to null.
    now = time.time()
    cached = None
    try:
        with open(MIRRORS_CACHE) as f:
            c = json.load(f)
        cached = c.get("n")
        if cached is not None and (now - c.get("ts", 0)) < MIRRORS_TTL:
            return cached
    except Exception:
        pass
    try:
        n = _findprovs_count()
    except Exception:
        n = None
    if n is None:
        return cached
    try:
        fd, tmp = tempfile.mkstemp(dir=os.path.dirname(MIRRORS_CACHE))
        with os.fdopen(fd, "w") as f: json.dump({"n": n, "ts": now}, f)
        os.chmod(tmp, 0o644)
        os.replace(tmp, MIRRORS_CACHE)
    except Exception:
        pass
    return n

doc = {"updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
       "nodes": {"testnet": node_status(20332), "mainnet": node_status(10332)},
       "watchdog": {"active": watchdog_active()},
       "mirrors": ipfs_mirrors()}
os.makedirs(os.path.dirname(OUT), exist_ok=True)
fd, tmp = tempfile.mkstemp(dir=os.path.dirname(OUT))
with os.fdopen(fd, "w") as f: json.dump(doc, f, separators=(",",":"))
os.chmod(tmp, 0o644)
os.replace(tmp, OUT)
print(json.dumps(doc, indent=2))
