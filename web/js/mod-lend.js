// mod-lend.js — module 4: LEND (MODULES.md).
// NEO+GAS collateral, FTB loans. This page is read-only for the pool; every
// position-changing action is a review-and-sign plan through the adapter.
//
// Owns (writes only these): #lend card — #lUtil, #lBApy, #lSApy, #lBorrows,
// #lReserve, #lendPos (#pHealth, #pNeo, #pGas, #pDebt, #pMax, #pShares),
// #lendOff, #lendRefresh.
(function () {
  'use strict';
  if (!FtbUtil.$('lend')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, fmt = FtbUtil.fmt;
  var C = FtbState.C;

  function bps(v) { return (Number(v) / 100).toFixed(2) + '%'; }
  function readVal(h, op, args) {
    return FtbRpc.read(h, op, args).then(function (st) {
      var it = st && st[0];
      return it ? it.value : null;
    });
  }

  function loadLend() {
    var lend = C.lend;
    if (!lend || lend.indexOf('0x0000') === 0 || !FtbRpc.current()) { var off0 = $('lendOff'); if (off0) off0.hidden = false; return; }
    var off = $('lendOff'); if (off) off.hidden = true;
    FtbRpc.readInt(lend, 'utilizationBps').then(function (v) { $('lUtil').textContent = bps(v); }).catch(function () {});
    FtbRpc.readInt(lend, 'borrowApyBps').then(function (v) { $('lBApy').textContent = bps(v); }).catch(function () {});
    FtbRpc.readInt(lend, 'supplyApyBps').then(function (v) { $('lSApy').textContent = bps(v); }).catch(function () {});
    FtbRpc.readInt(lend, 'totalBorrows').then(function (v) { $('lBorrows').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(lend, 'reserveAccrued').then(function (v) { $('lReserve').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    var acct = FtbState.acctHash160();
    if (!acct) return;
    var pos = $('lendPos'); if (pos) pos.hidden = false;
    var hu = [{ type: 'Hash160', value: acct }];
    FtbRpc.readInt(lend, 'collateralOf', hu.concat([{ type: 'Boolean', value: true }])).then(function (v) { $('pNeo').textContent = v.toString() + ' NEO'; }).catch(function () {});
    FtbRpc.readInt(lend, 'collateralOf', hu.concat([{ type: 'Boolean', value: false }])).then(function (v) { $('pGas').textContent = fmt(v) + ' GAS'; }).catch(function () {});
    FtbRpc.readInt(lend, 'debtOf', hu).then(function (v) { $('pDebt').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(lend, 'maxBorrow', hu).then(function (v) { $('pMax').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(lend, 'sharesOf', hu).then(function (v) { $('pShares').textContent = v.toString(); }).catch(function () {});
    readVal(lend, 'isUnhealthy', hu).then(function (v) {
      var e = $('pHealth'); if (!e) return;
      var bad = (v === true || v === 'true' || v === '1');
      e.textContent = bad ? 'LIQUIDATABLE' : 'healthy';
    }).catch(function () {});
  }

  var lb = $('lendRefresh'); if (lb) lb.addEventListener('click', loadLend);
  FtbState.on('rpc', loadLend);
  FtbState.on('account', loadLend);
  FtbState.on('refresh', loadLend);
  // keep the pool fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadLend(); }, 60000);
})();
