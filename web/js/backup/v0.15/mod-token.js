// mod-token.js - module 2: token on FTB (MODULES.md).
// Stat tiles for the token itself plus the connected wallet's balance strip.
//
// Owns (writes only these): tiles #tSupply, #tCap, #tBlock, and the balance
// strip #balStrip (#balFtb, #balNeo, #balGas). Reads overlap freely
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
  // Three green boxes: FTB / NEO / GAS. Claimable state is deliberately NOT
  // here - claiming is a one-time event, the claim module owns it. Clicking a
  // box is wired by mod-swap (it owns the ticket the click fills).
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
  }

  // coalesced like every other module (canon II.4): the shell bursts 'refresh'
  // around each landed tx; one pass in flight, one trailing run, 3 s apart.
  var tilesC = FtbUtil.coalesce(refreshTiles, 3000), balsC = FtbUtil.coalesce(refreshBalances, 3000);
  FtbState.on('rpc', tilesC);
  FtbState.on('rpc', balsC);
  FtbState.on('account', balsC);
  FtbState.on('refresh', function () { tilesC(); balsC(); });
})();
