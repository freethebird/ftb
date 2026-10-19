// ftb-rpc.js — the ONE JSON-RPC read stack (MODULES.md R3; closes WEB-01).
// Every module reads the chain through FtbRpc — no module opens its own fetch
// to an RPC endpoint, and no module re-implements pick/failover.
//
// Owns (writes only these): #rpcNote, and the #ovRpc own-node modal
// (#ownRpcBtn, #ownRpcIn, #ownRpcGo, #ownRpcReset, #ownRpcMsg).
(function () {
  'use strict';

  var CFG = window.FTB_CONFIG || {};
  var NET = CFG.network || 'TestNet';
  var RPCS = (CFG.rpcs && CFG.rpcs.length ? CFG.rpcs.slice() : [])
    .concat(NET === 'MainNet'
      ? ['https://mainnet1.neo.coz.io:443', 'https://mainnet2.neo.coz.io:443']
      : ['https://testnet1.neo.coz.io:443', 'https://testnet2.neo.coz.io:443']);
  try { var own = localStorage.getItem('ftb_rpc'); if (own) RPCS.unshift(own); } catch (e) {}

  var RPC = null;
  // probe every endpoint's height, then serve reads from the first one at tip.
  // a node that answers but is far behind is still syncing: alive for the
  // counter, but its chain state is from the past — contracts deployed later
  // are "not found" there, so it must not serve reads until it catches up.
  var SYNC_SLACK = 1000; // blocks (~4 h at 15 s); anything further behind is syncing

  function $(id) { return document.getElementById(id); }

  function call(url, method, params) {
    return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: method, params: params || [] }) })
      .then(function (r) { return r.json(); })
      .then(function (j) { if (j.error) throw new Error(j.error.message || 'rpc error'); return j.result; });
  }

  // say WHICH source won and, for a public fallback, why that is — the community
  // node list comes first in RPCS, so landing on a public endpoint means every
  // configured community node refused (still syncing, or down).
  function paintNote(url) {
    var n = $('rpcNote'); if (!n) return;
    var ci = (CFG.rpcs || []).indexOf(url), note;
    if (url === ownStored()) note = ' (your node)';
    else if (ci >= 0) note = ' (community node #' + (ci + 1) + ')';
    else if ((CFG.rpcs || []).length) note = ' (public fallback — community node unreachable or still syncing)';
    else note = NET === 'MainNet' ? ' (public mainnet endpoint)' : ' (public testnet endpoint)';
    n.textContent = 'rpc: ' + url + note;
  }
  function ownStored() { try { return localStorage.getItem('ftb_rpc'); } catch (e) { return null; } }

  function pick() {
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
      var n = $('rpcNote'); if (n) n.textContent = 'rpc: ' + e.message + ' — paste your own node';
      throw e;
    });
  }

  function read(hash, op, args, _retried) {
    return call(RPC, 'invokefunction', [hash, op, args || []]).then(function (r) {
      if (r.state !== 'HALT') throw new Error(r.exception || 'invocation faulted');
      return r.stack;
    }).catch(function (e) {
      // the picked endpoint can die mid-session (node restart, rate limit, sync
      // stall) — signing/broadcast goes through the wallet's own RPC, so writes
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
    var btn = $('ownRpcBtn'); if (!btn) return;
    btn.addEventListener('click', function () { $('ovRpc').hidden = false; });
    $('ownRpcGo').addEventListener('click', function () {
      var url = $('ownRpcIn').value.trim(); if (!url) return;
      call(url, 'getblockcount').then(function (h) {
        if (!(h > 0)) throw new Error('not a healthy node');
        try { localStorage.setItem('ftb_rpc', url); } catch (e) {}
        RPC = url; paintNote(url);
        $('ovRpc').hidden = true;
        if (window.FtbState) window.FtbState.emit('rpc', url);
      }).catch(function (e) {
        var m = $('ownRpcMsg'); m.innerHTML = '';
        var d = document.createElement('div'); d.className = 'err'; d.textContent = 'unusable: ' + e.message;
        m.appendChild(d);
      });
    });
    $('ownRpcReset').addEventListener('click', function () {
      try { localStorage.removeItem('ftb_rpc'); } catch (e) {}
      location.reload();
    });
  }
  wire();

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
