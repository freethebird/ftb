// ftb-rpc.js - the ONE JSON-RPC read stack (MODULES.md R3; closes WEB-01).
// Every module reads the chain through FtbRpc - no module opens its own fetch
// to an RPC endpoint, and no module re-implements pick/failover.
//
// Owns (writes only these): #rpcNote, and the #ovRpc own-node modal
// (#ownRpcBtn, #ownRpcIn, #ownRpcGo, #ownRpcReset, #ownRpcMsg).
(function () {
  'use strict';

  var CFG = window.FTB_CONFIG || {};
  var NET = CFG.network || 'TestNet';
  var PUB = (CFG.publicRpcs && CFG.publicRpcs.length) ? CFG.publicRpcs.slice()
    : (NET === 'MainNet'
        ? ['https://mainnet1.neo.coz.io:443', 'https://mainnet2.neo.coz.io:443']
        : ['https://testnet1.neo.coz.io:443', 'https://testnet2.neo.coz.io:443']);
  var RPCS = (CFG.rpcs && CFG.rpcs.length ? CFG.rpcs.slice() : []).concat(PUB)
    .filter(function (u, i, a) { return u && a.indexOf(u) === i; });
  try { var own = localStorage.getItem('ftb_rpc'); if (own && RPCS.indexOf(own) < 0) RPCS.unshift(own); } catch (e) {}

  var RPC = null;
  // probe every endpoint's height, then serve reads from the first one at tip.
  // a node that answers but is far behind is still syncing: alive for the
  // counter, but its chain state is from the past - contracts deployed later
  // are "not found" there, so it must not serve reads until it catches up.
  var SYNC_SLACK = 1000; // blocks (~4 h at 15 s); anything further behind is syncing

  function $(id) { return document.getElementById(id); }

  function call(url, method, params) {
    // a node that accepts the socket but never answers must not stall pick() or a
    // read forever (canon II.4/5): 10 s and the call is a failure like any other.
    var ctl = typeof AbortController !== 'undefined' ? new AbortController() : null;
    var to = ctl ? setTimeout(function () { ctl.abort(); }, 10000) : null;
    return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: method, params: params || [] }),
      signal: ctl ? ctl.signal : undefined })
      .then(function (r) { return r.json(); })
      .then(function (j) { if (j.error) throw new Error(j.error.message || 'rpc error'); return j.result; })
      .finally(function () { if (to) clearTimeout(to); });
  }

  // F1 remediation: make the serving node's TRUST STATUS explicit. Reads size and label the
  // transactions a user is invited to sign, so the UI distinguishes an ATTESTED node (one pinned
  // in the shipped config.rpcs set) from an UNATTESTED one — the user's own pasted node, or a
  // public fallback — and marks the latter plainly. An unattested node can mislead a display or
  // mis-size a deposit (recoverable) but cannot steal: recipients are config-pinned and every
  // contract re-validates on-chain. The community node list comes first in RPCS, so landing on a
  // public endpoint means every configured community node refused (still syncing, or down).
  function paintNote(url) {
    var n = $('rpcNote'); if (!n) return;
    var ci = (CFG.rpcs || []).indexOf(url), note, attested = true;
    if (url === ownStored()) { note = ' (your node - UNATTESTED; reads trust this node only)'; attested = false; }
    else if (ci >= 0) { note = ' (attested community node #' + (ci + 1) + ')'; }
    else if (PUB.indexOf(url) >= 0) { note = ' (public Neo N3 node)'; attested = false; }
    else if ((CFG.rpcs || []).length) { note = ' (UNATTESTED public fallback - every community node is unreachable or still syncing)'; attested = false; }
    else { note = NET === 'MainNet' ? ' (UNATTESTED public mainnet endpoint)' : ' (UNATTESTED public testnet endpoint)'; attested = false; }
    n.textContent = 'rpc: ' + url + note;
    try { n.dataset.attested = attested ? '1' : '0'; } catch (e) {} // lets the UI style unattested reads
    reflectPick(url);
  }
  // keep the dropdown honest: with no explicit user choice, show the node actually
  // served (normally the FTB node; a public fallback only if ours is unreachable).
  function reflectPick(url) {
    var sel = $('rpcPick'); if (!sel) return;
    var mp = null; try { mp = localStorage.getItem('ftb_rpc_pick'); } catch (e) {}
    if (mp === '__auto__') return;                 // user opted into auto - leave it
    if (mp && RPCS.indexOf(mp) >= 0) return;       // user pinned a node - leave it
    for (var i = 0; i < sel.options.length; i++) { if (sel.options[i].value === url) { sel.value = url; return; } }
  }
  function ownStored() { try { return localStorage.getItem('ftb_rpc'); } catch (e) { return null; } }

  // the default serves the attested primary (RPCS[0] - the FTB node when config.rpcs
  // is set); a stored per-node choice overrides it, and '__auto__' opts into
  // fastest-wins. primaryPick falls back to autoPick if our node is unreachable.
  function pick() {
    var manual = null; try { manual = localStorage.getItem('ftb_rpc_pick'); } catch (e) {}
    if (manual === '__auto__') return autoPick();
    if (manual && RPCS.indexOf(manual) >= 0) {
      return call(manual, 'getblockcount').then(function (h) {
        if (+h > 0) { RPC = manual; paintNote(RPC); return RPC; }
        return autoPick();
      }).catch(function () { return autoPick(); });
    }
    return primaryPick();
  }
  function primaryPick() {
    var p = RPCS[0]; if (!p) return autoPick();
    return call(p, 'getblockcount').then(function (h) {
      if (+h > 0) { RPC = p; paintNote(RPC); return RPC; }
      return autoPick();
    }).catch(function () { return autoPick(); });
  }
  function autoPick() {
    return Promise.all(RPCS.map(function (url) {
      return call(url, 'getblockcount')
        .then(function (h) { return { url: url, h: +h || 0 }; })
        .catch(function () { return { url: url, h: 0 }; });
    })).then(function (rs) {
      var tip = rs.reduce(function (m, r) { return Math.max(m, r.h); }, 0);
      for (var i = 0; i < rs.length; i++) {
        if (rs[i].h > 0 && rs[i].h >= tip - SYNC_SLACK) { RPC = rs[i].url; paintNote(RPC); return RPC; }
      }
      var e = new Error('no healthy RPC node answered');
      var n = $('rpcNote'); if (n) n.textContent = 'rpc: ' + e.message + ' - paste your own node';
      throw e;
    });
  }

  function read(hash, op, args, _retried) {
    return call(RPC, 'invokefunction', [hash, op, args || []]).then(function (r) {
      if (r.state !== 'HALT') throw new Error(r.exception || 'invocation faulted');
      return r.stack;
    }).catch(function (e) {
      // the picked endpoint can die mid-session (node restart, rate limit, sync
      // stall) - signing/broadcast goes through the wallet's own RPC, so writes
      // land while reads show "failed to fetch". re-probe every endpoint once and
      // retry the read on the new winner; a genuinely faulting contract just
      // faults again on the retry, unchanged.
      if (_retried) throw e;
      if (!repicking) repicking = pick().then(function () {}, function () {}).then(function () { repicking = null; });
      return repicking.then(function () { return read(hash, op, args, true); });
    });
  }
  var repicking = null;
  function readInt(hash, op, args) {
    return read(hash, op, args).then(function (st) {
      return (st && st[0] && st[0].value != null) ? BigInt(st[0].value) : 0n;
    });
  }
  function readBool(hash, op, args) {
    return read(hash, op, args).then(function (st) {
      var it = st && st[0]; if (!it) return false;
      if (it.type === 'Boolean') return !!it.value;
      try { return BigInt(it.value) !== 0n; } catch (e) { return false; }
    });
  }

  // ---------- own node modal ----------
  // a user on their own node cannot be censored; the choice is stored only in
  // this browser. validated with getblockcount before it is allowed to serve.
  function wire() {
    var ov = $('ovRpc'); if (!ov) return;              // no own-node modal on this page
    // The top-right button was retired; the rpc status note is the opener now, so the
    // 'use your own node' control sits next to the node it would replace.
    var opener = $('ownRpcBtn') || $('rpcNote');
    if (opener) {
      opener.addEventListener('click', function () { ov.hidden = false; });
      if (opener.id === 'rpcNote') {
        opener.classList.add('is-clickable');
        opener.title = 'use your own node';
        opener.setAttribute('role', 'button');
        opener.setAttribute('tabindex', '0');
        opener.addEventListener('keydown', function (e) {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); ov.hidden = false; }
        });
      }
    }
    var go = $('ownRpcGo');
    if (go) go.addEventListener('click', function () {
      var url = $('ownRpcIn').value.trim(); if (!url) return;
      call(url, 'getblockcount').then(function (h) {
        if (!(h > 0)) throw new Error('not a healthy node');
        try { localStorage.setItem('ftb_rpc', url); } catch (e) {}
        RPC = url; paintNote(url);
        ov.hidden = true;
        if (window.FtbState) window.FtbState.emit('rpc', url);
      }).catch(function (e) {
        var m = $('ownRpcMsg'); m.replaceChildren();
        var d = document.createElement('div'); d.className = 'err'; d.textContent = 'unusable: ' + e.message;
        m.appendChild(d);
      });
    });
    var rst = $('ownRpcReset');
    if (rst) rst.addEventListener('click', function () {
      try { localStorage.removeItem('ftb_rpc'); } catch (e) {}
      location.reload();
    });
  }
  // ---------- rpc picker: a dropdown to switch among the known-good nodes ----------
  function hostShort(u) { try { return new URL(u).hostname; } catch (e) { return u; } }
  function buildPicker() {
    var note = $('rpcNote'); if (!note || $('rpcPick')) return;
    var sel = document.createElement('select');
    sel.id = 'rpcPick'; sel.className = 'rpcpick';
    sel.title = 'switch RPC node'; sel.setAttribute('aria-label', 'switch RPC node');
    function opt(v, label) { var o = document.createElement('option'); o.value = v; o.textContent = label; return o; }
    var attested = CFG.rpcs || [];
    function label(u) { return attested.indexOf(u) >= 0 ? hostShort(u) + ' (FTB node)' : hostShort(u); }
    RPCS.forEach(function (u) { sel.appendChild(opt(u, label(u))); });   // FTB node first = default
    sel.appendChild(opt('__auto__', 'auto (fastest)'));
    if ($('ovRpc')) sel.appendChild(opt('__own__', 'your own node...'));
    // default selection is the attested FTB node (RPCS[0]); a stored choice wins.
    try {
      var cur = localStorage.getItem('ftb_rpc_pick') || '';
      var valid = cur === '__auto__' || (cur === '__own__' && $('ovRpc')) || RPCS.indexOf(cur) >= 0;
      sel.value = valid ? cur : (RPCS[0] || '');
    } catch (e) { sel.value = RPCS[0] || ''; }
    sel.addEventListener('change', function () {
      var v = sel.value;
      if (v === '__own__') { var ov = $('ovRpc'); if (ov) ov.hidden = false; try { sel.value = localStorage.getItem('ftb_rpc_pick') || (RPCS[0] || ''); } catch (e) {} return; }
      if (v === '__auto__') { try { localStorage.setItem('ftb_rpc_pick', '__auto__'); } catch (e) {} autoPick().then(function () { if (window.FtbState) window.FtbState.emit('rpc', RPC); }, function () {}); return; }
      call(v, 'getblockcount').then(function (h) {
        if (!(+h > 0)) throw new Error('unhealthy');
        RPC = v; try { localStorage.setItem('ftb_rpc_pick', v); } catch (e) {} paintNote(v);
        if (window.FtbState) window.FtbState.emit('rpc', v);
      }).catch(function () { try { sel.value = localStorage.getItem('ftb_rpc_pick') || (RPCS[0] || ''); } catch (e) {} });
    });
    note.parentNode.insertBefore(sel, note.nextSibling);
  }
  wire();
  buildPicker();

  window.FtbRpc = {
    pick: pick,
    call: function (method, params) { return call(RPC, method, params); },
    read: read,
    readInt: readInt,
    readBool: readBool,
    current: function () { return RPC; },
    endpoints: RPCS
  };
})();
