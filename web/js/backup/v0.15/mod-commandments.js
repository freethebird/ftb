// mod-commandments.js - the on-chain charter on the governance page (governance.html).
// FtbCommandments is immutable once inked and the owner renounced: these are the ratified
// commandments, read straight from contract storage. Read-only; no wallet, no writes.
// Canon: fail-closed - a read that errors leaves the standby, never a fabricated charter.
//
// Owns (writes only these): #cmdList, #cmdOff, #cmdCount.
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('cmdList')) return;
  if (!window.FtbState || !window.FtbRpc) return;

  var $ = FtbUtil.$, el = FtbUtil.el;
  var C = FtbState.C;

  function ready() { return !!(C.commandments && C.commandments.indexOf('0x0000') !== 0 && FtbRpc.current()); }
  function iarg(n) { return { type: 'Integer', value: String(n) }; }
  function b64utf8(b64) {
    try { if (b64 == null || b64 === '') return ''; var bin = atob(b64); try { return decodeURIComponent(escape(bin)); } catch (e) { return bin; } } catch (e) { return ''; }
  }

  function load() {
    var off = $('cmdOff'), list = $('cmdList');
    if (!ready()) { if (off) off.hidden = false; return; }
    if (off) off.hidden = true;
    FtbRpc.readInt(C.commandments, 'count', []).then(function (cntB) {
      var n = Number(cntB), ids = [];
      for (var i = 0; i < n && i < 100; i++) ids.push(i);
      var cc = $('cmdCount'); if (cc) cc.textContent = n ? (n + ' inked') : 'none yet';
      return Promise.all(ids.map(function (i) {
        return FtbRpc.read(C.commandments, 'commandment', [iarg(i)])
          .then(function (st) { return (st && st[0]) ? b64utf8(st[0].value) : ''; })
          .catch(function () { return null; });
      })).then(function (texts) {
        if (!list) return;
        list.replaceChildren();
        if (!n) { list.appendChild(el('div', 'small muted', 'no commandments inked on this network yet.')); return; }
        texts.forEach(function (t, i) {
          if (t === null) return; // read failed: skip, never fabricate
          var row = el('div', 'cmdrow');
          row.appendChild(el('span', 'cmdn', String(i + 1)));
          row.appendChild(el('span', 'cmdt', t || '(empty)'));
          list.appendChild(row);
        });
      });
    }).catch(function () { if (off) off.hidden = false; });
  }

  var loadC = FtbUtil.coalesce(load, 3000);
  FtbState.on('rpc', loadC);
  FtbState.on('refresh', loadC);
  if (FtbRpc.current()) loadC();
})();
