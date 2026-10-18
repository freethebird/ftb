// mod-board.js — module 6: proposals (MODULES.md).
// The bonded proposals board (registry v2): the bond is escrowed on-chain,
// refunded on a legitimate close, kept against spam.
//
// Owns (writes only these): #board card — #boardBody, #boardRefresh, and the
// submit ticket #pMeta, #pKindSel, #pRef, #pMetaHint, #pGo.
(function () {
  'use strict';
  if (!FtbUtil.$('boardBody')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, short = FtbUtil.short, errText = FtbUtil.errText;
  var C = FtbState.C;
  var pKind = 'ballot', boardBond = null;

  function boardReady() { return !!(FtbState.configured && C.registry && C.registry.indexOf('0x0000') !== 0); }
  function b64ToStr(s) { try { return decodeURIComponent(escape(atob(s))); } catch (e) { return atob(s); } }
  function hashFromItem(it) {
    return (it && it.value) ? '0x' + FtbDapi.util.reverseHex(FtbDapi.util.b64ToHex(it.value)) : '—';
  }
  function paintKindSel() {
    var box = $('pKindSel'); box.innerHTML = '';
    ['ballot', 'listing'].forEach(function (k) {
      var b = el('button', k === pKind ? 'on' : null, k);
      b.addEventListener('click', function () { pKind = k; paintKindSel(); });
      box.appendChild(b);
    });
  }
  function refreshBoard() {
    var body = $('boardBody'); if (!body) return;
    if (!FtbRpc.current() || !boardReady()) return; // standby markup already in place
    FtbRpc.readInt(C.registry, 'proposalBond').then(function (v) {
      boardBond = v;
      $('pMetaHint').textContent = 'bond: ' + fmt(v) + ' FTB · exact, escrowed — refunded on a legitimate close, kept against spam';
    }).catch(function () {});
    FtbRpc.readInt(C.registry, 'proposalCount').then(function (n) {
      body.innerHTML = '';
      var count = Number(n);
      if (!count) {
        body.appendChild(el('div', 'small muted', 'no proposals yet — the board is open; the first one costs the bond.'));
        return;
      }
      for (var id = count; id >= 1; id--) { (function (pid) { // newest first
        FtbRpc.read(C.registry, 'proposal', [{ type: 'Integer', value: String(pid) }]).then(function (st) {
          var items = st[0].value;
          var kind = b64ToStr(items[0].value);
          var ref = hashFromItem(items[1]);
          var meta = b64ToStr(items[2].value);
          var bond = BigInt(items[4].value || '0');
          var open = items[5].type === 'Boolean' ? items[5].value : (items[5].value && items[5].value !== '');
          var row = el('div', 'entry');
          row.appendChild(el('span', 'idx', String(pid).padStart(2, '0')));
          var mid = el('div'); mid.appendChild(el('span', 'ttl', meta)); row.appendChild(mid);
          var m = el('div', 'meta');
          m.appendChild(el('span', 'tag ' + (open ? 'pub' : 'sealed'), open ? 'open' : 'closed'));
          m.appendChild(el('span', 'tag', kind));
          m.appendChild(el('span', null, short(ref)));
          m.appendChild(el('span', null, 'bond ' + fmt(bond) + ' FTB'));
          row.appendChild(m);
          body.appendChild(row);
        }).catch(function () {});
      })(id); }
    }).catch(function (e) {
      body.innerHTML = ''; body.appendChild(el('div', 'small', 'board unavailable: ' + errText(e)));
    });
  }

  $('boardRefresh').addEventListener('click', refreshBoard);
  $('pGo').addEventListener('click', function () {
    if (!boardReady()) {
      FtbShell.showPlan('board not wired', 'the registry hash is a placeholder in this deployment. proposals go live with the TestNet contracts.',
        [['status', 'registry not configured']], null);
      return;
    }
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    var meta = $('pMeta').value.trim(), ref = $('pRef').value.trim();
    if (!meta) { $('pMetaHint').textContent = 'write the proposal first'; return; }
    if (!/^0x[0-9a-fA-F]{40}$/.test(ref)) { $('pMetaHint').textContent = 'reference must be a 0x contract hash (20 bytes)'; return; }
    FtbRpc.readInt(C.registry, 'proposalBond').then(function (bond) {
      if (bond <= 0n) throw new Error('proposal bond not configured on-chain');
      var plan = FtbState.dapi.submitProposal(FtbState.account, pKind, ref, meta, bond.toString());
      FtbShell.showPlan('submit ' + pKind + ' proposal',
        'the bond is escrowed by the registry. a legitimate close refunds it to you; spam forfeits it to the committee. the record stays on-chain either way.',
        [['kind', pKind],
         ['reference', ref],
         ['meta', meta.length > 80 ? meta.slice(0, 77) + '…' : meta],
         ['bond', fmt(bond) + ' FTB'],
         ['what you sign', plan.summary]],
        plan);
    }).catch(function (e) { $('pMetaHint').textContent = 'bond read failed: ' + errText(e); });
  });
  paintKindSel();

  FtbState.on('rpc', refreshBoard);
  FtbState.on('refresh', refreshBoard);
})();
