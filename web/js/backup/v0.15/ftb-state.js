// ftb-state.js - shared page state, event hub, and the two cross-module seams
// (MODULES.md R7; closes WEB-05 and WEB-02).
//
// Owns no DOM. Holds: the connected account, the dapi instance, the lazy
// proofs cache (fetched once, indexed once), and quoteFtb - the conversion
// preview seam that reads the migrator's on-chain quote() and only falls back
// to the local ratio mirror when the chain cannot answer.
//
// Events: 'rpc' (a healthy endpoint is serving reads), 'account' (wallet
// connected / changed / disconnected), 'refresh' (a transaction landed -
// re-read what you display).
(function () {
  'use strict';

  var CFG = window.FTB_CONFIG || {};
  var C = CFG.contracts || {};
  var PROOFS_URL = CFG.proofsUrl || './proofs.json';
  var configured = !!(C.ftb && C.ftb.indexOf('0x0000') !== 0);

  // ---------- tiny shared helpers ----------
  function $(id) { return document.getElementById(id); }
  function el(tag, cls, text) { var e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; }
  function short(h) { return (h && h.length > 14) ? h.slice(0, 8) + '…' + h.slice(-6) : (h || '-'); }
  // thousands separators on the integer part of a decimal string - every displayed
  // number is readable at a glance (100,000,000 not 100000000). Display-only; input
  // fields and builder arguments always use the raw string.
  function grp(s) {
    var p = String(s).split('.');
    p[0] = p[0].replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    return p.join('.');
  }
  function fmt(base) { return grp(FtbDapi.util.fromBaseUnits(base.toString())); }
  // tile headline: two decimals max so the number always fits one line; the FULL
  // precision value goes in the element's tooltip - trim the display, never the data.
  function tileNum(elm, base, suffix) {
    var full = fmt(base);
    var p = full.split('.');
    var short2 = p.length > 1 && p[1].length > 2 ? p[0] + '.' + p[1].slice(0, 2) : full;
    elm.textContent = short2 + (suffix || '');
    elm.title = full + (suffix || '');
  }
  // Refresh coalescing, shared by every module that loads on 'refresh' bursts:
  // one read pass in flight, at most one trailing run queued, a minimum interval
  // between passes. Without it, rapid events start overlapping reads and a slower
  // STALE pass can resolve last - painting old numbers over fresh ones.
  function coalesce(fn, minMs) {
    var inflight = false, last = 0, timer = null, queued = false;
    function done() {
      inflight = false;
      if (queued) timer = setTimeout(run, Math.max(0, last + minMs - Date.now()));
    }
    function run() {
      queued = false;
      if (timer) { clearTimeout(timer); timer = null; }
      inflight = true; last = Date.now();
      Promise.resolve(fn()).then(done, done);
    }
    return function () {
      if (inflight) { queued = true; return; }
      var wait = last + minMs - Date.now();
      if (wait <= 0) run();
      else if (!timer) timer = setTimeout(run, wait);
    };
  }
  // wallet errors are often plain objects ({type, description, data}) - never String()
  // them raw, or the user reads "[object Object]" and learns nothing.
  function errText(e) {
    if (!e) return 'unknown error';
    if (typeof e === 'string') return e;
    var m = e.message || e.description || e.error || '';
    if (m && typeof m === 'object') { try { m = JSON.stringify(m); } catch (_) { m = String(m); } }
    var d = e.data ? (typeof e.data === 'string' ? e.data : (function () { try { return JSON.stringify(e.data); } catch (_) { return String(e.data); } })()) : '';
    if (!m && !d) { try { return JSON.stringify(e); } catch (_) { return String(e); } }
    return d ? m + ' - ' + d : m;
  }

  // ---------- event hub ----------
  var listeners = {};
  function on(ev, fn) { (listeners[ev] = listeners[ev] || []).push(fn); }
  function emit(ev, a) {
    (listeners[ev] || []).slice().forEach(function (f) { try { f(a); } catch (e) {} });
  }

  // ---------- balance chips ----------
  // A clickable "you have X SYM · max" chip beside an amount input: shows the
  // connected wallet's live balance of one token and, on click, hands the raw
  // BigInt to onFill (the module formats for its own input - NEO is whole
  // units). Chips re-read on rpc/account/refresh, so a confirmed transaction
  // visibly moves the numbers without opening the wallet. Detached chips
  // unregister themselves via isConnected - wizards rebuild DOM constantly and
  // must not leak listeners.
  // tokenish: a Hash160 string, or a zero-arg function returning one (or null)
  // for handles resolved later (e.g. an LP pair hash loaded asynchronously).
  var chips = [];
  function chipSweep() { chips = chips.filter(function (draw) { return draw(); }); }
  function balChip(tokenish, sym, decimals, onFill) {
    var chip = el('button', 'balchip', sym + ' …');
    chip.type = 'button';
    var raw = null;
    function draw() {
      if (!chip.isConnected) return false; // removed from the DOM - drop me
      var acct = FtbState.acctHash160();
      var tok = typeof tokenish === 'function' ? tokenish() : tokenish;
      if (!acct || !tok || !window.FtbRpc || !FtbRpc.current()) {
        raw = null;
        chip.textContent = acct ? (sym + ' …') : 'connect to see your ' + sym;
        return true;
      }
      FtbRpc.readInt(tok, 'balanceOf', [{ type: 'Hash160', value: acct }]).then(function (v) {
        if (!chip.isConnected) return;
        raw = v;
        chip.textContent = 'you have ' + (decimals === 0 ? grp(v.toString()) : fmt(v)) + ' ' + sym + ' · max';
        chip.title = 'click to fill the input with your full ' + sym + ' balance';
      }).catch(function () { if (chip.isConnected) chip.textContent = sym + ' unreadable'; });
      return true;
    }
    chip.addEventListener('click', function () {
      if (!FtbState.account) { if (window.FtbWallet) FtbWallet.needConnect(); return; }
      if (raw !== null && raw > 0n) onFill(raw);
    });
    chips.push(draw);
    draw();
    return chip;
  }

  // ---------- proofs (lazy, fetched once, indexed once) ----------
  var proofsCache = null, proofsIdx = null;
  function proofs() {
    if (proofsCache) return Promise.resolve(proofsCache);
    return fetch(PROOFS_URL).then(function (r) {
      if (!r.ok) throw new Error('proofs file not found at ' + PROOFS_URL);
      return r.json();
    }).then(function (j) {
      proofsCache = j;
      proofsIdx = {};
      for (var i = 0; i < j.entries.length; i++) proofsIdx[j.entries[i].address] = j.entries[i];
      return j;
    });
  }
  function entryFor(addr) { return (proofsIdx && addr) ? (proofsIdx[addr] || null) : null; }

  // ---------- node status (live heartbeat, never cached) ----------
  // The node's own /status.json, written per-minute by ftb-status and served by Caddy
  // OUTSIDE the IPFS bundle, so it reflects the live node between deploys. An artifact
  // read, through this seam (R3) - not a module fetch and not a chain read. Live, so
  // never cached: each call re-reads. The URL is config-only (R5); absent => reject, and
  // the consuming module shows its named standby (R8).
  var STATUS_URL = CFG.statusUrl || '';
  function status() {
    if (!STATUS_URL) return Promise.reject(new Error('no statusUrl configured'));
    var ctl = typeof AbortController !== 'undefined' ? new AbortController() : null;
    var to = ctl ? setTimeout(function () { ctl.abort(); }, 8000) : null;
    return fetch(STATUS_URL, { cache: 'no-store', signal: ctl ? ctl.signal : undefined })
      .then(function (r) { if (!r.ok) throw new Error('status ' + r.status); return r.json(); })
      .finally(function () { if (to) clearTimeout(to); });
  }

  // ---------- the conversion seam (WEB-02) ----------
  // The preview reads migrator.quote() on-chain - the same integer arithmetic the
  // mint will apply (floor(amount × ratioNum / ratioDen), DECISIONS.md 18 Oct).
  // The local mirror FtbDapi.util.ftbOf is only a fallback for when the chain
  // cannot answer; the chain converts authoritatively at mint time regardless.
  function quoteFtb(flmBase) {
    var mig = C.migrator;
    if (mig && mig.indexOf('0x0000') !== 0 && window.FtbRpc && FtbRpc.current()) {
      return FtbRpc.readInt(mig, 'quote', [{ type: 'Integer', value: flmBase.toString() }])
        .catch(function () { return BigInt(FtbDapi.util.ftbOf(flmBase)); });
    }
    return Promise.resolve(BigInt(FtbDapi.util.ftbOf(flmBase)));
  }

  window.FtbUtil = { $: $, el: el, short: short, grp: grp, fmt: fmt, tileNum: tileNum, errText: errText, coalesce: coalesce, balChip: balChip };
  window.FtbState = {
    CFG: CFG,
    C: C,
    configured: configured,
    PROOFS_URL: PROOFS_URL,
    account: null,
    dapi: null,
    on: on,
    emit: emit,
    proofs: proofs,
    entryFor: entryFor,
    status: status,
    quoteFtb: quoteFtb,
    acctHash160: function () {
      return this.account ? '0x' + FtbDapi.util.addressToScriptHashHex(this.account, { le: true }) : null;
    }
  };
  // refresh events arrive in bursts around every signed tx - one coalesced
  // sweep for all chips, same discipline as the module boards.
  var chipSweepC = coalesce(chipSweep, 3000);
  on('rpc', chipSweepC);
  on('account', chipSweepC);
  on('refresh', chipSweepC);
})();
