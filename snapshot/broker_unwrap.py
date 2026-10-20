#!/usr/bin/env python3
"""Unwrap FlamingoBroker internal account balances at snapshot block 12,782,704.

Stage 1 (keys):   enumerate all 0x06-prefixed storage keys (user||token -> balance map)
Stage 2 (bal):    historic getAccountBalance for each candidate user, FLM + FLOCKS
Stage 3 (report): join with snapshot CSV, flagged list, claim tree; write CSV + summary
Resumable: caches to broker_keys.json / broker_balances.json
"""
import json, time, base64, hashlib, urllib.request, os, sys, csv

WS = "/Users/merlin/Desktop/FLM KIMI"
RPC = "https://mainnet1.neo.coz.io:443"
BROKER = "0xec268e9c642b7d09d10fe658bcb1cc63c0895d4d"
FLM_BE = "f0151f528127558851b39c2cd8aa47da7418ab28"
FLOCKS_BE = "dad88d8c8d4643548aa0f63c83796e81c3c6b04c"
FLM_LE = bytes.fromhex(FLM_BE)[::-1]
FLOCKS_LE = bytes.fromhex(FLOCKS_BE)[::-1]
SNAP_BLOCK = 12782704
HDR = {"Content-Type": "application/json", "User-Agent": "ftb-audit/1.0"}
_id = [0]

def rpc(method, params, retries=5):
    _id[0] += 1
    body = json.dumps({"jsonrpc": "2.0", "id": _id[0], "method": method, "params": params}).encode()
    last = None
    for a in range(retries):
        try:
            req = urllib.request.Request(RPC, data=body, headers=HDR)
            with urllib.request.urlopen(req, timeout=40) as f:
                d = json.load(f)
            if "error" in d:
                raise Exception(d["error"])
            return d["result"]
        except Exception as e:
            last = e
            time.sleep(1.2 * (a + 1))
    raise RuntimeError(f"rpc failed {method}: {last}")

B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
def b58encode(b):
    n = int.from_bytes(b, "big"); s = ""
    while n > 0:
        n, r = divmod(n, 58); s = B58[r] + s
    pad = 0
    for c in b:
        if c == 0: pad += 1
        else: break
    return "1" * pad + s

def sh_le_to_addr(le):
    payload = b"\x35" + le
    chk = hashlib.sha256(hashlib.sha256(payload).digest()).digest()[:4]
    return b58encode(payload + chk)

def historic_balance(user_be, token_be):
    r = rpc("invokefunctionhistoric", [SNAP_BLOCK, BROKER, "getAccountBalance", [
        {"type": "Hash160", "value": "0x" + user_be},
        {"type": "Hash160", "value": "0x" + token_be}]])
    st = r.get("stack", [])
    if r.get("state") != "HALT" or not st:
        return None
    v = st[0].get("value")
    return int(v) if v not in (None, "") else 0

def stage_keys():
    cache = os.path.join(WS, "broker_keys.json")
    if os.path.exists(cache):
        keys = json.load(open(cache))
        print(f"cached keys: {len(keys)}")
        return keys
    keys, seen = [], set()
    pages = 0
    while True:
        params = [BROKER, base64.b64encode(b"\x06").decode(), len(seen)]
        r = rpc("findstorage", params)
        results = r.get("results", [])
        new = 0
        for e in results:
            if e["key"] in seen:
                continue
            seen.add(e["key"]); keys.append(e["key"]); new += 1
        pages += 1
        if pages % 20 == 0:
            print(f"  page {pages}, keys {len(keys)}", flush=True)
        if not r.get("truncated") or not results:
            break
    json.dump(keys, open(cache, "w"))
    print(f"total 0x06 keys: {len(keys)} in {pages} pages")
    return keys

def candidate_users(keys):
    users = {}  # user_BE -> set of tokens seen in keys
    for kb64 in keys:
        k = base64.b64decode(kb64)
        if len(k) != 41 or k[0] != 0x06:
            continue
        user_le, token_le = k[1:21], k[21:41]
        if token_le in (FLM_LE, FLOCKS_LE):
            ube = user_le[::-1].hex()
            users.setdefault(ube, set()).add("flm" if token_le == FLM_LE else "flocks")
    return users

def stage_bal(users):
    cache = os.path.join(WS, "broker_balances.json")
    bals = json.load(open(cache)) if os.path.exists(cache) else {}
    todo = [u for u in users if u not in bals]
    print(f"users: {len(users)}, cached: {len(bals)}, todo: {todo.__len__()}")
    for i, u in enumerate(todo):
        flm = historic_balance(u, FLM_BE)
        flocks = historic_balance(u, FLOCKS_BE)
        if flm is None or flocks is None:
            print(f"  WARN non-HALT for {u}, will retry next run")
            continue
        bals[u] = {"flm": flm, "flocks": flocks}
        if (i + 1) % 50 == 0:
            json.dump(bals, open(cache, "w"))
            print(f"  {i+1}/{len(todo)} done", flush=True)
        time.sleep(0.12)
    json.dump(bals, open(cache, "w"))
    return bals

def stage_report(users, bals):
    snap = {}
    with open(os.path.join(WS, "snapshot_full_state.csv")) as f:
        for row in csv.DictReader(f):
            snap[row["address"]] = row
    flagged = {}
    with open(os.path.join(WS, "FLM_top20_flagged.csv")) as f:
        for row in csv.DictReader(f):
            flagged[row["address"]] = row
    rows, tot_flm, tot_flocks = [], 0, 0
    for u, b in sorted(bals.items(), key=lambda kv: -(kv[1]["flm"] + kv[1]["flocks"])):
        if b["flm"] == 0 and b["flocks"] == 0:
            continue
        addr = sh_le_to_addr(bytes.fromhex(u)[::-1])
        flm, flocks = b["flm"] / 1e8, b["flocks"] / 1e8
        tot_flm += flm; tot_flocks += flocks
        s = snap.get(addr, {})
        rows.append({
            "address": addr, "scripthash_be": u,
            "broker_flm": f"{flm:.8f}", "broker_flocks": f"{flocks:.8f}",
            "wallet_flm_snapshot": s.get("flm", ""), "wallet_flocks_snapshot": s.get("flocks", ""),
            "in_snapshot_csv": "yes" if s else "no",
            "flagged": flagged.get(addr, {}).get("recommend_exclude", ""),
            "flag_label": flagged.get(addr, {}).get("label", ""),
        })
    out = os.path.join(WS, "broker_unwrap_snapshot.csv")
    with open(out, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0].keys()))
        w.writeheader(); w.writerows(rows)
    print(f"wrote {out}: {len(rows)} users with nonzero balance")
    print(f"TOTAL broker FLM at snapshot:    {tot_flm:,.4f}")
    print(f"TOTAL broker FLOCKS at snapshot: {tot_flocks:,.4f}")
    zero = sum(1 for b in bals.values() if b["flm"] == 0 and b["flocks"] == 0)
    print(f"(users checked: {len(bals)}, zero at snapshot: {zero})")

if __name__ == "__main__":
    stage = sys.argv[1] if len(sys.argv) > 1 else "all"
    keys = stage_keys()
    users = candidate_users(keys)
    print(f"candidate users (FLM/FLOCKS keys): {len(users)}")
    if stage in ("all", "bal", "report"):
        bals = stage_bal(users)
        print(f"balances resolved: {len(bals)}/{len(users)}")
        if len(bals) == len(users):
            stage_report(users, bals)
        else:
            print("incomplete; re-run to resume")
