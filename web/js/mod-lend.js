// mod-lend.js — module 4: LEND (MODULES.md).
// NEO+GAS collateral, FTB loans. This page is read-only for the pool; every
// position-changing action is a review-and-sign plan through the adapter.
//
// Owns (writes only these): #lend card — #lUtil, #lBApy, #lSApy, #lBorrows,
// #lReserve, #lendPos (#pHealth, #pNeo, #pGas, #pDebt, #pMax, #pShares),
// #lendAct (the four wizard keys #lnWizLoan/#lnWizPay/#lnWizAdd/#lnWizRemove
// and the step-by-step panel #lnWizard they build), #lendOff, #lendRefresh.
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

  // ---------- wizards: one question at a time ----------
  // A) start loan: 1) FTB amount 2) NEO|GAS 3) predetermined collateral, sign.
  // B) pay loan. C) add liquidity (shows approx APY). D) remove liquidity.
  // Final steps are pure dapi plans executed only by the shell's modal (R6).
  var toBase = FtbDapi.util.toBaseUnits, fromBase = FtbDapi.util.fromBaseUnits;
  var wiz = $('lnWizard');

  function amtVal(v, whole) {
    v = (v || '').trim();
    var re = whole ? /^[0-9]+$/ : /^[0-9]+(\.[0-9]+)?$/;
    return (re.test(v) && parseFloat(v) > 0) ? v : null;
  }
  function needAcct() {
    if (!FtbState.account) { FtbWallet.needConnect(); return null; }
    return FtbState.account;
  }
  function h160(hash) { return { type: 'Hash160', value: hash }; }
  function clearWiz() { if (wiz) wiz.innerHTML = ''; }
  function stepBox(no, labelText) {
    var box = el('div', 'ticket'); box.style.marginTop = '10px';
    var row = el('div', 'trow');
    var lbl = el('label'); lbl.textContent = no + ') ' + labelText;
    row.appendChild(lbl);
    box.appendChild(row);
    return { box: box, row: row };
  }
  function wizNote(box, text) {
    var n = el('div', 'tmeta'); n.textContent = text; box.appendChild(n); return n;
  }
  function mkInput(placeholder, mode) {
    var i = el('input'); i.placeholder = placeholder; i.inputMode = mode || 'decimal';
    i.autocomplete = 'off'; return i;
  }
  function mkKey(label, fn) {
    var b = el('button', null, label);
    b.addEventListener('click', fn); return b;
  }
  function keyWrap() { return el('div', 'tsel'); }
  function badAmt() {
    FtbShell.showPlan('bad amount', 'enter a positive number. NEO is whole units only (0 decimals); FTB and GAS take 8.', [], null);
  }

  // ----- A) start loan -----
  function wizLoan() {
    if (!needAcct()) return;
    clearWiz();
    var s1 = stepBox('1', 'how many ftb do you want to borrow?');
    var in1 = mkInput('0.0');
    var k1 = keyWrap(); k1.appendChild(mkKey('next ->', function () {
      var a = amtVal(in1.value); if (a === null) return badAmt();
      wizLoan2(BigInt(toBase(a)), a);
    }));
    s1.row.appendChild(in1); s1.row.appendChild(k1);
    wiz.appendChild(s1.box);
    in1.focus();
  }
  function wizLoan2(loanBase, loanDec) {
    var s2 = stepBox('2', 'collateral — neo or gas?');
    var which = { v: null };
    var bN = el('button', null, 'NEO'); var bG = el('button', null, 'GAS');
    bN.addEventListener('click', function () { which.v = 'NEO'; bN.classList.add('on'); bG.classList.remove('on'); });
    bG.addEventListener('click', function () { which.v = 'GAS'; bG.classList.add('on'); bN.classList.remove('on'); });
    var k2 = keyWrap(); k2.appendChild(bN); k2.appendChild(bG);
    k2.appendChild(mkKey('next ->', function () {
      if (!which.v) return;
      wizLoan3(loanBase, loanDec, which.v);
    }));
    s2.row.appendChild(el('span', null, '')); s2.row.appendChild(k2);
    wiz.appendChild(s2.box);
  }
  function wizLoan3(loanBase, loanDec, which) {
    var s3 = stepBox('3', 'collateral amount — predetermined by the oracle ratio');
    var assetHash = which === 'NEO' ? FtbState.CFG.assets.neo : FtbState.CFG.assets.gas;
    s3.row.appendChild(el('span', 'gname', 'reading oracle…'));
    var note = wizNote(s3.box, '');
    wiz.appendChild(s3.box);
    FtbRpc.readInt(C.oracle, 'price', [h160(assetHash)]).then(function (price) {
      // value = whole×price (NEO) or base×price/1e8 (GAS); maxBorrow = value×60%.
      // required = loan ÷ 0.6 ÷ price, +10% safety so a small price move can't fault the borrow.
      var num = loanBase * 110n, den = 60n * price;
      var needWhole = (num + den - 1n) / den;                     // NEO: whole units
      var needBase = (num * 100000000n + den - 1n) / den;         // GAS: base units
      var needDec = which === 'NEO' ? needWhole.toString() : fromBase(needBase.toString());
      var priceDec = fromBase(price.toString());
      s3.row.removeChild(s3.row.lastChild);
      s3.row.appendChild(el('span', 'gname', needDec + ' ' + which));
      var keys = keyWrap();
      keys.appendChild(mkKey('1. sign deposit ->', function () {
        var plan = which === 'NEO'
          ? FtbState.dapi.addCollateralNeo(FtbState.account, needWhole.toString())
          : FtbState.dapi.addCollateralGas(FtbState.account, needBase.toString());
        FtbShell.showPlan('deposit ' + needDec + ' ' + which + ' collateral',
          'predetermined: ' + loanDec + ' FTB ÷ (60% LTV × ' + priceDec + ' FTB per ' + which + ') + 10% safety. after it confirms, sign the borrow.',
          [['collateral', needDec + ' ' + which], ['oracle ratio', '1 ' + which + ' = ' + priceDec + ' FTB'], ['what you sign', plan.summary]],
          plan);
      }));
      keys.appendChild(mkKey('2. sign borrow ->', function () {
        var acct = FtbState.acctHash160();
        FtbRpc.readInt(C.lend, 'maxBorrow', [h160(acct)]).then(function (maxB) {
          FtbShell.showPlan('borrow ' + loanDec + ' FTB',
            'debt accrues at the borrow rate (2% base, 12% at the 80% kink). below 75% collateral cover you can be liquidated.',
            [['amount', loanDec + ' FTB'],
             ['your max borrow', fmt(maxB) + ' FTB'],
             ['within limit', loanBase <= maxB ? 'yes' : 'NO — deposit first, or more'],
             ['what you sign', 'lend.borrow(you, ' + loanDec + ')']],
            FtbState.dapi.borrow(FtbState.account, loanBase.toString()));
        }).catch(function () {
          FtbShell.showPlan('borrow ' + loanDec + ' FTB', 'could not read max borrow — the chain enforces it regardless.',
            [['amount', loanDec + ' FTB']], FtbState.dapi.borrow(FtbState.account, loanBase.toString()));
        });
      }));
      s3.row.appendChild(keys);
      note.textContent = 'predetermined: ' + loanDec + ' FTB ÷ (60% LTV × ' + priceDec + ' FTB per ' + which + ') + 10% safety margin. sign the deposit, wait for "confirmed on-chain", then sign the borrow.';
    }).catch(function (e) {
      s3.row.removeChild(s3.row.lastChild);
      s3.row.appendChild(el('span', 'gname', 'oracle unavailable'));
      note.textContent = 'the oracle has no fresh ' + which + ' price: ' + FtbUtil.errText(e);
    });
  }

  // ----- B) pay loan -----
  function wizPay() {
    if (!needAcct()) return;
    clearWiz();
    var s = stepBox('1', 'repay how much ftb?');
    var acct = FtbState.acctHash160();
    var input = mkInput('0.0');
    FtbRpc.readInt(C.lend, 'debtOf', [h160(acct)]).then(function (d) {
      input.value = fromBase(d.toString());
      note.textContent = 'your debt right now: ' + fmt(d) + ' FTB (prefilled — overpayment credits only what you owe).';
    }).catch(function () {});
    var note = wizNote(s.box, 'reading your debt…');
    var keys = keyWrap();
    keys.appendChild(mkKey('sign repayment ->', function () {
      var a = amtVal(input.value); if (a === null) return badAmt();
      FtbShell.showPlan('repay ' + a + ' FTB',
        'transfers FTB to the pool against your debt.',
        [['amount', a + ' FTB'], ['what you sign', 'ftb.transfer(you -> lend, ' + a + ', "repay")']],
        FtbState.dapi.repay(FtbState.account, toBase(a)));
    }));
    s.row.appendChild(input); s.row.appendChild(keys);
    wiz.appendChild(s.box);
    input.focus();
  }

  // ----- C) add liquidity -----
  function wizAdd() {
    if (!needAcct()) return;
    clearWiz();
    var s = stepBox('1', 'how many ftb do you lend to the pool?');
    var input = mkInput('0.0');
    var note = wizNote(s.box, 'reading the rate…');
    FtbRpc.readInt(C.lend, 'supplyApyBps', []).then(function (v) {
      note.textContent = 'approximate apy right now: ' + (Number(v) / 100).toFixed(2) + '% — it floats with utilization (borrow × util × 2/3).';
    }).catch(function () { note.textContent = 'apy unreadable — the chain still enforces the curve.'; });
    var keys = keyWrap();
    keys.appendChild(mkKey('sign supply ->', function () {
      var a = amtVal(input.value); if (a === null) return badAmt();
      FtbShell.showPlan('add ' + a + ' FTB liquidity',
        'you receive pool shares; interest accrues to the share price.',
        [['amount', a + ' FTB'], ['what you sign', 'ftb.transfer(you -> lend, ' + a + ', "supply")']],
        FtbState.dapi.supply(FtbState.account, toBase(a)));
    }));
    s.row.appendChild(input); s.row.appendChild(keys);
    wiz.appendChild(s.box);
    input.focus();
  }

  // ----- D) remove liquidity -----
  function wizRemove() {
    if (!needAcct()) return;
    clearWiz();
    var s = stepBox('1', 'withdraw how many shares?');
    var input = mkInput('0', 'numeric');
    var note = wizNote(s.box, 'reading your shares…');
    var acct = FtbState.acctHash160();
    FtbRpc.readInt(C.lend, 'sharesOf', [h160(acct)]).then(function (v) {
      input.value = v.toString();
      note.textContent = 'you hold ' + v.toString() + ' shares (prefilled with all of them). shares pay out FTB at the current share price.';
    }).catch(function () {});
    var keys = keyWrap();
    keys.appendChild(mkKey('sign withdrawal ->', function () {
      var a = amtVal(input.value, true); if (a === null) return badAmt();
      FtbShell.showPlan('withdraw ' + a + ' shares',
        'burns pool shares and returns FTB at the current share price. only free cash can leave — borrows stay funded.',
        [['shares', a], ['what you sign', 'lend.withdrawSupply(you, ' + a + ')']],
        FtbState.dapi.withdrawSupply(FtbState.account, a));
    }));
    s.row.appendChild(input); s.row.appendChild(keys);
    wiz.appendChild(s.box);
    input.focus();
  }

  function wire(id, fn) { var b = $(id); if (b) b.addEventListener('click', fn); }
  wire('lnWizLoan', wizLoan);
  wire('lnWizPay', wizPay);
  wire('lnWizAdd', wizAdd);
  wire('lnWizRemove', wizRemove);

  var lb = $('lendRefresh'); if (lb) lb.addEventListener('click', loadLend);
  FtbState.on('rpc', loadLend);
  FtbState.on('account', loadLend);
  FtbState.on('refresh', loadLend);
  // keep the pool fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadLend(); }, 60000);
})();
