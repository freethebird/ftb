// mod-token.js - module 2: token on FTB (MODULES.md).
// Stat tiles for the token itself plus the connected wallet's balance strip.
//
// Owns (writes only these): tiles #tSupply, #tCap, #tBlock, and the balance
// strip #balStrip (#balFtb, #balNeo, #balGas, #balClaim). Reads overlap freely
// (totalSupply / maxSupply are also read by mod-claim and mod-verify - each
// module answers to its own corner of the page).
(function () {
  'use strict';
  if (!FtbUtil.$('tSupply')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, fmt = FtbUtil.fmt;
  var CFG = FtbState.CFG, C = FtbState.C;

  function refreshTiles() {
    if (!FtbRpc.current()) return;
    FtbRpc.call('getblockcount').then(function (h) {
      $('tBlock').textContent = Number(h).toLocaleString('en-US');
    }).catch(function () {});
    if (!FtbState.configured) return;
    FtbRpc.readInt(C.ftb, 'totalSupply').then(function (v) { FtbUtil.tileNum($('tSupply'), v); }).catch(function () {});
    FtbRpc.readInt(C.ftb, 'maxSupply').then(function (v) { FtbUtil.tileNum($('tCap'), v); }).catch(function () {});
  }

  // ---------- your wallet strip (once connected) ----------
  function refreshBalances() {
    var account = FtbState.account;
    if (!account || !FtbState.configured || !FtbRpc.current()) return;
    $('balStrip').hidden = false;
    var h = [{ type: 'Hash160', value: FtbState.acctHash160() }];
    FtbRpc.readInt(C.ftb, 'balanceOf', h).then(function (v) { $('balFtb').textContent = fmt(v); }).catch(function () {});
    var neo = (CFG.assets || {}).neo;
    if (neo) FtbRpc.readInt(neo, 'balanceOf', h).then(function (v) { $('balNeo').textContent = FtbUtil.grp(v.toString()); }).catch(function () {}); // NEO: whole units, no scaling
    var gas = (CFG.assets || {}).gas;
    if (gas) FtbRpc.readInt(gas, 'balanceOf', h).then(function (v) { $('balGas').textContent = fmt(v); }).catch(function () {});
    FtbState.proofs().then(function () {
      var en = FtbState.entryFor(account);
      if (!en) { $('balClaim').textContent = 'not in snapshot'; return; }
      FtbRpc.read(C.migrator, 'claimed', h).then(function (st) {
        var it = st && st[0], c = it && (it.type === 'Boolean' ? it.value : (it.type === 'Integer' ? parseInt(it.value || '0', 10) > 0 : (it.value && it.value !== '')));
        if (c) { $('balClaim').textContent = 'claimed ✓'; return; }
        FtbState.quoteFtb(en.amount).then(function (q) { $('balClaim').textContent = fmt(q) + ' FTB'; });
      }).catch(function () {});
    }).catch(function () {});
  }

  FtbState.on('rpc', refreshTiles);
  FtbState.on('rpc', refreshBalances);
  FtbState.on('account', refreshBalances);
  FtbState.on('refresh', function () { refreshTiles(); refreshBalances(); });
})();
