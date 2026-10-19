// mod-lend.js — module 4: LEND (MODULES.md).
// NEO+GAS collateral, FTB loans. This page is read-only for the pool; every
// position-changing action is a review-and-sign plan through the adapter.
//
// Read discipline: loads coalesce (one pass in flight, at most one trailing
// run, 3 s minimum interval) — the shell emits 'refresh' in a burst around
// every signed tx, and the board must not re-enumerate storage per event.
// Board enumeration paginates findstorage (next/truncated) and says so when
// the page cap is hit; address rendering uses the adapter's codec, never
// crypto.subtle, so the board also works on plain-http mirrors.
//
// Owns (writes only these): #lend card — the 4-stat panel (#lSApy liquidity apy,
// #lBApy borrow apy, #lTotal total liquidity, #pHealth), #lendAct (the wizard keys
// #lnWizLoan/#lnWizPay/#lnWizWdCol/#lnWizAdd/#lnWizRemove and the step panel
// #lnWizard they build), #lendOff, #lendRefresh, and the loans board (#loansBody
// plus its totals strip #lbLoaned/#lbCollat/#lbCount, #loansRefresh). All detail
// lives on the board: per-position debt/collateral/health, your headroom, and the
// permanent LIQUIDATED tag (liquidationsOf, prefix 0x34).
(function () {
  'use strict';
  if (!FtbUtil.$('lend')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, grp = FtbUtil.grp;
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
    FtbRpc.readInt(lend, 'borrowApyBps').then(function (v) { $('lBApy').textContent = bps(v); }).catch(function () {});
    FtbRpc.readInt(lend, 'supplyApyBps').then(function (v) { $('lSApy').textContent = bps(v); }).catch(function () {});
    // total liquidity = what suppliers would withdraw today: cash + outstanding
    // borrows, minus the holders' reserve (that third belongs to FTB holders).
    Promise.all([
      FtbRpc.readInt(C.ftb, 'balanceOf', [h160(lend)]),
      FtbRpc.readInt(lend, 'totalBorrows', []),
      FtbRpc.readInt(lend, 'reserveAccrued', [])
    ]).then(function (vs) { $('lTotal').textContent = fmt(vs[0] + vs[1] - vs[2]) + ' FTB'; }).catch(function () {});
    var acct = FtbState.acctHash160();
    if (!acct) return;
    var act = $('lendAct'); if (act) act.hidden = false;
    var hu = [{ type: 'Hash160', value: acct }];
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
      [['NEO', grp(needNeo.toString()), ps[0], true, needNeo.toString()],
       ['GAS', grp(fromBase(needGas.toString())), ps[1], false, needGas.toString()]].forEach(function (o) {
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
    s.row.appendChild(keys);
    wizNote(s.box, 'a loan is two transactions, signed one after the other: 1) your ' + which +
      ' locks as collateral — wait for "confirmed on-chain" — 2) the loan lands in your wallet. your collateral is returned when you repay.');
    wiz.appendChild(s.box);

    // Pre-read the position: if collateral already on-chain covers this loan
    // (deposit confirmed, borrow failed or page reloaded), step 1 is DONE — never
    // offer a second deposit. This is read from the chain, not remembered locally.
    var acct0 = FtbState.acctHash160();
    FtbRpc.readInt(C.lend, 'maxBorrow', [h160(acct0)]).then(function (maxB) {
      if (maxB >= BigInt(loanBase)) {
        var doneK = mkKey('1. collateral already locked ✓', function () {});
        doneK.disabled = true;
        keys.insertBefore(doneK, keys.firstChild);
        return;
      }
      keys.insertBefore(mkKey('1. lock ' + needDec + ' ' + which + ' ->', function () {
        var plan = wholeNeo
          ? FtbState.dapi.addCollateralNeo(FtbState.account, needArg)
          : FtbState.dapi.addCollateralGas(FtbState.account, needArg);
        FtbShell.showPlan('lock ' + needDec + ' ' + which + ' as collateral',
          'transaction 1 of 2: your ' + which + ' moves into the lend contract as collateral. it stays yours — you get it back when the loan is repaid.',
          [['collateral', needDec + ' ' + which], ['oracle ratio', '1 ' + which + ' = ' + priceDec + ' FTB'], ['what you sign', plan.summary]],
          plan);
      }), keys.firstChild);
    }).catch(function () {});

    keys.appendChild(borrowPlanKey('2. take ' + loanDec + ' ftb loan ->', loanBase, loanDec, 0n));
  }

  // The borrow button, shared by the loan wizard (step 2) and the loans board's
  // direct "take loan" action. Pre-checks BOTH chain gates before asking for a
  // signature: LTV headroom (existingDebt counts — the board path borrows against
  // collateral that may already back a loan) and pool liquidity. An empty pool
  // faults the borrow with "not enough liquidity" — show it here, in words,
  // instead of an opaque wallet error after signing.
  function borrowPlanKey(label, loanBase, loanDec, existingDebt) {
    return mkKey(label, function () {
      var acct = FtbState.acctHash160();
      Promise.all([
        FtbRpc.readInt(C.lend, 'maxBorrow', [h160(acct)]),
        FtbRpc.readInt(C.ftb, 'balanceOf', [h160(C.lend)]).catch(function () { return 0n; }),
        FtbRpc.readInt(C.lend, 'reserveAccrued', []).catch(function () { return 0n; })
      ]).then(function (vs) {
        var maxB = vs[0], head = maxB - BigInt(existingDebt), liquid = vs[1] - vs[2];
        var okLtv = BigInt(loanBase) <= head;
        var okLiq = BigInt(loanBase) <= liquid;
        // express the limit the way users think about it: the % LTV after this loan
        var ltvAfter = maxB > 0n ? (Number((BigInt(existingDebt) + BigInt(loanBase)) * 10000n / maxB) * 0.006).toFixed(1) : '—';
        FtbShell.showPlan('take a ' + loanDec + ' FTB loan',
          okLtv && okLiq
            ? 'the contract sends you ' + loanDec + ' FTB against your locked collateral. interest accrues (2% base, 12% at the 80% kink); repay any time to unlock your collateral.'
            : 'this will be REJECTED on-chain — fix the failing check first.',
          [['you receive', loanDec + ' FTB'],
           ['ltv after this loan', ltvAfter + '% of the 60% max — ' + (okLtv ? 'within limit' : 'NO — over the limit; lock more collateral or repay first')],
           ['pool liquidity', fmt(liquid) + ' FTB — ' + (okLiq ? 'enough' : 'NO — the pool is short. add liquidity (ADD LIQUIDITY below) or borrow less')],
           ['what you sign', 'lend.borrow(you, ' + loanDec + ')']],
          okLtv && okLiq ? FtbState.dapi.borrow(FtbState.account, loanBase.toString()) : null);
      }).catch(function () {
        FtbShell.showPlan('take a ' + loanDec + ' FTB loan', 'could not pre-check the chain — the contract enforces both limits regardless.',
          [['you receive', loanDec + ' FTB']], FtbState.dapi.borrow(FtbState.account, loanBase.toString()));
      });
    });
  }

  // ----- A2) borrow against collateral already locked (from the loans board) -----
  // A deposit without a borrow is a valid position; its owner must be able to draw
  // the loan later without restarting the wizard. Opened from the board's
  // "take loan ->" key on your own row.
  function wizBorrowDirect() {
    if (!needAcct()) return;
    clearWiz();
    var s = stepBox('1', 'borrow how much ftb against your locked collateral?');
    var input = mkInput('0.0');
    var note = wizNote(s.box, 'reading your headroom…');
    var acct = FtbState.acctHash160();
    var curDebt = 0n;
    Promise.all([
      FtbRpc.readInt(C.lend, 'maxBorrow', [h160(acct)]),
      FtbRpc.readInt(C.lend, 'debtOf', [h160(acct)])
    ]).then(function (vs) {
      curDebt = vs[1];
      var head = vs[0] - vs[1];
      if (head < 0n) head = 0n;
      var ltv = vs[0] > 0n ? (Number(vs[1] * 10000n / vs[0]) * 0.006).toFixed(1) : '0.0';
      input.value = fromBase(head.toString());
      note.textContent = 'your ltv is ' + ltv + '% (max 60%). headroom: ' + fmt(head) + ' FTB — prefilled with the maximum; borrow less for a bigger safety buffer against liquidation.';
    }).catch(function () { note.textContent = 'could not read your position — the chain enforces the limit regardless.'; });
    var keys = keyWrap();
    keys.appendChild(mkKey('review borrow ->', function () {
      var a = amtVal(input.value); if (a === null) return badAmt();
      var kb = borrowPlanKey('take ' + a + ' ftb ->', toBase(a), a, curDebt);
      kb.click();
    }));
    s.row.appendChild(input); s.row.appendChild(keys);
    wiz.appendChild(s.box);
    input.focus();
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
      note.textContent = 'approximate apy right now: ' + (Number(v) / 100).toFixed(2) + '% — it floats with how much of the pool is loaned out (borrow rate × % loaned × 2/3).';
    }).catch(function () { note.textContent = 'apy unreadable — the chain still enforces the curve.'; });
    var keys = keyWrap();
    keys.appendChild(mkKey('sign liquidity ->', function () {
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
      note.textContent = 'you hold ' + fmt(v) + ' shares (shares are 8-decimal, like FTB — prefilled with all of them). shares pay out FTB at the current share price.';
    }).catch(function () {});
    var keys = keyWrap();
    keys.appendChild(mkKey('sign withdrawal ->', function () {
      var a = amtVal(input.value, true); if (a === null) return badAmt();
      FtbShell.showPlan('withdraw ' + fmt(BigInt(a)) + ' shares',
        'burns pool shares and returns FTB at the current share price. only free cash can leave — borrows stay funded.',
        [['shares', fmt(BigInt(a))], ['what you sign', 'lend.withdrawSupply(you, ' + a + ')']],
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
      state.neo = vs[0]; state.gas = vs[1]; state.debt = vs[2];
      keys.innerHTML = '';
      if (vs[2] > 0n) {
        note.textContent = 'you still owe ' + fmt(vs[2]) + ' FTB — even dust debt locks collateral. repay it first (PAY LOAN over-prefills and the contract refunds the excess), or take back only what stays free.';
        keys.appendChild(mkKey('pay loan first ->', wizPay));
        keys.appendChild(mkKey('claim collateral ->', function () { wizWdFree(state); }));
      } else {
        note.textContent = 'locked: ' + grp(vs[0].toString()) + ' NEO · ' + fmt(vs[1]) + ' GAS — debt is zero, so everything comes back in one transaction.';
        keys.appendChild(mkKey('claim collateral ->', function () {
          if (state.neo === 0n && state.gas === 0n) {
            FtbShell.showPlan('nothing locked', 'you have no collateral in the lend contract.', [], null);
            return;
          }
          var plan = FtbState.dapi.withdrawCollateral(FtbState.account, state.neo.toString(), state.gas.toString());
          FtbShell.showPlan('claim ' + grp(state.neo.toString()) + ' NEO + ' + fmt(state.gas) + ' GAS',
            'one transaction returns everything you have locked.',
            [['NEO back', grp(state.neo.toString())], ['GAS back', fmt(state.gas)], ['what you sign', plan.summary]],
            plan);
        }));
      }
    }).catch(function () { note.textContent = 'could not read your collateral.'; });
    s.row.appendChild(el('span', 'gname', 'all of it'));
    s.row.appendChild(keys);
    wiz.appendChild(s.box);
  }
  // partial withdrawal while debt exists: keep enough value for debt ÷ 60% + 5%
  // margin. NEO leaves only if the GAS alone can cover; GAS leaves down to the cover.
  function wizWdFree(state) {
    var assets = FtbState.CFG.assets;
    Promise.all([
      FtbRpc.readInt(C.oracle, 'price', [h160(assets.neo)]),
      FtbRpc.readInt(C.oracle, 'price', [h160(assets.gas)])
    ]).then(function (ps) {
      var pN = ps[0], pG = ps[1];
      var needVal = state.debt * 105n / 60n + 1n;                 // FTB base that must stay covered
      var neoVal = state.neo * pN;                                // NEO: value = whole × price
      var gasVal = state.gas * pG / 100000000n;                   // GAS: value = base × price / 1e8
      var freeNeo = gasVal >= needVal ? state.neo : 0n;
      var coverFromNeo = (freeNeo > 0n ? 0n : neoVal);
      var shortfall = needVal > coverFromNeo ? needVal - coverFromNeo : 0n;
      var keepGas = shortfall * 100000000n / pG + 1n;
      var freeGas = state.gas > keepGas ? state.gas - keepGas : 0n;
      if (freeNeo === 0n && freeGas === 0n) {
        FtbShell.showPlan('nothing free', 'everything locked is covering your debt. repay first to free it.', [], null);
        return;
      }
      var plan = FtbState.dapi.withdrawCollateral(FtbState.account, freeNeo.toString(), freeGas.toString());
      FtbShell.showPlan('claim ' + grp(freeNeo.toString()) + ' NEO + ' + fmt(freeGas) + ' GAS',
        'the most that can leave while your ' + fmt(state.debt) + ' FTB debt stays covered (60% LTV + 5% margin). repay to free the rest.',
        [['NEO back', grp(freeNeo.toString())], ['GAS back', fmt(freeGas)], ['what you sign', plan.summary]],
        plan);
    }).catch(function (e) {
      FtbShell.showPlan('oracle unavailable', FtbUtil.errText(e), [], null);
    });
  }

  function wire(id, fn) { var b = $(id); if (b) b.addEventListener('click', fn); }
  wire('lnWizLoan', wizLoan);
  wire('lnWizPay', wizPay);
  wire('lnWizWdCol', wizWdCol);
  wire('lnWizAdd', wizAdd);
  wire('lnWizRemove', wizRemove);

  // ----- loans board: every position enumerated from contract storage -----
  // prefixes 0x31–0x34 via findstorage, PAGINATED: the C# RPC caps one page at
  // 100 entries and answers {results, next, truncated}; we follow `next` until
  // done, capped at MAX_PAGES — past the cap the board SAYS it is truncated
  // instead of silently showing a partial enumeration (R5/R8).
  var MAX_PAGES = 5;
  function findAll(hash, prefixB64) {
    var out = [];
    function page(start) {
      var params = start == null ? [hash, prefixB64] : [hash, prefixB64, start];
      return FtbRpc.call('findstorage', params).then(function (r) {
        if (Array.isArray(r)) return { items: out.concat(r), truncated: false }; // neo-go shape: one full array
        out = out.concat((r && r.results) || []);
        if (r && r.truncated) {
          if (out.length >= MAX_PAGES * 100) return { items: out, truncated: true };
          return page(r.next);
        }
        return { items: out, truncated: false };
      });
    }
    return page(null);
  }

  // Refresh coalescing lives in the shared seam (FtbUtil.coalesce): the shell
  // emits 'refresh' in a burst around every signed transaction (optimistic, each
  // 5 s confirmation poll, confirmed, the modal's ok & refresh) — up to ~17 events
  // for one landing — and without coalescing a slower STALE pass can resolve last
  // and paint old numbers over fresh ones.

  // Addresses render through the adapter's scriptHashToAddress (pure-JS SHA-256,
  // no crypto.subtle) — the Article 10 single toolkit; this module keeps no codec.
  function loadLoans() {
    var body = $('loansBody'); if (!body || !FtbRpc.current() || !C.lend) return;
    body.textContent = 'reading chain storage…';
    // Four prefixes: 0x31 debt ('MQ=='), 0x32 NEO collateral ('Mg=='), 0x33 GAS
    // collateral ('Mw=='), 0x34 liquidation count ('NA=='). A deposit can land
    // without its borrow (failed second signature), collateral outlives a repaid
    // loan, and a LIQUIDATION outlives the position itself — all must be VISIBLE,
    // or users think funds (or history) vanished.
    Promise.all(['MQ==', 'Mg==', 'Mw==', 'NA=='].map(function (p) {
      return findAll(C.lend, p).catch(function () { return { items: [], truncated: false }; });
    })).then(function (sets) {
      var boardTrunc = sets.some(function (s) { return s.truncated; });
      var seen = {}, users = [];
      sets.forEach(function (s) {
        s.items.forEach(function (e) {
          var sh = FtbDapi.util.b64ToHex(e.key).slice(2); // storage-order bytes, prefix dropped
          if (!seen[sh]) { seen[sh] = 1; users.push(sh); }
        });
      });
      if (!users.length) {
        body.textContent = 'no positions yet.';
        // the strip is earned numbers (R5): with zero positions it reads zero, not a stale sum
        var z1 = $('lbLoaned'); if (z1) z1.textContent = '0 FTB';
        var z2 = $('lbUtil'); if (z2) z2.textContent = '0.0%';
        var z3 = $('lbCollat'); if (z3) z3.textContent = '0 NEO + 0 GAS';
        var z4 = $('lbCount'); if (z4) z4.textContent = '0';
        return;
      }
      return Promise.all(users.map(function (sh) {
        var be = '0x' + FtbDapi.util.reverseHex(sh); // canonical RPC form for Hash160 args
        return Promise.all([
          FtbRpc.readInt(C.lend, 'debtOf', [h160(be)]),
          FtbRpc.readInt(C.lend, 'collateralOf', [h160(be), { type: 'Boolean', value: true }]),
          FtbRpc.readInt(C.lend, 'collateralOf', [h160(be), { type: 'Boolean', value: false }]),
          FtbRpc.read(C.lend, 'isUnhealthy', [h160(be)]).then(function (st) {
            var it = st && st[0];
            return it && (it.type === 'Boolean' ? it.value : (it.value && it.value !== '0' && it.value !== ''));
          }).catch(function () { return false; }),
          FtbRpc.readInt(C.lend, 'liquidationsOf', [h160(be)]).catch(function () { return 0n; }),
          FtbRpc.readInt(C.lend, 'maxBorrow', [h160(be)]).catch(function () { return 0n; })
        ]).then(function (vs) {
          return { addr: FtbDapi.util.scriptHashToAddress(sh), be: be, debt: vs[0], neo: vs[1], gas: vs[2], bad: !!vs[3], liqs: vs[4], maxB: vs[5] };
        });
      })).then(function (rows) {
        var open = rows.filter(function (r) { return r.debt > 0n || r.neo > 0n || r.gas > 0n || r.liqs > 0n; });
        // totals strip: chain-wide, summed from the same enumeration
        var totNeo = 0n, totGas = 0n, nLoans = 0;
        rows.forEach(function (r) { totNeo += r.neo; totGas += r.gas; if (r.debt > 0n) nLoans++; });
        FtbRpc.readInt(C.lend, 'totalBorrows', []).then(function (tb) {
          var e = $('lbLoaned'); if (e) e.textContent = fmt(tb) + ' FTB';
        }).catch(function () {});
        FtbRpc.readInt(C.lend, 'utilizationBps', []).then(function (u) {
          var e = $('lbUtil'); if (e) e.textContent = (Number(u) / 100).toFixed(1) + '%';
        }).catch(function () {});
        var ec = $('lbCollat'); if (ec) ec.textContent = grp(totNeo.toString()) + ' NEO + ' + fmt(totGas) + ' GAS';
        var en = $('lbCount'); if (en) en.textContent = nLoans + (rows.length > nLoans ? ' (+ ' + (rows.length - nLoans) + ' closed/collateral-only)' : '');
        body.innerHTML = '';
        if (!open.length) { body.textContent = 'no open positions — ' + rows.length + ' closed.'; return; }
        open.forEach(function (r) {
          var row = el('div', 'entry');
          row.style.padding = '8px 0';
          var idx = el('span', 'idx', r.bad ? '!!' : '//');
          var mid = el('div');
          var mine = FtbState.account && r.addr === FtbState.account;
          var ttl = el('span', 'ttl', r.addr.slice(0, 6) + '…' + r.addr.slice(-4) + (mine ? ' (you)' : ''));
          ttl.title = r.addr;
          mid.appendChild(ttl);
          var meta = el('div', 'meta');
          var closedByLiq = r.debt === 0n && r.neo === 0n && r.gas === 0n && r.liqs > 0n;
          if (closedByLiq) {
            meta.appendChild(el('span', null, 'position closed by liquidation'));
          } else {
            meta.appendChild(el('span', null, 'debt ' + fmt(r.debt) + ' FTB'));
            meta.appendChild(el('span', null, grp(r.neo.toString()) + ' NEO'));
            meta.appendChild(el('span', null, fmt(r.gas) + ' GAS'));
          }
          if (mine) {
            var head = r.maxB - r.debt; if (head < 0n) head = 0n;
            // maxB is collateral × 60%, so ltv% = debt / maxB × 60
            var ltv = r.maxB > 0n ? (Number(r.debt * 10000n / r.maxB) * 0.006).toFixed(1) : '0.0';
            meta.appendChild(el('span', null, 'ltv ' + ltv + '% of 60% · headroom ' + fmt(head) + ' FTB'));
          }
          // the health slot carries four states: liquidatable (actionable NOW) wins,
          // then a liquidation mark that never fades, then the live states.
          var tag = r.bad
            ? el('span', 'tag pub', 'LIQUIDATABLE')
            : (r.liqs > 0n
              ? el('span', 'tag pub', 'liquidated ×' + r.liqs.toString())
              : (r.debt > 0n ? el('span', 'tag', 'healthy') : el('span', 'tag', 'collateral only — claimable')));
          meta.appendChild(tag);
          var btnWrap = el('div', 'read');
          if (mine && !r.bad) {
            // your own position is actionable straight from the board: draw a loan
            // against collateral already locked — no wizard restart (the deposit
            // is a fact on-chain; the borrow should be one click from seeing it).
            var bk = el('button', 'linkish', 'take loan ->');
            bk.addEventListener('click', wizBorrowDirect);
            btnWrap.appendChild(bk);
          }
          if (r.bad) {
            var lb = el('button', 'linkish', 'liquidate ->');
            lb.addEventListener('click', function () {
              if (!needAcct()) return;
              var plan = FtbState.dapi.liquidate(FtbState.account, r.debt.toString(), r.be);
              FtbShell.showPlan('liquidate ' + r.addr.slice(0, 6) + '…' + r.addr.slice(-4),
                'you repay up to ' + fmt(r.debt) + ' FTB of their debt and seize collateral at an 8% bonus. the pool stays solvent; the borrower keeps any excess collateral.',
                [['borrower', r.addr], ['debt covered', fmt(r.debt) + ' FTB'], ['what you sign', plan.summary]],
                plan);
            });
            btnWrap.appendChild(lb);
          }
          row.appendChild(idx); row.appendChild(mid); row.appendChild(btnWrap);
          mid.appendChild(meta);
          body.appendChild(row);
        });
        body.appendChild(el('div', 'small muted', open.length + ' open · ' + (rows.length - open.length) + ' closed — enumerated from contract storage, not a server.'));
        if (boardTrunc) body.appendChild(el('div', 'small muted', 'board truncated at ' + (MAX_PAGES * 100) + ' entries per prefix — point the page at your own node (OWN NODE) for the full enumeration.'));
      });
    }).catch(function (e) { body.textContent = 'board unavailable: ' + FtbUtil.errText(e); });
  }

  function flash(b) {
    if (!b) return;
    var t = b.textContent; b.textContent = 'reading…'; b.disabled = true;
    setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500);
  }

  // every entry point goes through the coalesced pass — manual buttons included:
  // a click during an in-flight pass queues exactly one trailing run.
  var loadLendC = FtbUtil.coalesce(loadLend, 3000), loadLoansC = FtbUtil.coalesce(loadLoans, 3000);
  var lb = $('lendRefresh'); if (lb) lb.addEventListener('click', function () { flash(lb); loadLendC(); });
  var lr = $('loansRefresh'); if (lr) lr.addEventListener('click', function () { flash(lr); loadLoansC(); });
  FtbState.on('refresh', loadLoansC);
  FtbState.on('rpc', loadLoansC);
  FtbState.on('rpc', loadLendC);
  FtbState.on('account', loadLendC);
  FtbState.on('refresh', loadLendC);
  // a confirmed write means the wizard's job is done — tear it down so no stale
  // amounts linger on screen looking like the transaction never happened.
  FtbState.on('refresh', clearWiz);
  // keep the pool fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadLendC(); }, 60000);
})();
