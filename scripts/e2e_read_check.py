#!/usr/bin/env python3
"""End-to-end read-side verification of every FTB module against TestNet.

Hits the community RPC exactly the way index.html does (invokefunction,
read-only) and prints one line per module read: value or FAULT reason.
"""
import json, sys, urllib.request

RPC = sys.argv[1] if len(sys.argv) > 1 else 'https://freethebird.co/rpc'

C = {
    'ftb':       '0xdc82bb0c89085dfeb81a40cebce786cb1b73044c',
    'migrator':  '0x6b79b687956b0f49474643ba7212f7b3b55adba9',
    'registry':  '0x0d59f3bb8018f5668ac4448249f21927fe961614',
    'oracle':    '0x1c92a93d438b9468b45706cbc4b1ddcc345df875',
    'lend':      '0xae55c7b4950f9da4486baafdfd6ba2ae5ebdf8f4',
    'ballot':    '0x25483d3645d3cfed8016717aa7e120b5cababce1',
    'bondvault': '0x029785b1a2b395a295ebf96f7d68a3cee9afb967',
    'treasury':  '0xcac71ad3478491401ec72c35831521437d06aea6',
    'dividends': '0xb98cd5d04b20b84fcef0fc9d305dd5092c5a0df9',
    'router':    '0x0000000000000000000000000000000000000000',
}
NEO = '0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5'
GAS = '0xd2a4cff31913016155e38e474a2c06d08be276cf'


def rpc(method, params):
    body = json.dumps({'jsonrpc': '2.0', 'id': 1, 'method': method, 'params': params}).encode()
    req = urllib.request.Request(RPC, data=body, headers={
        'Content-Type': 'application/json', 'User-Agent': 'curl/8.0'})
    with urllib.request.urlopen(req, timeout=30) as r:
        out = json.loads(r.read())
    if 'error' in out:
        raise RuntimeError(out['error'].get('message', out['error']))
    return out['result']


def read(contract, method, args=()):
    """invokefunction; returns (state, stack) — state HALT/FAULT."""
    res = rpc('invokefunction', [contract, method, [
        {'type': t, 'value': v} for t, v in args]])
    return res['state'], res.get('stack', [])


def intval(stack):
    if not stack:
        return None
    v = stack[0].get('value')
    return int(v) if v not in (None, '') else 0


def ftb(units, dec=8):
    return f'{units / 10**dec:,.8f}'.rstrip('0').rstrip('.') if units is not None else '?'


rows = []

# --- shell / network ---
h = rpc('getblockcount', [])
rows.append(('shell', 'block height', f'{h:,}'))

# --- token / claim module ---
s, st = read(C['ftb'], 'totalSupply')
rows.append(('claim/token', 'ftb totalSupply', f'{ftb(intval(st))} FTB' if s == 'HALT' else f'FAULT: {st}'))
s, st = read(C['ftb'], 'maxSupply')
rows.append(('claim/token', 'ftb maxSupply', f'{ftb(intval(st))} FTB' if s == 'HALT' else f'FAULT: {st}'))
s, st = read(C['ftb'], 'migrationOpen')
rows.append(('claim/token', 'migrationOpen', (st[0].get('value') if st else '?') if s == 'HALT' else f'FAULT: {st}'))

# --- migrator ---
s, st = read(C['migrator'], 'quote', [('Integer', '100000000000')])  # 1,000.00000 FLM-equiv sample
rows.append(('claim', 'migrator.quote(1,000 FLM)', f'{ftb(intval(st))} FTB (expect ~189.832)' if s == 'HALT' else f'FAULT: {st}'))

# --- governance ---
for i, label in ((1, 'adopt'), (2, 'wait')):
    s, st = read(C['ballot'], 'tally', [('Integer', str(i))])
    rows.append(('governance', f'ballot tally[{label}]', f'{ftb(intval(st))} FLM' if s == 'HALT' else f'FAULT: {st}'))

# --- oracle ---
for name, h160 in (('NEO', NEO), ('GAS', GAS)):
    s, st = read(C['oracle'], 'price', [('Hash160', h160)])
    rows.append(('lend/oracle', f'oracle.price({name})', f'{intval(st):,} (raw)' if s == 'HALT' else f'FAULT: {st}'))

# --- lend ---
for m, label in (('borrowApyBps', 'borrow apy bps'), ('supplyApyBps', 'supply apy bps'),
                 ('totalBorrows', 'total borrows'), ('totalShares', 'total lp shares'),
                 ('utilizationBps', '% liquidity loaned (bps)'),
                 ('reserveAccrued', 'reserve accrued')):
    s, st = read(C['lend'], m)
    v = intval(st)
    if s == 'HALT':
        if m.endswith('Bps'):
            shown = f'{v / 100:.2f}%'
        elif m == 'totalShares':
            shown = f'{v:,} shares'
        else:
            shown = f'{ftb(v)} FTB'
    else:
        shown = f'FAULT: {st}'
    rows.append(('lend', label, shown))
for name, h160 in (('NEO collateral', NEO), ('GAS collateral', GAS), ('pool FTB cash', C['ftb'])):
    s, st = read(h160, 'balanceOf', [('Hash160', C['lend'])])
    dec = 0 if name.startswith('NEO') else 8
    rows.append(('lend/tvl', f'lend holds {name}', f'{ftb(intval(st), dec)}' if s == 'HALT' else f'FAULT: {st}'))

# --- dividends / earn ---
s, st = read(C['dividends'], 'monthIndex')
mi = intval(st)
rows.append(('earn', 'monthIndex', str(mi) if s == 'HALT' else f'FAULT: {st}'))
for i in {mi, mi - 1} if s == 'HALT' else set():
    if i < 0:
        continue
    s2, st2 = read(C['dividends'], 'monthTotal', [('Integer', str(i))])
    rows.append(('earn', f'monthTotal[{i}]', f'{ftb(intval(st2))} FTB' if s2 == 'HALT' else f'FAULT: {st2}'))
for m in ('totalReceived', 'totalStaked', 'accPerShare'):
    s, st = read(C['dividends'], m)
    v = intval(st)
    rows.append(('earn', m, (f'{ftb(v)} FTB' if m != 'accPerShare' else f'{v:,} (1e18-scaled)') if s == 'HALT' else f'FAULT: {st}'))

# --- treasury / fee split proof ---
s, st = read(C['ftb'], 'balanceOf', [('Hash160', C['treasury'])])
rows.append(('earn/treasury', 'treasury FTB held', f'{ftb(intval(st))} FTB' if s == 'HALT' else f'FAULT: {st}'))
s, st = read(C['ftb'], 'balanceOf', [('Hash160', C['dividends'])])
rows.append(('earn/treasury', 'dividends FTB held', f'{ftb(intval(st))} FTB' if s == 'HALT' else f'FAULT: {st}'))

# --- swap / market (standby expected) ---
if C['router'] == '0x' + '0' * 40:
    rows.append(('swap/market', 'router', 'PLACEHOLDER — standby by design (Decision 3): no AMM on TestNet, neo#3285'))

w = max(len(a) + len(b) for a, b, _ in rows)
for mod, item, val in rows:
    print(f'[{mod:<13}] {item:<28} {val}')
