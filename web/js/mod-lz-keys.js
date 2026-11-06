// mod-lz-keys.js -- FTB key-transparency panel (librationzero.html).
// Reads each contract's state live from the chain: getcontractstate confirms it is deployed,
// then the owner storage slot shows "Keyless" (ownership renounced on-chain -> immutable) or
// "Key active" (an owner can still call privileged methods). Contracts listed in CFG.sealed are
// deployed but held SEALED -- their real hash is shown and verified on-chain, but CFG.contracts
// stays dark so the app does not wire them until the governance milestone (TestNet soak +
// renounce). Each sealed row carries a countdown to the published unseal target
// (CFG.governance.unsealAt); the on-chain renounce is the real event it names. Read-only:
// getcontractstate / getstorage, no wallet, no writes. Canon: fail-closed -- a read that errors
// leaves the row's standby, never a fabricated status. DOM via textContent + createElement only.
(function () {
  'use strict';
  if (!window.FtbRpc || !window.FTB_CONFIG) return;
  var C = window.FTB_CONFIG.contracts || {};
  var SEALED = window.FTB_CONFIG.sealed || {};
  var GOV = window.FTB_CONFIG.governance || {};

  function zero(h) { return !h || /^0x0+$/.test(h); }
  function keyB64(hexByte) { return btoa(String.fromCharCode(parseInt(hexByte, 16))); }
  function decodeVal(b64) {
    if (b64 == null || b64 === '') return { len: 0, be: '' };
    var s = atob(b64), le = [];
    for (var i = 0; i < s.length; i++) { var h = s.charCodeAt(i).toString(16); le.push((h.length < 2 ? '0' : '') + h); }
    return { len: s.length, be: le.reverse().join('') };
  }

  function setState(row, state, beHex) {
    var b = row.querySelector('.lz-kstat'); if (!b) return;
    b.classList.remove('is-keyless', 'is-active', 'is-pending', 'is-sealed');
    if (state === 'keyless') { b.textContent = 'Keyless'; b.classList.add('is-keyless'); b.removeAttribute('title'); }
    else if (state === 'active') { b.textContent = 'Key active'; b.classList.add('is-active'); if (beHex) b.title = 'owner 0x' + beHex; }
    else if (state === 'sealed') { b.textContent = 'Sealed'; b.classList.add('is-sealed'); if (beHex) b.title = 'owner 0x' + beHex; }
    else if (state === 'pending') { b.textContent = 'pending deploy'; b.classList.add('is-pending'); b.removeAttribute('title'); }
  }

  function fmtLeft(ms) {
    if (ms <= 0) return 'unseal window open';
    var m = Math.floor(ms / 60000);
    var d = Math.floor(m / 1440); m -= d * 1440;
    var h = Math.floor(m / 60); m -= h * 60;
    return 'keyless in ' + (d ? d + 'd ' : '') + h + 'h ' + m + 'm';
  }
  function countdown(row) {
    var st = row.querySelector('.lz-kstat'); if (!st || !st.parentNode) return;
    var el = st.parentNode.querySelector('.lz-kcount');
    if (!el) { el = document.createElement('span'); el.className = 'lz-kcount'; st.parentNode.appendChild(el); }
    var t = GOV.unsealAt ? Date.parse(GOV.unsealAt) : NaN;
    if (isNaN(t)) { el.textContent = 'sealed'; return; }
    function tick() { el.textContent = fmtLeft(t - Date.now()); }
    tick(); setInterval(tick, 60000);
  }

  function readKey(row, hash, kb, sealed) {
    function finish(d) {
      var keyless = (d.len === 0 || /^0+$/.test(d.be));
      setState(row, keyless ? 'keyless' : (sealed ? 'sealed' : 'active'), d.be);
    }
    FtbRpc.call('getcontractstate', [hash]).then(function (state) {
      if (!state || !state.hash) { setState(row, sealed ? 'sealed' : 'pending'); return; }
      if (kb.indexOf('call:') === 0) {
        FtbRpc.read(hash, kb.slice(5), []).then(function (stack) {
          var it = stack && stack[0];
          finish((it && it.value != null) ? decodeVal(it.value) : { len: 0, be: '' });
        }).catch(function () { });
        return;
      }
      FtbRpc.call('getstorage', [hash, keyB64(kb)]).then(function (res) { finish(decodeVal(res)); }).catch(function () { });
    }).catch(function () { /* fail-closed: standby stays */ });
  }

  function run() {
    var rows = Array.prototype.slice.call(document.querySelectorAll('#lzKeys tr[data-c]'));
    rows.forEach(function (row) {
      var dc = row.getAttribute('data-c');
      var sh = SEALED[dc];
      var sealed = !!(sh && !zero(sh));
      var hash = sealed ? sh : C[dc];
      var a = row.querySelector('.lz-kaddr');
      if (zero(hash)) { setState(row, 'pending'); if (a) { a.removeAttribute('href'); } return; }
      if (a) { a.textContent = hash; a.href = 'explorer.html?q=' + encodeURIComponent(hash); }
      if (sealed) { setState(row, 'sealed'); countdown(row); }
      else if (row.getAttribute('data-key') === 'none') { setState(row, 'keyless'); }
    });
    FtbRpc.pick().then(function () {
      rows.forEach(function (row) {
        var dc = row.getAttribute('data-c');
        var sh = SEALED[dc];
        var sealed = !!(sh && !zero(sh));
        var hash = sealed ? sh : C[dc];
        var kb = row.getAttribute('data-key');
        if (zero(hash) || kb === 'none') return;
        readKey(row, hash, kb, sealed);
      });
    }).catch(function () { });
  }

  if (document.readyState !== 'loading') { run(); }
  else { document.addEventListener('DOMContentLoaded', run); }
})();
