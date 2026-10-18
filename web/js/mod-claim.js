// mod-claim.js — module 7: claim (MODULES.md).
// The migration card: status reads, the snapshot check, and the claim plan.
// One claim per address, verified on-chain against the published merkle root.
//
// Owns (writes only these): #claim card — #migState, #supplyLine, #capLine,
// #refreshBtn, #checkBtn, #claimGo, #proofsBtn, #rootLine, #rootCount, #claimHint.
(function () {
  'use strict';
  if (!FtbUtil.$('claim')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, fmt = FtbUtil.fmt, short = FtbUtil.short, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;
  var MERKLE_ROOT = CFG.merkleRoot || '';

  // ---------- merkle row: root from config, holder count from the tree itself ----------
  // (WEB-03: the count is derived from proofs.json, never a hand-typed literal.)
  $('rootLine').textContent = MERKLE_ROOT ? short(MERKLE_ROOT) : '—';
  FtbState.proofs().then(function (j) {
    $('rootCount').textContent = j.entries.length.toLocaleString('en-US') + ' holders';
  }).catch(function () { $('rootCount').textContent = 'holders —'; });

  // ---------- migration status reads ----------
  function refresh() {
    if (!FtbRpc.current() || !FtbState.configured) return;
    FtbRpc.readBool(C.ftb, 'migrationOpen').then(function (open) {
      var t = $('migState');
      t.textContent = open ? 'migration open' : 'mint sealed';
      t.className = open ? 'tag pub' : 'tag sealed';
    }).catch(function () {});
    FtbRpc.readInt(C.ftb, 'totalSupply').then(function (v) {
      $('supplyLine').textContent = 'supply ' + fmt(v) + ' FTB';
    }).catch(function () {});
    FtbRpc.readInt(C.ftb, 'maxSupply').then(function (v) {
      $('capLine').textContent = 'cap ' + fmt(v) + ' FTB';
    }).catch(function () {});
  }

  // ---------- check + claim (the big "claim your ftb" key runs the same flow) ----------
  function checkAndClaim() {
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    FtbState.proofs().then(function () {
      var en = FtbState.entryFor(FtbState.account);
      if (!en) {
        FtbShell.showPlan('no claim', 'this address is not in the published snapshot. only pre-exploit holders can claim; the attacker and contracts are excluded by construction.',
          [['address', FtbState.account]], null);
        return;
      }
      var plan = FtbState.dapi.claim(FtbState.account, en.amount, en.proof);
      FtbState.quoteFtb(en.amount).then(function (q) {
        FtbShell.showPlan('claim ' + fmt(q) + ' FTB',
          'one claim per address, verified on-chain against the published merkle root. the migrator converts at the committed r = 0.189832 (1 FLM/FLOCKS = 0.189832 FTB). check the hashes in section IV before you sign.',
          [['address', en.address],
           ['entitlement', fmt(en.amount) + ' FLM-equiv'],
           ['you receive', fmt(q) + ' FTB'],
           ['proof', en.proof.length + ' nodes'],
           ['what you sign', plan.summary]],
          plan);
      });
    }).catch(function (e) {
      FtbShell.showPlan('proofs unavailable', errText(e), [['proofs file', FtbState.PROOFS_URL]], null);
    });
  }
  $('checkBtn').addEventListener('click', checkAndClaim);
  if ($('claimGo')) $('claimGo').addEventListener('click', checkAndClaim);
  $('proofsBtn').addEventListener('click', function () { window.open(FtbState.PROOFS_URL, '_blank'); });
  $('refreshBtn').addEventListener('click', refresh);

  FtbState.on('rpc', refresh);
  FtbState.on('refresh', refresh);
})();
