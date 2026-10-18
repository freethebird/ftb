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

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt;
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
    FtbRpc.readInt(lend, 'collateralOf', hu.concat([{ type: 'Boolean', value: true }])).then(function (v) { $('pNeo').textContent = v.toString(); }).catch(function () {});
    FtbRpc.readInt(lend, 'collateralOf', hu.concat([{ type: 'Boolean', value: false }])).then(function (v) { $('pGas').textContent = fmt(v); }).catch(function () {});
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
  // screen 1: amount. screen 2: BOTH collateral options with predetermined
  // amounts, one click chooses. screen 3: the two transactions, in plain words.
  // every screen REPLACES the previous one — no stacking, no duplicates.
  function collatNeed(loanBase, price, wholeNeo) {
    // loan ÷ (60% LTV × price) + 10% safety, rounded up (BigInt exact)
    var num = loanBase * 110n, den = 60n * price;
    return wholeNeo ? (num + den - 1n) / den : (num * 100000000n + den - 1n) / den;
  }
  function wizLoan() {
    if (!needAcct()) return;
    clearWiz();
    var s1 = stepBox('1', 'how many ftb do you want to borrow?');
    var in1 = mkInput('0.0');
    var k1 = keyWrap(); k1.appendChild(mkKey('next ->', function () {
      var a = amtVal(in1.value); if (a === null) return badAmt();
      wizLoanPick(BigInt(toBase(a)), a);
    }));
    s1.row.appendChild(in1); s1.row.appendChild(k1);
    wiz.appendChild(s1.box);
    in1.focus();
  }
  function wizLoanPick(loanBase, loanDec) {
    clearWiz();
    var s = stepBox('2', 'choose your collateral — the amounts are predetermined');
    s.row.appendChild(el('span', 'gname', 'reading oracle…'));
    var note = wizNote(s.box, '');
    wiz.appendChild(s.box);
    var assets = FtbState.CFG.assets;
    Promise.all([
      FtbRpc.readInt(C.oracle, 'price', [h160(assets.neo)]),
      FtbRpc.readInt(C.oracle, 'price', [h160(assets.gas)])
    ]).then(function (ps) {
      var needNeo = collatNeed(loanBase, ps[0], true);
      var needGas = collatNeed(loanBase, ps[1], false);
      s.row.removeChild(s.row.lastChild);
      // one row per option: predetermined amount on the left, the choice key on the right.
      // [name, display, price, isNeo, rawArg] — rawArg is the exact contract integer.
      [['NEO', needNeo.toString(), ps[0], true, needNeo.toString()],
       ['GAS', fromBase(needGas.toString()), ps[1], false, needGas.toString()]].forEach(function (o) {
        var r = el('div', 'trow');
        var lbl = el('label'); lbl.textContent = '';
        r.appendChild(lbl);
        r.appendChild(el('span', 'gname', o[1] + ' ' + o[0]));
        var kw = keyWrap();
        kw.appendChild(mkKey('use ' + o[0].toLowerCase() + ' ->', function () {
          wizLoanSign(loanBase, loanDec, o[0], o[1], o[2], o[3], o[4]);
        }));
        r.appendChild(kw);
        s.box.insertBefore(r, note);
      });
      note.textContent = 'predetermined from the live oracle: ' + loanDec + ' FTB ÷ (60% LTV × ratio) + 10% safety. ratios now: 1 NEO = ' +
        fromBase(ps[0].toString()) + ' FTB · 1 GAS = ' + fromBase(ps[1].toString()) + ' FTB.';
    }).catch(function (e) {
      s.row.removeChild(s.row.lastChild);
      s.row.appendChild(el('span', 'gname', 'oracle unavailable'));
      note.textContent = 'no fresh oracle price: ' + FtbUtil.errText(e);
    });
  }
  function wizLoanSign(loanBase, loanDec, which, needDec, price, wholeNeo, needArg) {
    clearWiz();
    var s = stepBox('3', 'two signatures — lock your collateral, then take the loan');
    var priceDec = fromBase(price.toString());
    s.row.appendChild(el('span', 'gname', loanDec + ' FTB  against  ' + needDec + ' ' + which));
    var keys = keyWrap();
    keys.appendChild(mkKey('1. lock ' + needDec + ' ' + which + ' ->', function () {
      var plan = wholeNeo
        ? FtbState.dapi.addCollateralNeo(FtbState.account, needArg)
        : FtbState.dapi.addCollateralGas(FtbState.account, needArg);
      FtbShell.showPlan('lock ' + needDec + ' ' + which + ' as collateral',
        'transaction 1 of 2: your ' + which + ' moves into the lend contract as collateral. it stays yours — you get it back when the loan is repaid.',
        [['collateral', needDec + ' ' + which], ['oracle ratio', '1 ' + which + ' = ' + priceDec + ' FTB'], ['what you sign', plan.summary]],
        plan);
    }));
    keys.appendChild(mkKey('2. take ' + loanDec + ' ftb loan ->', function () {
      var acct = FtbState.acctHash160();
      FtbRpc.readInt(C.lend, 'maxBorrow', [h160(acct)]).then(function (maxB) {
        FtbShell.showPlan('take a ' + loanDec + ' FTB loan',
          'transaction 2 of 2: the contract sends you ' + loanDec + ' FTB. interest accrues (2% base, 12% at the 80% kink); repay any time to unlock your collateral.',
          [['you receive', loanDec + ' FTB'],
           ['your max borrow', fmt(maxB) + ' FTB'],
           ['within limit', loanBase <= maxB ? 'yes' : 'NO — lock the collateral first'],
           ['what you sign', 'lend.borrow(you, ' + loanDec + ')']],
          FtbState.dapi.borrow(FtbState.account, loanBase.toString()));
      }).catch(function () {
        FtbShell.showPlan('take a ' + loanDec + ' FTB loan', 'could not read max borrow — the chain enforces it regardless.',
          [['you receive', loanDec + ' FTB']], FtbState.dapi.borrow(FtbState.account, loanBase.toString()));
      });
    }));
    s.row.appendChild(keys);
    wizNote(s.box, 'a loan is two transactions, signed one after the other: 1) your ' + which +
      ' locks as collateral — wait for "confirmed on-chain" — 2) the loan lands in your wallet. your collateral is returned when you repay.');
    wiz.appendChild(s.box);
  }

  // ----- B) pay loan -----
  function wizPay() {
    if (!needAcct()) return;
    clearWiz();
    var s = stepBox('1', 'repay how much ftb?');
    var acct = FtbState.acctHash160();
    var input = mkInput('0.0');
    FtbRpc.readInt(C.lend, 'debtOf', [h160(acct)]).then(function (d) {
      // interest accrues per block, so the exact debt at read-time is already
      // stale when the tx lands — prefill +0.001 FTB; repayIn refunds the excess.
      input.value = fromBase((d + 100000n).toString());
      note.textContent = 'your debt right now: ' + fmt(d) + ' FTB (prefilled a hair higher — interest ticks every block; the contract refunds whatever isn\'t owed).';
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

  // ----- E) withdraw collateral -----
  function wizWdCol() {
    if (!needAcct()) return;
    clearWiz();
    var s = stepBox('1', 'take back your collateral');
    var acct = FtbState.acctHash160();
    var note = wizNote(s.box, 'reading your collateral…');
    var keys = keyWrap();
    var state = { neo: null, gas: null };
    Promise.all([
      FtbRpc.readInt(C.lend, 'collateralOf', [h160(acct), { type: 'Boolean', value: true }]),
      FtbRpc.readInt(C.lend, 'collateralOf', [h160(acct), { type: 'Boolean', value: false }]),
      FtbRpc.readInt(C.lend, 'debtOf', [h160(acct)])
    ]).then(function (vs) {
      state.neo = vs[0]; state.gas = vs[1];
      note.textContent = 'locked: ' + vs[0].toString() + ' NEO · ' + fmt(vs[1]) + ' GAS. debt: ' + fmt(vs[2]) +
        ' FTB — ' + (vs[2] > 0n ? 'repay first (or the chain enforces the LTV on what stays).' : 'zero, so everything below comes back in one transaction.');
    }).catch(function () { note.textContent = 'could not read your collateral.'; });
    keys.appendChild(mkKey('return my collateral ->', function () {
      if (state.neo === null) return;
      if (state.neo === 0n && state.gas === 0n) {
        FtbShell.showPlan('nothing locked', 'you have no collateral in the lend contract.', [], null);
        return;
      }
      var plan = FtbState.dapi.withdrawCollateral(FtbState.account, state.neo.toString(), state.gas.toString());
      FtbShell.showPlan('return ' + state.neo.toString() + ' NEO + ' + fmt(state.gas) + ' GAS',
        'one transaction returns everything you have locked. if any debt remains, the contract keeps enough collateral to cover the LTV — repay first to empty it.',
        [['NEO back', state.neo.toString()], ['GAS back', fmt(state.gas)], ['what you sign', plan.summary]],
        plan);
    }));
    s.row.appendChild(el('span', 'gname', 'all of it'));
    s.row.appendChild(keys);
    wiz.appendChild(s.box);
  }

  function wire(id, fn) { var b = $(id); if (b) b.addEventListener('click', fn); }
  wire('lnWizLoan', wizLoan);
  wire('lnWizPay', wizPay);
  wire('lnWizWdCol', wizWdCol);
  wire('lnWizAdd', wizAdd);
  wire('lnWizRemove', wizRemove);

  var lb = $('lendRefresh'); if (lb) lb.addEventListener('click', loadLend);
  FtbState.on('rpc', loadLend);
  FtbState.on('account', loadLend);
  FtbState.on('refresh', loadLend);
  // keep the pool fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadLend(); }, 60000);
})();
