// mod-explorer.js — the user-facing block explorer (explorer.html).
//
// Paste a Neo N3 transaction id → a plain, icon-first view: what moved, from → to,
// the GAS it cost, and when it became final. Paste a wallet address (N…) or a
// contract script hash (0x…) → its token balances and recent transfer history.
// No VM internals, no script bytes — the information a person actually needs.
//
// Reads the chain through FtbRpc only (getapplicationlog + getrawtransaction for a
// tx; getnep17balances + getnep17transfers for an address, on the network this site
// is configured for). Dependency-free; BigInt for every amount; DOM built with
// createElement/textContent — chain data never reaches innerHTML (Canonical Bible,
// JS edition: the UNSAFE inventory stays empty).
//
// Article 10 / 12 (the shared toolkit, the Occam's razor): address rendering and
// amount formatting are NOT re-implemented here — they are the adapter's canonical
// rails, FtbDapi.util.scriptHashToAddress and .fromBaseUnits, which use the pure-JS
// SHA-256 (never crypto.subtle) so the explorer works on plain-http mirrors — own
// node, local IPFS gateway — exactly as the lend board does.
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('exSec')) return; // inert where the explorer markup is absent
  if (!window.FtbDapi || !FtbDapi.util) return;        // the shared rails must be present

  var $ = FtbUtil.$, el = FtbUtil.el, short = FtbUtil.short, errText = FtbUtil.errText, grp = FtbUtil.grp;
  var DU = FtbDapi.util; // the canonical chain primitives (Article 10)
  var CFG = window.FTB_CONFIG || {}, C = CFG.contracts || {}, A = CFG.assets || {};

  // known tokens: lowercase 0x BE hash -> { sym, dec, cls }. Unknown tokens are
  // looked up on demand (symbol + decimals) and drawn with the generic coin glyph.
  var KNOWN = {};
  function reg(h, sym, dec, cls) { if (h) KNOWN[String(h).toLowerCase()] = { sym: sym, dec: dec, cls: cls }; }
  reg(C.ftb, 'FTB', 8, 't-ftb');
  reg(A.gas, 'GAS', 8, 't-gas');
  reg(A.neo, 'NEO', 0, 't-neo');
  reg('0xf0151f528127558851b39c2cd8aa47da7418ab28', 'FLM', 8, 't-flm');
  reg('0xdad88d8c8d4643548aa0f63c83796e81c3c6b04c', 'FLOCKS', 8, 't-flk');
  var tokCache = {};

  function amt(base, dec) { return grp(DU.fromBaseUnits(String(base), dec)); }
  function hue(seed) { seed = String(seed); var n = 0; for (var i = 0; i < seed.length; i++) n = (n * 31 + seed.charCodeAt(i)) >>> 0; return n % 360; }

  // ---------- icons (inline SVG; no external asset) ----------
  function svg(paths, extra) {
    var s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    s.setAttribute('viewBox', '0 0 24 24'); s.setAttribute('fill', 'none');
    s.setAttribute('stroke', 'currentColor'); s.setAttribute('stroke-width', '2');
    s.setAttribute('stroke-linecap', 'round'); s.setAttribute('stroke-linejoin', 'round');
    s.setAttribute('class', 'exico' + (extra ? ' ' + extra : '')); s.setAttribute('aria-hidden', 'true');
    paths.forEach(function (d) { var p = document.createElementNS('http://www.w3.org/2000/svg', 'path'); p.setAttribute('d', d); s.appendChild(p); });
    return s;
  }
  var ICO = {
    coin: function () { return svg(['M12 3a9 4 0 1 0 0 8 9 4 0 1 0 0-8', 'M3 7v5a9 4 0 0 0 18 0V7', 'M3 12v5a9 4 0 0 0 18 0v-5']); },
    gas: function () { return svg(['M5 21V5a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v16', 'M3 21h14', 'M15 9h2a2 2 0 0 1 2 2v5a1.5 1.5 0 0 0 3 0V8l-3-3']); },
    ok: function () { return svg(['M20 6 9 17l-5-5'], 'ex-ok'); },
    bad: function () { return svg(['M18 6 6 18', 'M6 6l12 12'], 'ex-bad'); },
    clock: function () { return svg(['M12 7v5l3 2', 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0']); },
    arrow: function () { return svg(['M5 12h14', 'M13 6l6 6-6 6'], 'ex-arrow'); },
    mint: function () { return svg(['M12 5v14', 'M5 12h14']); },
    burn: function () { return svg(['M12 3s6 5 6 10a6 6 0 0 1-12 0c0-2 1-3 2-4 0 2 1 3 2 3 0-3 2-5 2-9z']); },
    wallet: function () { return svg(['M3 7a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v2', 'M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-6a2 2 0 0 0-2-2H6', 'M16 13h.01']); },
    down: function () { return svg(['M12 5v14', 'M6 13l6 6 6-6'], 'ex-in'); },
    up: function () { return svg(['M12 19V5', 'M6 11l6-6 6 6'], 'ex-out'); }
  };

  function tokenChip(hashLower, meta) {
    var chip = el('span', 'extok ' + (meta.cls || 't-unk'));
    chip.appendChild(ICO.coin());
    chip.appendChild(el('span', 'extok-sym', meta.sym));
    chip.title = hashLower;
    return chip;
  }
  function addrChip(addr, seed) {
    var chip = el('span', 'exaddr');
    var sq = el('span', 'exid'); sq.style.background = 'hsl(' + hue(seed || addr) + ' 55% 45%)';
    chip.appendChild(sq);
    chip.appendChild(el('span', 'exaddr-t', short(addr)));
    chip.title = addr; return chip;
  }

  function tokenMeta(hashBE) {
    var low = String(hashBE).toLowerCase();
    if (KNOWN[low]) return Promise.resolve(KNOWN[low]);
    if (tokCache[low]) return Promise.resolve(tokCache[low]);
    return Promise.all([
      FtbRpc.call('invokefunction', [hashBE, 'symbol', []]).catch(function () { return null; }),
      FtbRpc.call('invokefunction', [hashBE, 'decimals', []]).catch(function () { return null; })
    ]).then(function (r) {
      var sym = '?', dec = 0;
      try { if (r[0] && r[0].stack && r[0].stack[0]) sym = atob(r[0].stack[0].value); } catch (e) {}
      try { if (r[1] && r[1].stack && r[1].stack[0]) dec = parseInt(r[1].stack[0].value, 10) || 0; } catch (e) {}
      var m = { sym: sym || '?', dec: dec, cls: 't-unk' }; tokCache[low] = m; return m;
    });
  }

  // ---------- input routing ----------
  var TXID = /^0x[0-9a-fA-F]{64}$/;
  var SHASH = /^0x[0-9a-fA-F]{40}$/;
  var NADDR = /^N[1-9A-HJ-NP-Za-km-z]{33}$/; // Neo N3 base58 address
  function msg(kind, text) { var m = $('exMsg'); m.hidden = false; m.className = 'bprev' + (kind === 'err' ? ' cxerr' : ''); m.textContent = text; }
  function clearMsg() { var m = $('exMsg'); m.hidden = true; m.textContent = ''; }

  function lookup(raw) {
    var q = (raw || '').trim();
    $('exOut').hidden = true; $('exOut').replaceChildren();
    if (!q) { msg('err', 'paste a transaction id, a wallet address, or a contract hash.'); return; }
    if (NADDR.test(q)) return lookupAddr(q);
    var hx = (q.indexOf('0x') === 0 || /^[0-9a-fA-F]+$/.test(q)) ? (q.indexOf('0x') === 0 ? q : '0x' + q) : q;
    if (TXID.test(hx)) return lookupTx(hx);
    if (SHASH.test(hx)) return lookupAddr(hx);
    msg('err', 'that does not look like a Neo N3 transaction id (0x + 64 hex), a wallet address (starts with N), or a contract hash (0x + 40 hex).');
  }

  // ---------- transaction lookup ----------
  function lookupTx(tx) {
    if (!FtbRpc.current()) { msg('err', 'no node is connected yet — one moment, then try again.'); return; }
    msg('', 'reading the ledger…');
    Promise.all([
      FtbRpc.call('getapplicationlog', [tx]),
      FtbRpc.call('getrawtransaction', [tx, 1])
    ]).then(function (r) { return render(tx, r[0], r[1]); })
      .then(clearMsg)
      .catch(function (e) {
        var s = errText(e);
        if (/Unknown transaction|not found|not be found|-100|does not exist/i.test(s)) msg('err', 'no transaction with that id on this network. (The site is reading ' + (CFG.network || 'the configured network') + '.)');
        else msg('err', 'could not read that transaction: ' + s);
      });
  }

  function transfers(appLog) {
    var out = [];
    (appLog.executions || []).forEach(function (ex) {
      (ex.notifications || []).forEach(function (n) {
        if (n.eventname !== 'Transfer') return;
        var v = n.state && n.state.value; if (!v || v.length < 3) return;
        out.push({ token: n.contract, from: v[0], to: v[1], amount: (v[2] && v[2].value != null) ? String(v[2].value) : '0' });
      });
    });
    return out;
  }
  function party(item) {
    if (!item || item.type === 'Any' || item.value == null || item.value === '') return null;
    var hex = DU.b64ToHex(item.value);
    if (hex.length !== 40) return null;
    return { hex: hex, addr: DU.scriptHashToAddress(hex) };
  }

  function render(tx, appLog, rawTx) {
    var ex = (appLog.executions && appLog.executions[0]) || {};
    var halted = ex.vmstate === 'HALT';
    var mv = transfers(appLog);
    mv.forEach(function (t) { t._from = party(t.from); t._to = party(t.to); });
    return Promise.all(mv.map(function (t) { return tokenMeta(t.token).then(function (m) { t._m = m; }); })).then(function () {
      var out = $('exOut'); out.replaceChildren(); out.hidden = false;
      var head = el('div', 'exhead');
      head.appendChild(halted ? ICO.ok() : ICO.bad());
      var ht = el('div', 'exhead-t');
      ht.appendChild(el('div', 'exhead-k', halted ? 'transaction succeeded' : 'transaction failed on-chain'));
      var idrow = el('div', 'exid-row'); idrow.appendChild(el('span', 'exid-txt', tx));
      ht.appendChild(idrow); head.appendChild(ht); out.appendChild(head);
      var moved = el('section', 'exmoved');
      if (!mv.length) {
        moved.appendChild(el('div', 'exempty', 'No tokens moved in this transaction.'));
      } else {
        mv.forEach(function (t) {
          var row = el('div', 'exmv');
          row.appendChild(tokenChip(String(t.token).toLowerCase(), t._m));
          row.appendChild(el('span', 'examt', amt(t.amount, t._m.dec) + ' ' + t._m.sym));
          var flow = el('span', 'exflow');
          if (!t._from) { flow.appendChild(ICO.mint()); flow.appendChild(el('span', 'exflow-l', 'minted to')); flow.appendChild(addrChip(t._to.addr, t._to.hex)); }
          else if (!t._to) { flow.appendChild(ICO.burn()); flow.appendChild(el('span', 'exflow-l', 'burned from')); flow.appendChild(addrChip(t._from.addr, t._from.hex)); }
          else { flow.appendChild(addrChip(t._from.addr, t._from.hex)); flow.appendChild(ICO.arrow()); flow.appendChild(addrChip(t._to.addr, t._to.hex)); }
          row.appendChild(flow);
          moved.appendChild(row);
        });
      }
      out.appendChild(moved);
      var facts = el('section', 'exfacts');
      var sys = rawTx && rawTx.sysfee ? BigInt(rawTx.sysfee) : 0n;
      var net = rawTx && rawTx.netfee ? BigInt(rawTx.netfee) : 0n;
      facts.appendChild(tile(ICO.gas(), 'gas paid', amt((sys + net).toString(), 8) + ' GAS'));
      var conf = rawTx && (rawTx.confirmations != null);
      if (conf) {
        var when = rawTx.blocktime ? new Date(Number(rawTx.blocktime)) : null;
        var fin = tile(ICO.ok(), 'finality', 'final · one block · ~15s'); fin.classList.add('ex-okt'); facts.appendChild(fin);
        if (when) facts.appendChild(tile(ICO.clock(), 'when', fmtTime(rawTx.blocktime)));
      } else {
        facts.appendChild(tile(ICO.clock(), 'finality', 'pending · not yet in a block'));
      }
      out.appendChild(facts);
      out.appendChild(el('p', 'bnote', 'Neo N3 has single-block finality: a transaction is irreversible the moment it is included in a block — there are no confirmations to wait for and no re-orgs. “GAS paid” is the network + system fee the sender spent.'));
    });
  }

  // ---------- address lookup (NEP-17 balances + transfer history) ----------
  function balMeta(b) {
    var low = String(b.assethash).toLowerCase(), k = KNOWN[low];
    return { sym: (k && k.sym) || b.symbol || '?', dec: (k ? k.dec : parseInt(b.decimals, 10)) || 0, cls: (k && k.cls) || 't-unk', hash: low };
  }
  function fmtTime(ms) { if (ms == null) return ''; try { return new Date(Number(ms)).toISOString().replace('T', ' ').replace(/\..+/, ' UTC'); } catch (e) { return ''; } }

  function lookupAddr(id) {
    if (!FtbRpc.current()) { msg('err', 'no node is connected yet — one moment, then try again.'); return; }
    msg('', 'reading the ledger…');
    Promise.all([
      FtbRpc.call('getnep17balances', [id]),
      FtbRpc.call('getnep17transfers', [id]).catch(function () { return null; })
    ]).then(function (r) { return renderAddr(id, r[0], r[1]); })
      .then(clearMsg)
      .catch(function (e) {
        var s = errText(e);
        if (/Method not found|-32601/i.test(s)) msg('err', 'this node does not expose the NEP-17 tracker, so address lookups are unavailable here — try your own node or another RPC.');
        else msg('err', 'could not read that address: ' + s);
      });
  }

  function renderAddr(id, bal, xfer) {
    var balances = (bal && bal.balance) || [];
    // collect transfer rows (sent = out, received = in), newest first
    var rows = [];
    function push(list, dir) {
      (list || []).forEach(function (e) {
        rows.push({ dir: dir, t: e.timestamp, token: e.assethash, amount: String(e.amount != null ? e.amount : '0'), counter: e.transferaddress || null, tx: e.txhash || null });
      });
    }
    if (xfer) { push(xfer.sent, 'out'); push(xfer.received, 'in'); }
    rows.sort(function (a, b) { return Number(b.t || 0) - Number(a.t || 0); });
    var LIMIT = 30, shown = rows.slice(0, LIMIT);

    // resolve metadata for every token that appears (balances give it directly; the
    // rest via KNOWN or an on-demand symbol/decimals read)
    var need = {};
    balances.forEach(function (b) { need[String(b.assethash).toLowerCase()] = balMeta(b); });
    var pending = [];
    shown.forEach(function (r) { var low = String(r.token).toLowerCase(); if (!need[low]) pending.push(low); });
    pending = pending.filter(function (v, i, s) { return s.indexOf(v) === i; });

    return Promise.all(pending.map(function (low) { return tokenMeta(low).then(function (m) { need[low] = { sym: m.sym, dec: m.dec, cls: m.cls, hash: low }; }); })).then(function () {
      var out = $('exOut'); out.replaceChildren(); out.hidden = false;

      // headline: wallet glyph + the address
      var head = el('div', 'exhead');
      var sq = el('span', 'exid exid-lg'); sq.style.background = 'hsl(' + hue(id) + ' 55% 45%)'; head.appendChild(sq);
      var ht = el('div', 'exhead-t');
      ht.appendChild(el('div', 'exhead-k', NADDR.test(id) ? 'wallet' : 'contract'));
      var idrow = el('div', 'exid-row'); idrow.appendChild(el('span', 'exid-txt', id));
      ht.appendChild(idrow); head.appendChild(ht); out.appendChild(head);

      // balances
      out.appendChild(el('div', 'exsub', 'holdings'));
      var bsec = el('section', 'exbals');
      if (!balances.length) {
        bsec.appendChild(el('div', 'exempty', 'No NEP-17 token balances on ' + (CFG.network || 'this network') + '.'));
      } else {
        balances.forEach(function (b) {
          var m = need[String(b.assethash).toLowerCase()];
          var row = el('div', 'exbal');
          row.appendChild(tokenChip(m.hash, m));
          row.appendChild(el('span', 'examt', amt(b.amount, m.dec)));
          row.appendChild(el('span', 'exbal-sym', m.sym));
          bsec.appendChild(row);
        });
      }
      out.appendChild(bsec);

      // recent transfers
      out.appendChild(el('div', 'exsub', 'recent transfers' + (rows.length > LIMIT ? ' (newest ' + LIMIT + ' of ' + rows.length + ')' : '')));
      var tsec = el('section', 'extxs');
      if (!shown.length) {
        tsec.appendChild(el('div', 'exempty', xfer ? 'No recent NEP-17 transfers in the window this node returns.' : 'Transfer history is unavailable from this node.'));
      } else {
        shown.forEach(function (r) {
          var m = need[String(r.token).toLowerCase()] || { sym: '?', dec: 0, cls: 't-unk', hash: String(r.token).toLowerCase() };
          var tr = el('div', 'extr ' + (r.dir === 'in' ? 'extr-in' : 'extr-out'));
          var d = el('span', 'extr-dir');
          d.appendChild(r.dir === 'in' ? ICO.down() : ICO.up());
          d.appendChild(el('span', 'extr-dl', r.dir === 'in' ? 'in' : 'out'));
          tr.appendChild(d);
          tr.appendChild(el('span', 'extr-amt', (r.dir === 'in' ? '+' : '−') + amt(r.amount, m.dec) + ' ' + m.sym));
          var who = el('span', 'extr-who');
          who.appendChild(el('span', 'extr-wl', r.dir === 'in' ? 'from' : 'to'));
          if (r.counter) who.appendChild(addrChip(r.counter, r.counter)); else who.appendChild(el('span', 'exaddr-t', r.dir === 'in' ? 'minted' : 'burned'));
          tr.appendChild(who);
          tr.appendChild(el('span', 'extr-when', fmtTime(r.t)));
          if (r.tx) {
            var txl = el('button', 'extr-tx', short(r.tx)); txl.type = 'button'; txl.title = r.tx;
            txl.addEventListener('click', function () { $('exTx').value = r.tx; lookup(r.tx); });
            tr.appendChild(txl);
          }
          tsec.appendChild(tr);
        });
      }
      out.appendChild(tsec);

      out.appendChild(el('p', 'bnote', 'Balances and transfers come from this node’s NEP-17 tracker on ' + (CFG.network || 'the configured network') + '. History is limited to the window the node keeps; click any tx to open it in full. Nothing you paste leaves your browser.'));
    });
  }

  function tile(icon, k, v) {
    var t = el('div', 'extile');
    var ic = el('div', 'extile-ic'); ic.appendChild(icon); t.appendChild(ic);
    var b = el('div', 'extile-b');
    b.appendChild(el('div', 'extile-k', k));
    b.appendChild(el('div', 'extile-v', v));
    t.appendChild(b); return t;
  }

  // ---------- wire ----------
  $('exGo').addEventListener('click', function () { lookup($('exTx').value); });
  $('exTx').addEventListener('keydown', function (e) { if (e.key === 'Enter') lookup($('exTx').value); });
  (function () { try { var q = new URLSearchParams(location.search).get('q'); if (q) { var t = $('exTx'); if (t) { t.value = q; } if (window.FtbRpc) { FtbRpc.pick().then(function (u) { if (window.FtbState) { FtbState.emit('rpc', u); } lookup(q); }).catch(function () { lookup(q); }); } else { lookup(q); } } } catch (ignore) {} })();

  window.FtbExplorer = { lookup: lookup, lookupTx: lookupTx, lookupAddr: lookupAddr, _amt: amt, _party: party, _transfers: transfers };
})();
