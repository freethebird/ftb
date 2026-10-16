#!/usr/bin/env python3
"""
Pre-exploit FLM + FLOCKS holder snapshot via authenticated state (findstates).

Why this exists alongside snapshot_tool.py
------------------------------------------
snapshot_tool.py asks an archive node "what was address X's balance at block N?"
once per holder (invokefunctionhistoric) and enumerates candidates from indexers.
That works, but the indexers cap and rate-limit, and completeness rests on the
candidate set being big enough.

This script instead enumerates the token contracts' BALANCE STORAGE directly at
the snapshot block's state root (getstateroot + findstates). Every non-zero
balance is a storage entry, so the holder list is complete BY CONSTRUCTION — no
indexer, no candidate guessing — and the built-in proof is exact: the sum of all
decoded balance entries must equal the pre-exploit totalSupply to the last unit.

VERIFIED ON-CHAIN 2026-10-09 (COZ archive node mainnet1.neo.coz.io)
  Exploit mint tx : 0x5e310782aac17676a80572e073ba0d70a7d533caff60b38a4e23740ddec22030
  Exploit block   : 12,782,705  -> snapshot at 12,782,704 (one before)
  State root @ snapshot: 0x86a3042d22bfc7d0d847a476cd145460c50d436881b53ea4037de6ac4a69baf7
  FLM    (id 26) : balance keys  01 01 <20-byte BE scripthash>,  totalSupply key 01 00 "totalSupply"
  FLOCKS (id 449): balance keys  09    <20-byte BE scripthash>,  totalSupply key 08
  (storage keys carry the BIG-ENDIAN / explorer-display form of the scripthash — verified)
  Anchors: FLM raw 57,470,878,533,054,587 (574,708,785.33) / FLOCKS raw 12,234,644,406,057,840 (122,346,444.06)

Outputs (workspace root):
  snapshot_full_state.csv  every non-zero balance entry at the snapshot (evidence, incl. contracts)
  snapshot.csv             the fair-supporter list: contracts and excluded wallets removed
Both sum-checked; the FULL file must hit the anchors EXACTLY.

Usage:
  python3 snapshot_state.py               # full run (enumeration + contract detection)
  python3 snapshot_state.py --workers 8   # concurrency for the contract-detection pass
"""
import argparse, base64, csv, json, os, sys, time
from concurrent.futures import ThreadPoolExecutor, as_completed
import requests

# ---------------- CONFIG ----------------
RPC        = "https://mainnet1.neo.coz.io:443"   # archive node WITH StateService (findstates)
SNAP_BLOCK = 12_782_704                          # block just before the exploit mint
EXPECTED_ROOT = "0x86a3042d22bfc7d0d847a476cd145460c50d436881b53ea4037de6ac4a69baf7"

FLM    = "0xf0151f528127558851b39c2cd8aa47da7418ab28"   # 8 decimals
FLOCKS = "0xdad88d8c8d4643548aa0f63c83796e81c3c6b04c"   # 8 decimals
FLM_BALANCE_PREFIX    = b"\x01\x01"   # 01 01 <20-byte LE scripthash> -> balance
FLOCKS_BALANCE_PREFIX = b"\x09"       # 09    <20-byte LE scripthash> -> balance
DECIMALS = 8

# Excluded from the community list: exploit wallets and the null address.
EXCLUDE_ADDRESSES = {
    "NWuHQdxabXPdC6vVwJhxjYELDQPqc1d4TG",   # attacker: holds 35.49T minted FLM
    "NcNCXJf2H8GSHp8MYSgJRCjRvPjznasZMy",   # attacker: holds 2.18T minted FLM (Council-blocked)
    "NKuyBkoGdZZSLyPbJEetheRhMjeznFZszf",   # zero/null address (mint "from"); never a real holder
    "NbRED8fXc3rRNxas4HxL1qFnKkExE49T3B",   # attacker consolidation wallet
}

SANITY_FLM_SUPPLY    = 57_470_878_533_054_587    # raw; EXACT match required over all entries
SANITY_FLOCKS_SUPPLY = 12_234_644_406_057_840    # raw; EXACT match required over all entries

STATE_DIR = "snap_state"
# ----------------------------------------

S = requests.Session()
S.headers.update({"content-type": "application/json"})

_B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

def _b58decode(s):
    num = 0
    for ch in s:
        num = num * 58 + _B58.index(ch)
    full = num.to_bytes((num.bit_length() + 7) // 8, "big")
    pad = len(s) - len(s.lstrip("1"))
    return b"\x00" * pad + full

def _b58encode(b):
    num = int.from_bytes(b, "big")
    out = ""
    while num:
        num, r = divmod(num, 58)
        out = _B58[r] + out
    pad = len(b) - len(b.lstrip(b"\x00"))
    return "1" * pad + out

def _sha256(b):
    import hashlib
    return hashlib.sha256(b).digest()

# NOTE: NEO stores UInt160 in contract storage keys as its BIG-ENDIAN (explorer display)
# bytes. Verified 2026-10-09: balance entry for 85,103,198.42 FLM sits under the
# big-endian form of NTRGiigCG7M3RsyBJRc1WoZDV2cfvZDwp8, not the little-endian one.
def scripthash_be_to_address(be_hex):
    """20-byte big-endian scripthash (hex str, as in storage keys) -> N-address."""
    body = b"\x35" + bytes.fromhex(be_hex)                # version 0x35 + big-endian hash
    chk = _sha256(_sha256(body))[:4]
    return _b58encode(body + chk)

def addr_to_scripthash_be(addr):
    raw = _b58decode(addr)
    assert len(raw) == 25, "bad address: " + addr
    return raw[1:21].hex()

def be_to_le(be_hex):
    return bytes.fromhex(be_hex)[::-1].hex()

EXCLUDE_BE = {addr_to_scripthash_be(a) for a in EXCLUDE_ADDRESSES}

def rpc(method, params, tries=6):
    for t in range(tries):
        try:
            r = S.post(RPC, json={"jsonrpc": "2.0", "id": 1, "method": method, "params": params},
                       timeout=60)
            if r.status_code == 200 and r.text:
                return r.json()
        except Exception:
            pass
        time.sleep(0.6 * (t + 1))
    raise RuntimeError(f"rpc {method} failed after {tries} tries")

def get_state_root():
    res = rpc("getstateroot", [SNAP_BLOCK])["result"]
    root = res["roothash"]
    if root != EXPECTED_ROOT:
        print(f"WARNING: state root {root} != expected {EXPECTED_ROOT}")
    return root

def walk_balances(root, contract_hash, prefix, label):
    """Paginate findstates over the balance prefix; returns {be_scripthash_hex: raw_int}."""
    cache = os.path.join(STATE_DIR, f"{label}_entries.json")
    if os.path.exists(cache):
        d = {k: int(v) for k, v in json.load(open(cache)).items()}
        print(f"  {label}: loaded {len(d)} cached entries")
        return d
    out, start, pages = {}, None, 0
    b64prefix = base64.b64encode(prefix).decode()
    while True:
        params = [root, contract_hash, b64prefix] + ([start] if start else [])
        j = rpc("findstates", params)
        res = j.get("result")
        if not res:
            raise RuntimeError(f"findstates failed for {label}: {j.get('error')}")
        rows = res.get("results", [])
        for r in rows:
            k = base64.b64decode(r["key"])
            v = base64.b64decode(r["value"])
            if len(k) != len(prefix) + 20:
                print(f"  {label}: skipping odd key {k.hex()}")
                continue
            bal = int.from_bytes(v, "little", signed=True)
            if bal > 0:
                out[k[len(prefix):].hex()] = bal
        pages += 1
        if pages % 10 == 0:
            print(f"  {label}: {pages} pages, {len(out)} entries...")
        if not res.get("truncated"):
            break
        start = rows[-1]["key"]
    os.makedirs(STATE_DIR, exist_ok=True)
    json.dump(out, open(cache, "w"))
    print(f"  {label}: {len(out)} non-zero balance entries ({pages} pages)")
    return out

def is_contract(le_hex):
    """A holder is a contract if getcontractstate resolves in either byte order."""
    h = "0x" + le_hex
    be = "0x" + bytes.fromhex(le_hex)[::-1].hex()
    for form in (h, be):
        try:
            j = rpc("getcontractstate", [form], tries=3)
            if j.get("result"):
                return True
        except Exception:
            pass
    return False

def detect_contracts(all_hashes, workers):
    cache_file = os.path.join(STATE_DIR, "contract_flags.json")
    flags = json.load(open(cache_file)) if os.path.exists(cache_file) else {}
    todo = [h for h in all_hashes if h not in flags]
    print(f"Contract detection: {len(flags)} cached, {len(todo)} to check")
    done = 0
    with ThreadPoolExecutor(max_workers=workers) as ex:
        futs = {ex.submit(is_contract, h): h for h in todo}
        for fut in as_completed(futs):
            h = futs[fut]
            try:
                flags[h] = bool(fut.result())
            except Exception:
                flags[h] = False
            done += 1
            if done % 200 == 0:
                json.dump(flags, open(cache_file, "w"))
                print(f"  {done}/{len(todo)}")
    json.dump(flags, open(cache_file, "w"))
    return flags

def fmt(raw):
    return raw / 10 ** DECIMALS

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--workers", type=int, default=6)
    args = ap.parse_args()

    print(f"State root at snapshot block {SNAP_BLOCK}...")
    root = get_state_root()
    print(f"  root: {root}")

    print("Enumerating balance storage at the snapshot state root...")
    flm = walk_balances(root, FLM, FLM_BALANCE_PREFIX, "flm")
    flk = walk_balances(root, FLOCKS, FLOCKS_BALANCE_PREFIX, "flocks")

    sum_flm, sum_flk = sum(flm.values()), sum(flk.values())
    print("\n==== EXACT SUPPLY CHECK (all entries, before any exclusion) ====")
    print(f"FLM    : {sum_flm:,} vs anchor {SANITY_FLM_SUPPLY:,}  "
          f"diff {sum_flm - SANITY_FLM_SUPPLY:+,}")
    print(f"FLOCKS : {sum_flk:,} vs anchor {SANITY_FLOCKS_SUPPLY:,}  "
          f"diff {sum_flk - SANITY_FLOCKS_SUPPLY:+,}")
    exact = (sum_flm == SANITY_FLM_SUPPLY and sum_flk == SANITY_FLOCKS_SUPPLY)

    all_hashes = sorted(set(flm) | set(flk))
    flags = detect_contracts(all_hashes, args.workers)

    full_rows, community_rows = [], []
    drop = {"contract": 0, "excluded": 0}
    for h in all_hashes:                                   # h = big-endian scripthash (storage form)
        f, k = flm.get(h, 0), flk.get(h, 0)
        addr = scripthash_be_to_address(h)
        contract = flags.get(h, False)
        excluded = h in EXCLUDE_BE
        full_rows.append((addr, be_to_le(h), f, k, contract, excluded))
        if contract:
            drop["contract"] += 1
            continue
        if excluded:
            drop["excluded"] += 1
            continue
        community_rows.append((addr, f, k))

    full_rows.sort(key=lambda r: -(r[2] + r[3]))
    community_rows.sort(key=lambda r: -(r[1] + r[2]))

    with open("snapshot_full_state.csv", "w", newline="") as fh:
        w = csv.writer(fh)
        w.writerow(["address", "scripthash_le", "flm_raw", "flocks_raw", "flm", "flocks",
                    "is_contract", "is_excluded_wallet"])
        for a, h, f, k, c, x in full_rows:
            w.writerow([a, h, f, k, fmt(f), fmt(k), int(c), int(x)])

    with open("snapshot.csv", "w", newline="") as fh:
        w = csv.writer(fh)
        w.writerow(["address_or_scripthash", "flm_raw", "flocks_raw", "flm", "flocks"])
        for a, f, k in community_rows:
            w.writerow([a, f, k, fmt(f), fmt(k)])

    com_flm = sum(r[1] for r in community_rows)
    com_flk = sum(r[2] for r in community_rows)
    print("\n==== SNAPSHOT COMPLETE (state-root method) ====")
    print(f"entries at snapshot : {len(full_rows)} (incl. contracts)")
    print(f"dropped             : {drop['contract']} contracts, {drop['excluded']} excluded wallets")
    print(f"community holders   : {len(community_rows)}  -> snapshot.csv")
    print(f"community FLM       : {fmt(com_flm):,.2f} of {fmt(SANITY_FLM_SUPPLY):,.2f} "
          f"({100 * com_flm / SANITY_FLM_SUPPLY:.2f}%)")
    print(f"community FLOCKS    : {fmt(com_flk):,.2f} of {fmt(SANITY_FLOCKS_SUPPLY):,.2f} "
          f"({100 * com_flk / SANITY_FLOCKS_SUPPLY:.2f}%)")
    if exact:
        print("VERDICT: storage sums equal the pre-exploit supplies EXACTLY. "
              "The holder list is complete by construction.")
    else:
        print("VERDICT: sums differ from the anchors — investigate before publishing "
              "(pagination gap, wrong prefix, or node state issue).")

if __name__ == "__main__":
    main()
