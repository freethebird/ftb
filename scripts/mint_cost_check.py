#!/usr/bin/env python3
"""Measure the true execution cost (gasconsumed) of an FTB migration claim
— the ONLY mint path — by simulating claim() against TestNet with real
proofs from proofs.json. RPC simulation does not verify signatures, so a
HALT reports exactly what the chain would charge. Samples the smallest,
median, and largest UNCLAIMED entitlements.
"""
import base64, json, sys, urllib.request

RPC = sys.argv[1] if len(sys.argv) > 1 else 'https://testnet1.neo.coz.io:443'
MIGRATOR = '0x6b79b687956b0f49474643ba7212f7b3b55adba9'
B58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'


def addr_to_h160(addr):
    n = 0
    for ch in addr:
        n = n * 58 + B58.index(ch)
    raw = n.to_bytes(25, 'big')
    assert raw[0] == 0x35, 'not an N3 address'
    return '0x' + raw[1:21][::-1].hex()


def rpc(method, params):
    body = json.dumps({'jsonrpc': '2.0', 'id': 1, 'method': method, 'params': params}).encode()
    req = urllib.request.Request(RPC, data=body, headers={
        'Content-Type': 'application/json', 'User-Agent': 'curl/8.0'})
    with urllib.request.urlopen(req, timeout=60) as r:
        out = json.loads(r.read())
    if 'error' in out:
        raise RuntimeError(out['error'].get('message', out['error']))
    return out['result']


def mig(method, args, signers=None):
    params = [MIGRATOR, method, args] + ([signers] if signers else [])
    return rpc('invokefunction', params)


def sim_claim(h160, amount, proof_hex):
    args = [
        {'type': 'Hash160', 'value': h160},
        {'type': 'Integer', 'value': amount},
        # RPC wants ByteString params base64; proofs.json stores hex (NeoLine
        # does this conversion in the browser — we do it by hand here)
        {'type': 'Array', 'value': [
            {'type': 'ByteString', 'value': base64.b64encode(bytes.fromhex(h)).decode()}
            for h in proof_hex]},
    ]
    return mig('claim', args, [{'account': h160, 'scopes': 'CalledByEntry'}])


entries = json.load(open('proofs.json'))['entries']
es = sorted(entries, key=lambda e: int(e['amount']))

# collect unclaimed wallets, probing evenly across the size range
unclaimed = []
step = max(1, len(es) // 60)
for e in es[::step]:
    h = addr_to_h160(e['address'])
    r = mig('claimed', [{'type': 'Hash160', 'value': h}])
    if r['state'] == 'HALT' and r['stack'] and r['stack'][0].get('value') is False:
        unclaimed.append((e, h))

if not unclaimed:
    print('no unclaimed wallets found in the probe window')
    raise SystemExit(1)

samples = [unclaimed[0], unclaimed[len(unclaimed) // 2], unclaimed[-1]]
print(f'{len(unclaimed)} unclaimed in probe window; sampling min / median / max\n')
print(f'{"wallet":<36} {"entitlement FLM":>18} {"mints FTB":>14} {"state":<7} {"gasconsumed":>12} {"sys fee GAS":>12}')
fees = []
for e, h in samples:
    r = sim_claim(h, e['amount'], e['proof'])
    gc = int(r['gasconsumed'])
    mint = int(e['amount']) * 189832 // 1000000
    note = '' if r['state'] == 'HALT' else '  <- ' + (r.get('exception') or '')[:60]
    if r['state'] == 'HALT':
        fees.append(gc)
    print(f'{e["address"]:<36} {int(e["amount"])/1e8:>18,.8f} {mint/1e8:>14,.8f} {r["state"]:<7} {gc:>12,} {gc/1e8:>12.8f}{note}')

if fees:
    lo, hi = min(fees) / 1e8, max(fees) / 1e8
    mid = sorted(fees)[len(fees) // 2] / 1e8
    print(f'\nsystem fee per successful claim: {lo:.8f} – {hi:.8f} GAS (median {mid:.8f})')
    print('network fee (single signature + ~700-byte tx) adds roughly 0.005–0.01 GAS on top')
    print(f'\nall 4,830 wallets claiming: ~{(mid + 0.008) * 4830:,.1f} GAS network-wide —')
    print('paid per-wallet by each claimant at claim time, NEVER by the deployer')
