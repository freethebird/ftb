// mod-claim-page.js - the standalone CLAIM page (claim.html).
//
// Supersedes mod-claim.js for this page: it reads the FINAL v3 / R5 claim tree
// directly, so claim.html shows the definitive post-custody-release entitlements
// regardless of what the rest of the site still points at. One address -> its
// total claim, split by product (FLM / FLOCKS / LP / orderbook / lend / grant),
// the FTB it mints at the 100M cap, and its merkle proof. The claim signature
// runs only for the connected wallet, one claim per address, verified on-chain.
//
// Data: claim_breakdown_v3.json (small, loaded upfront - drives the whole
// explorer) and proofs_v3.json (the 5k-leaf proof set, loaded lazily only when
// a wallet actually claims).
//
// Owns (writes only these): #migState, #supplyLine, #capLine, #refreshBtn,
// #rootLine, #rootCount, #proofsBtn, #cxAddr, #cxGo, #cxMine, #cxOut, #cxMsg,
// #claimGo.
(function () {
  'use strict';
  if (!FtbUtil.$('cxSec')) return; // inert where the explorer markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, short = FtbUtil.short, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;

  // v3 / R5 final tree - the files this page serves (committed beside it).
  var PROOFS_URL = './proofs_v3.json';
  var BREAKDOWN_URL = './claim_breakdown_v3.json';

  // the committed cap arithmetic (DECISIONS.md): hard cap 100,000,000 FTB =
  // 10^16 base units; the v3 tree sums to ratioDen, so one FLM-equiv base unit
  // mints floor(amount * 10^16 / ratioDen). The on-chain migrator.quote() applies
  // the identical integer math once the v3 migrator is live.
  var RNUM = 10000000000000000n;              // 10^16  (100M FTB * 10^8)
  var RDEN = 54027173482587996n;              // v3 / R5 total (FLM-equiv base units)
  function ftbOf(amt) { return (BigInt(amt) * RNUM) / RDEN; }
  // headline form: two decimals max so a large number fits one line (the site's
  // tile convention); full precision stays in the tooltip and the detail rows.
  function fmt2(base) { var f = fmt(base), p = f.split('.'); return (p.length > 1 && p[1].length > 2) ? p[0] + '.' + p[1].slice(0, 2) : f; }

  // product metadata: field key -> [label, css class, one-line note]
  var FIELDS = [
    ['flm',       'FLM (direct)',     'p-flm',  'snapshot FLM balance at block 12,782,704'],
    ['flocks',    'FLOCKS (direct)',  'p-flk',  'snapshot FLOCKS balance, 1:1 with FLM'],
    ['lp',        'LP salvage',       'p-lp',   'your share of FLM locked in Flamingo pools'],
    ['orderbook', 'order book',       'p-ob',   'FLM locked in open FlamingoSwap limit orders'],
    ['vault',     'lend / vault',     'p-vt',   'FLM collateral in the FToken lending vaults'],
    ['grant',     'migration grant',  'p-gr',   'post-snapshot grant held in dev-wallet custody']
  ];

  // ---------- data ----------
  // breakdown (small) upfront; proofs (big) lazily, only to sign a claim.
  var compIdx = null, META = null, loadErr = null, proofsIdx = null;
  var loading = fetch(BREAKDOWN_URL).then(function (r) {
    if (!r.ok) throw new Error('claim_breakdown_v3.json not found'); return r.json();
  }).then(function (bd) {
    compIdx = bd.wallets || {};
    META = { root: bd.root, count: bd.count, fields: bd.fields || FIELDS.map(function (f) { return f[0]; }) };
    paintMeta(); return true;
  }).catch(function (e) { loadErr = e; paintMeta(); throw e; });

  function loadProofs() {
    if (proofsIdx) return Promise.resolve(proofsIdx);
    return fetch(PROOFS_URL).then(function (r) {
      if (!r.ok) throw new Error('proofs_v3.json not found'); return r.json();
    }).then(function (pf) {
      proofsIdx = {}; for (var i = 0; i < pf.entries.length; i++) proofsIdx[pf.entries[i].address] = pf.entries[i];
      return proofsIdx;
    });
  }

  // total entitlement = sum of the six product components (element [6] is the
  // merkle proof node count, not a component).
  function totalOf(comp) { var t = 0n; for (var i = 0; i < 6; i++) t += BigInt(comp[i] || '0'); return t; }

  // ---------- snapshot / root meta ----------
  function paintMeta() {
    if ($('rootLine')) $('rootLine').textContent = META ? short(META.root) : (loadErr ? 'unavailable' : '-');
    if ($('rootCount')) $('rootCount').textContent = META ? (META.count.toLocaleString('en-US') + ' holders') : 'holders -';
  }

  // ---------- migration status reads (live chain) ----------
  function refresh() {
    if (!FtbRpc.current() || !FtbState.configured) return;
    FtbRpc.readBool(C.ftb, 'migrationOpen').then(function (open) {
      var t = $('migState'); if (!t) return;
      t.textContent = open ? 'migration open' : 'mint sealed';
      t.className = open ? 'tag pub' : 'tag sealed';
    }).catch(function () {});
    FtbRpc.readInt(C.ftb, 'totalSupply').then(function (v) { if ($('supplyLine')) $('supplyLine').textContent = 'supply ' + fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(C.ftb, 'maxSupply').then(function (v) { if ($('capLine')) $('capLine').textContent = 'cap ' + fmt(v) + ' FTB'; }).catch(function () {});
  }

  // ---------- the breakdown explorer ----------
  var NEO_ADDR = /^N[1-9A-HJ-NP-Za-km-z]{33}$/;

  function msg(kind, text) { var m = $('cxMsg'); m.hidden = false; m.className = 'bprev' + (kind === 'err' ? ' cxerr' : ''); m.textContent = text; }
  function clearMsg() { var m = $('cxMsg'); m.hidden = true; m.textContent = ''; }

  function lookup(addrRaw) {
    var addr = (addrRaw || '').trim();
    $('cxOut').hidden = true; $('cxOut').innerHTML = '';
    if (!addr) { msg('err', 'paste a Neo N3 wallet address to see its claim.'); return; }
    if (!NEO_ADDR.test(addr)) { msg('err', 'that does not look like a Neo N3 address (starts with N, 34 characters).'); return; }
    msg('', 'reading the published tree…');
    loading.then(function () {
      clearMsg();
      var comp = compIdx[addr];
      if (!comp) { renderAbsent(addr); return; }
      render(addr, comp);
    }).catch(function (e) { msg('err', 'claim data unavailable: ' + errText(e)); });
  }

  function renderAbsent(addr) {
    var out = $('cxOut'); out.innerHTML = ''; out.hidden = false;
    out.appendChild(el('div', 'cxhead', 'not in the snapshot'));
    out.appendChild(el('div', 'cxnote', 'This address is not in the published pre-exploit snapshot, so it has no claim. Only pre-exploit holders (and salvaged LP / order-book / vault positions) are in the tree; the attacker and the flagged cluster are excluded by construction.'));
    out.appendChild(kvLine('address', addr));
  }

  function kvLine(k, v) {
    var row = el('div', 'cxkv'); row.appendChild(el('span', 'cxk', k));
    var val = el('span', 'cxv'); val.textContent = v; row.appendChild(val); return row;
  }

  function render(addr, comp) {
    var out = $('cxOut'); out.innerHTML = ''; out.hidden = false;
    var total = totalOf(comp);
    var nodes = comp[6] || '?';
    var ftb = ftbOf(total);

    // headline: FTB received (2dp) + full entitlement
    var head = el('div', 'cxresult');
    var big = el('div', 'cxbig');
    var ftbNum = el('span', 'cxftb', fmt2(ftb.toString())); ftbNum.title = fmt(ftb.toString()) + ' FTB';
    big.appendChild(ftbNum); big.appendChild(el('span', 'cxftbu', ' FTB'));
    head.appendChild(big);
    head.appendChild(el('div', 'cxsub', fmt(total.toString()) + ' FLM-equiv entitlement → ' + fmt(ftb.toString()) + ' FTB at the v3 / R5 cap (100,000,000 FTB hard cap)'));
    out.appendChild(head);

    // the product split
    var rows = el('div', 'cxbars'), fields = META.fields;
    for (var i = 0; i < FIELDS.length; i++) {
      var key = FIELDS[i][0], idx = fields.indexOf(key);
      var raw = idx >= 0 ? (comp[idx] || '0') : '0';
      if (raw === '0') continue;
      var share = total > 0n ? Number((BigInt(raw) * 10000n) / total) / 100 : 0; // % of total, 2dp
      var row = el('div', 'cxbar');
      var lbl = el('div', 'cxblabel'); lbl.appendChild(el('span', 'cxdot ' + FIELDS[i][2])); lbl.appendChild(el('span', null, FIELDS[i][1]));
      row.appendChild(lbl);
      var track = el('div', 'cxtrack'); var fill = el('div', 'cxfill ' + FIELDS[i][2]); fill.style.width = Math.max(share, 0.6) + '%'; track.appendChild(fill); row.appendChild(track);
      var v = el('div', 'cxbval'); v.appendChild(el('span', 'cxbnum', fmt(raw))); v.appendChild(el('span', 'cxbpct', share >= 0.01 ? share.toFixed(2) + '%' : '<0.01%')); v.title = FIELDS[i][3]; row.appendChild(v);
      rows.appendChild(row);
    }
    out.appendChild(rows);

    // proof + address
    var meta = el('div', 'cxmeta');
    meta.appendChild(kvLine('address', addr));
    meta.appendChild(kvLine('entitlement', fmt(total.toString()) + ' FLM-equiv'));
    meta.appendChild(kvLine('you receive', fmt(ftb.toString()) + ' FTB'));
    meta.appendChild(kvLine('merkle proof', 'in the published tree · ' + nodes + '-node proof'));
    out.appendChild(meta);

    // claim action - only for the connected wallet (you can only claim your own)
    var act = el('div', 'cxact');
    if (FtbState.account && FtbState.account === addr) {
      maybeClaimButton(act, addr, total);
    } else {
      act.appendChild(el('div', 'cxnote', FtbState.account ? 'Connected as a different address. Connect the wallet that owns this address to claim it.' : 'This is a read-only view. Connect that wallet to claim its FTB.'));
    }
    out.appendChild(act);
  }

  function maybeClaimButton(act, addr, total) {
    act.appendChild(el('div', 'cxnote', 'checking the on-chain claim ledger…'));
    FtbRpc.readBool(C.migrator, 'claimed', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(function (done) {
      act.innerHTML = '';
      if (done) { act.appendChild(el('div', 'cxnote', 'This address has already claimed its FTB — one claim per snapshot address, forever. The ledger lives in the token contract.')); return; }
      addClaimBtn(act, addr);
    }).catch(function () { act.innerHTML = ''; addClaimBtn(act, addr); act.appendChild(el('div', 'cxnote', 'Could not pre-read the claimed ledger — the chain enforces one claim per address regardless.')); });
  }
  function addClaimBtn(act, addr) {
    var b = el('button', 'bkey', 'claim this →'); b.type = 'button';
    b.addEventListener('click', function () { signClaim(addr); });
    act.appendChild(b);
    act.appendChild(el('div', 'cxnote', 'One claim per address, verified on-chain against root ' + short(META.root) + '. Check the hashes in verify before you sign.'));
  }

  function norm(h) { return (h || '').toLowerCase().replace(/^0x/, ''); }

  function signClaim(addr) {
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    // The page serves the final v3 / R5 tree. If the deployed migrator still
    // carries an earlier root, a v3 proof cannot verify on-chain - so don't
    // submit a doomed transaction; say plainly that v3 claiming isn't live yet.
    if (META && CFG.merkleRoot && norm(CFG.merkleRoot) !== norm(META.root)) {
      FtbShell.showPlan('v3 / R5 not live yet',
        'This page shows the final v3 / R5 claim tree, but the deployed migrator still carries the previous root — so a v3 proof would not verify on-chain yet. v3 claiming opens when the R5 migrator is deployed at MainNet launch. Your breakdown above is final; the claim button activates once the on-chain root matches.',
        [['page root (v3/R5)', short(META.root)], ['deployed root', short(CFG.merkleRoot)]], null);
      return;
    }
    loadProofs().then(function (idx) {
      var e = idx[addr];
      if (!e) { FtbShell.showPlan('proof unavailable', 'could not find a proof for this address in proofs_v3.json.', [['address', addr]], null); return; }
      var ftb = ftbOf(e.amount);
      var plan = FtbState.dapi.claim(FtbState.account, e.amount, e.proof);
      FtbShell.showPlan('claim ' + fmt2(ftb.toString()) + ' FTB',
        'one claim per address, verified on-chain against the published v3 / R5 merkle root. the migrator converts at the committed cap ratio. check the hashes in verify before you sign.',
        [['address', e.address], ['entitlement', fmt(e.amount) + ' FLM-equiv'], ['you receive', fmt(ftb.toString()) + ' FTB'], ['proof', e.proof.length + ' nodes'], ['what you sign', plan.summary]], plan);
    }).catch(function (er) { FtbShell.showPlan('proofs unavailable', errText(er), [['proofs file', PROOFS_URL]], null); });
  }

  // ---------- wiring ----------
  $('cxGo').addEventListener('click', function () { lookup($('cxAddr').value); });
  $('cxAddr').addEventListener('keydown', function (e) { if (e.key === 'Enter') lookup($('cxAddr').value); });
  function useMine() { if (!FtbState.account) { FtbWallet.needConnect(); return; } $('cxAddr').value = FtbState.account; lookup(FtbState.account); }
  if ($('cxMine')) $('cxMine').addEventListener('click', useMine);
  if ($('claimGo')) $('claimGo').addEventListener('click', useMine);
  if ($('proofsBtn')) $('proofsBtn').addEventListener('click', function () { window.open(PROOFS_URL, '_blank'); });

  function flash(b) { if (!b) return; var t = b.textContent; b.textContent = 'reading…'; b.disabled = true; setTimeout(function () { b.textContent = t; b.disabled = false; }, 1200); }
  if ($('refreshBtn')) $('refreshBtn').addEventListener('click', function () { flash($('refreshBtn')); refresh(); });

  FtbState.on('account', function () { if (FtbState.account && !$('cxAddr').value) $('cxAddr').value = FtbState.account; });
  FtbState.on('rpc', refresh);
  FtbState.on('refresh', refresh);
})();
