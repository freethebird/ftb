// mod-lend.js — module 4: LEND (MODULES.md).
// NEO+GAS collateral, FTB loans. This page is read-only for the pool; every
// position-changing action is a review-and-sign plan through the adapter.
//
// Owns (writes only these): #lend card — #lUtil, #lBApy, #lSApy, #lBorrows,
// #lReserve, #lendPos (#pHealth, #pNeo, #pGas, #pDebt, #pMax, #pShares),
// #lendAct (the action keys: #lnSupplyGo, #lnWdSupplyGo, #lnNeoGo, #lnWdNeoGo,
// #lnGasGo, #lnWdGasGo, #lnBorrowGo, #lnRepayGo + their inputs),
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
    var act = $('lendAct'); if (act) act.hidden = false;
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

  // ---------- action keys: pure plans, executed only by the shell's modal (R6) ----------
  function amt(id, whole) {
    var v = ($(id).value || '').trim();
    var re = whole ? /^[0-9]+$/ : /^[0-9]+(\.[0-9]+)?$/;
    if (!re.test(v) || parseFloat(v) <= 0) return null;
    return v;
  }
  function badAmt() {
    FtbShell.showPlan('bad amount', 'enter a positive number. NEO is whole units only (0 decimals); FTB and GAS take 8.', [], null);
  }
  function needAcct() {
    if (!FtbState.account) { FtbWallet.needConnect(); return null; }
    return FtbState.account;
  }
  function wire(id, fn) { var b = $(id); if (b) b.addEventListener('click', fn); }
  var toBase = FtbDapi.util.toBaseUnits;

  wire('lnSupplyGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnSupplyAmt'); if (a === null) return badAmt();
    FtbShell.showPlan('supply ' + a + ' FTB',
      'you transfer FTB to the lend pool and receive cToken shares; interest accrues to the share price. suppliers earn borrow × utilization × 2/3.',
      [['amount', a + ' FTB'], ['what you sign', 'ftb.transfer(you -> lend, ' + a + ', "supply")']],
      FtbState.dapi.supply(u, toBase(a)));
  });
  wire('lnWdSupplyGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnWdShares', true); if (a === null) return badAmt();
    FtbShell.showPlan('withdraw ' + a + ' supply shares',
      'burns cToken shares and returns FTB at the current share price. only the pool\'s free cash can leave — borrows stay funded.',
      [['shares', a], ['what you sign', 'lend.withdrawSupply(you, ' + a + ')']],
      FtbState.dapi.withdrawSupply(u, a));
  });
  wire('lnNeoGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnNeoAmt', true); if (a === null) return badAmt();
    FtbShell.showPlan('add ' + a + ' NEO collateral',
      'NEO moves to the lend contract as collateral (whole units, 0 decimals). priced by the oracle at 100 FTB per NEO; 60% LTV.',
      [['amount', a + ' NEO'], ['what you sign', 'neo.transfer(you -> lend, ' + a + ')']],
      FtbState.dapi.addCollateralNeo(u, a));
  });
  wire('lnGasGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnGasAmt'); if (a === null) return badAmt();
    FtbShell.showPlan('add ' + a + ' GAS collateral',
      'GAS moves to the lend contract as collateral. priced by the oracle at 40 FTB per GAS; 60% LTV.',
      [['amount', a + ' GAS'], ['what you sign', 'gas.transfer(you -> lend, ' + a + ')']],
      FtbState.dapi.addCollateralGas(u, toBase(a)));
  });
  wire('lnWdNeoGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnNeoAmt', true); if (a === null) return badAmt();
    FtbShell.showPlan('withdraw ' + a + ' NEO collateral',
      'debt-free users withdraw without touching the oracle; with debt, your remaining collateral must still cover the LTV.',
      [['amount', a + ' NEO'], ['what you sign', 'lend.withdrawCollateral(you, ' + a + ' NEO, 0 GAS)']],
      FtbState.dapi.withdrawCollateral(u, a, 0));
  });
  wire('lnWdGasGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnGasAmt'); if (a === null) return badAmt();
    FtbShell.showPlan('withdraw ' + a + ' GAS collateral',
      'debt-free users withdraw without touching the oracle; with debt, your remaining collateral must still cover the LTV.',
      [['amount', a + ' GAS'], ['what you sign', 'lend.withdrawCollateral(you, 0 NEO, ' + a + ' GAS)']],
      FtbState.dapi.withdrawCollateral(u, 0, toBase(a)));
  });
  wire('lnBorrowGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnBorrowAmt'); if (a === null) return badAmt();
    var acct = FtbState.acctHash160();
    FtbRpc.readInt(C.lend, 'maxBorrow', [{ type: 'Hash160', value: acct }]).then(function (maxB) {
      var base = toBase(a);
      FtbShell.showPlan('borrow ' + a + ' FTB',
        'debt accrues at the borrow rate (2% base, 12% at the 80% kink). if your collateral value × 75% falls below debt, you can be liquidated.',
        [['amount', a + ' FTB'],
         ['your max borrow', fmt(maxB) + ' FTB'],
         ['within limit', BigInt(base) <= maxB ? 'yes' : 'NO — the chain will reject this'],
         ['what you sign', 'lend.borrow(you, ' + a + ')']],
        FtbState.dapi.borrow(u, base));
    }).catch(function () {
      FtbShell.showPlan('borrow ' + a + ' FTB', 'could not read your max borrow — the chain enforces the limit regardless.',
        [['amount', a + ' FTB'], ['what you sign', 'lend.borrow(you, ' + a + ')']],
        FtbState.dapi.borrow(u, toBase(a)));
    });
  });
  wire('lnRepayGo', function () {
    var u = needAcct(); if (!u) return;
    var a = amt('lnRepayAmt'); if (a === null) return badAmt();
    FtbShell.showPlan('repay ' + a + ' FTB',
      'transfers FTB to the pool against your debt. overpayment is fine — the contract credits only what you owe.',
      [['amount', a + ' FTB'], ['what you sign', 'ftb.transfer(you -> lend, ' + a + ', "repay")']],
      FtbState.dapi.repay(u, toBase(a)));
  });

  var lb = $('lendRefresh'); if (lb) lb.addEventListener('click', loadLend);
  FtbState.on('rpc', loadLend);
  FtbState.on('account', loadLend);
  FtbState.on('refresh', loadLend);
  // keep the pool fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadLend(); }, 60000);
})();
