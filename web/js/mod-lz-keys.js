// mod-lz-keys.js -- FTB DEX key-transparency panel (librationzero.html).
// For every listed contract, read its owner storage slot live from the chain and
// show "Keyless" (ownership renounced on-chain -> immutable forever) or "Key active"
// (an owner can still call privileged methods). Read-only: getstorage, no wallet, no
// writes, no signing. Canon: fail-closed -- a read that errors leaves the row's
// on-chain standby, never a fabricated status (JS Art. 8 / HTML Art. 12). The owner
// storage key per contract is pinned in the markup (data-key, verified against the
// contract source), so a wrong-slot read can never mis-report "Keyless". DOM via
// textContent only.
(function () {
  'use strict';
  if (!window.FtbRpc || !window.FTB_CONFIG) return;
  var C = window.FTB_CONFIG.contracts || {};

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
    b.classList.remove('is-keyless', 'is-active', 'is-pending');
    if (state === 'keyless') { b.textContent = 'Keyless'; b.classList.add('is-keyless'); b.removeAttribute('title'); }
    else if (state === 'active') { b.textContent = 'Key active'; b.classList.add('is-active'); if (beHex) b.title = 'owner 0x' + beHex; }
    else if (state === 'pending') { b.textContent = 'pending deploy'; b.classList.add('is-pending'); b.removeAttribute('title'); }
  }

  function run() {
    var rows = Array.prototype.slice.call(document.querySelectorAll('#lzKeys tr[data-c]'));
    rows.forEach(function (row) {
      var hash = C[row.getAttribute('data-c')];
      var a = row.querySelector('.lz-kaddr');
      if (zero(hash)) { setState(row, 'pending'); if (a) { a.removeAttribute('href'); } return; }
      if (a) { a.textContent = hash; a.href = 'explorer.html?q=' + encodeURIComponent(hash); }
      if (row.getAttribute('data-key') === 'none') { setState(row, 'keyless'); }
    });
    FtbRpc.pick().then(function () {
      rows.forEach(function (row) {
        var hash = C[row.getAttribute('data-c')], kb = row.getAttribute('data-key');
        if (zero(hash) || kb === 'none') return;
        if (kb.indexOf('call:') === 0) {
          // method-read: the AMM admin getter falls back to a hardcoded address when
          // its storage slot is empty, so a raw slot read would mis-report "keyless".
          FtbRpc.read(hash, kb.slice(5), []).then(function (stack) {
            var it = stack && stack[0];
            var d = (it && it.value != null) ? decodeVal(it.value) : { len: 0, be: '' };
            setState(row, (d.len === 0 || /^0+$/.test(d.be)) ? 'keyless' : 'active', d.be);
          }).catch(function () { /* fail-closed */ });
          return;
        }
        FtbRpc.call('getstorage', [hash, keyB64(kb)]).then(function (res) {
          var d = decodeVal(res);
          setState(row, d.len === 0 ? 'keyless' : 'active', d.be);
        }).catch(function () { /* fail-closed: on-chain standby stays */ });
      });
    }).catch(function () { /* no healthy node: standby stays */ });
  }

  if (document.readyState !== 'loading') { run(); }
  else { document.addEventListener('DOMContentLoaded', run); }
})();
