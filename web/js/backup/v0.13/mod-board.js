// mod-board.js - module 6: proposals (MODULES.md). v0.11 - typed create ticket.
// The bonded proposals board (registry v2): every accepted proposal opens AS a
// ballot, at most 3 active at once, and the bond rises with the queue behind that
// cap. The create ticket is a CSS-only ballot|listing toggle (no JS state); this
// module reads the checked type at submit time.
//
// Owns (writes only these): #board card - #boardBody, #boardRefresh, #pGo,
// #pMetaHint. Reads the create inputs (#ptBallot/#ptListing, #pqBallot, the
// option fields, #plToken) at submit time.
(function () {
  'use strict';
  if (!FtbUtil.$('boardBody')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, short = FtbUtil.short, errText = FtbUtil.errText;
  var C = FtbState.C;

  function boardReady() { return !!(FtbState.configured && C.registry && C.registry.indexOf('0x0000') !== 0); }
  function b64ToStr(s) { try { return decodeURIComponent(escape(atob(s))); } catch (e) { return atob(s); } }
  function hashFromItem(it) {
    return (it && it.value) ? '0x' + FtbDapi.util.reverseHex(FtbDapi.util.b64ToHex(it.value)) : '-';
  }
  function kind() { var b = $('ptBallot'); return (b && b.checked) ? 'ballot' : 'listing'; }

  function refreshBoard() {
    var body = $('boardBody'); if (!body) return;
    if (!FtbRpc.current() || !boardReady()) return; // the empty/standby markup stays in place
    // the authoritative bond is read live into the review/sign plan at submit time
    // (Art 11); the ticket states only the rule. #pMetaHint carries validation only.
    FtbRpc.readInt(C.registry, 'proposalCount').then(function (n) {
      body.innerHTML = '';
      var count = Number(n);
      if (!count) {
        var e = el('div', 'entry');
        e.appendChild(el('span', 'idx', '//'));
        var mid = el('div'); mid.appendChild(el('span', 'ttl', 'no proposals yet - the board is open.')); e.appendChild(mid);
        var m = el('div', 'meta'); m.appendChild(el('span', 'tag', 'open')); e.appendChild(m);
        body.appendChild(e);
        return;
      }
      for (var id = count; id >= 1; id--) { (function (pid) { // newest first
        FtbRpc.read(C.registry, 'proposal', [{ type: 'Integer', value: String(pid) }]).then(function (st) {
          var items = st[0].value;
          var k = b64ToStr(items[0].value);
          var ref = hashFromItem(items[1]);
          var meta = b64ToStr(items[2].value);
          var bond = BigInt(items[4].value || '0');
          var open = items[5].type === 'Boolean' ? items[5].value : (items[5].value && items[5].value !== '');
          var row = el('div', 'entry');
          row.appendChild(el('span', 'idx', String(pid).padStart(2, '0')));
          var mid = el('div'); mid.appendChild(el('span', 'ttl', meta)); row.appendChild(mid);
          var m = el('div', 'meta');
          m.appendChild(el('span', 'tag ' + (open ? 'pub' : 'sealed'), open ? 'open' : 'closed'));
          m.appendChild(el('span', 'tag', k));
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

  function flash(b) {
    if (!b) return;
    var t = b.textContent; b.textContent = 'reading…'; b.disabled = true;
    setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500);
  }
  var bb = $('boardRefresh'); if (bb) bb.addEventListener('click', function () { flash(bb); refreshBoard(); });

  var pGo = $('pGo');
  if (pGo) pGo.addEventListener('click', function () {
    var k = kind();
    if (!boardReady()) {
      FtbShell.showPlan('board not wired', 'the registry hash is a placeholder in this deployment. proposals go live with the TestNet registry contract.',
        [['type', k], ['status', 'registry not configured']], null);
      return;
    }
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    // a LISTING references the project's token hash; the registry accepts it directly.
    // a BALLET with inline options (a..d) is ahead of the current registry - it opens
    // a ballot via the v2 factory, not yet deployed - so here it is announced, in words.
    if (k === 'ballot') {
      var q = ($('pqBallot') && $('pqBallot').value.trim()) || '';
      var opts = Array.prototype.map.call(document.querySelectorAll('.pf-ballot .pfopt input'), function (i) { return i.value.trim(); }).filter(Boolean);
      if (!q) { var h = $('pMetaHint'); if (h) h.textContent = 'write the question first'; return; }
      if (opts.length < 1) { var h2 = $('pMetaHint'); if (h2) h2.textContent = 'give at least one option'; return; }
      FtbShell.showPlan('open ballot', 'inline-option ballots open through the v2 ballot factory (one contract per proposal, the hash becomes the proposal id). the factory is not yet wired on this network; the question and options are ready to submit once it is.',
        [['question', q], ['options', opts.join(' · ')], ['status', 'ballot factory pending']], null);
      return;
    }
    var ref = ($('plToken') && $('plToken').value.trim()) || '';
    if (!/^0x[0-9a-fA-F]{40}$/.test(ref)) { var h3 = $('pMetaHint'); if (h3) h3.textContent = 'token contract must be a 0x hash (20 bytes)'; return; }
    FtbRpc.readInt(C.registry, 'proposalBond').then(function (bond) {
      if (bond <= 0n) throw new Error('proposal bond not configured on-chain');
      var meta = 'listing: ' + ref;
      var plan = FtbState.dapi.submitProposal(FtbState.account, 'listing', ref, meta, bond.toString());
      FtbShell.showPlan('submit listing proposal',
        'the bond is escrowed by the registry; on approval it seeds the market 50% to liquidity and 50% to a rug-pull insurance reserve, and is refunded if the listing is rejected.',
        [['type', 'listing'], ['token', ref], ['bond', fmt(bond) + ' FTB'], ['what you sign', plan.summary]], plan);
    }).catch(function (e) { var h4 = $('pMetaHint'); if (h4) h4.textContent = 'bond read failed: ' + errText(e); });
  });

  FtbState.on('rpc', refreshBoard);
  FtbState.on('refresh', refreshBoard);
})();
