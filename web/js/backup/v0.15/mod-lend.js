// mod-lend.js - module 4: LEND (MODULES.md) - FtbLoanBook peer-to-peer term loans.
// Oracleless, keyless, immutable: a lender escrows FTB into an OFFER naming the
// collateral (NEO or GAS), its amount, a flat term rate and a term; a borrower TAKES it
// by locking exactly that collateral and receives the FTB; repay principal+interest
// before maturity to unlock the collateral; an unrepaid loan is CLOSED by anyone after
// maturity and the collateral goes to the lender. No oracle, no liquidation price, no
// margin call, no owner. The page is read-only for the book; every value move is a
// review-and-sign plan through the adapter (FtbShell.showPlan).
//
// Owns (writes only these): #lend card - the stat strip (#lnOpen/#lnActive/#lnOffered/
// #lnProceeds), #lendAct (the keys #lnWizLend/#lnWizWithdraw and the #lnWizard panel they
// build), the offer book (#loansBody, #loansRefresh), #lendOff, #lendRefresh. Every amount
// is base units on the wire: FTB and GAS 8dp, NEO whole. rateBps = flat % x 100; termMs ms.
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('lend')) return; // R2: inert where the markup is absent
  if (!window.FtbState || !window.FtbRpc) return;

  var $ = FtbUtil.$, el = FtbUtil.el, grp = FtbUtil.grp, short = FtbUtil.short;
  var C = FtbState.C, CFG = FtbState.CFG || {}, AS = CFG.assets || {}, U = (window.FtbDapi && FtbDapi.util) || null;
  var wiz = $('lnWizard');

  var ST = {
    1: { t: 'open', c: 's-open' }, 2: { t: 'active', c: 's-active' },
    3: { t: 'repaid', c: 's-done' }, 4: { t: 'defaulted', c: 's-default' },
    5: { t: 'cancelled', c: 's-done' }
  };

  function live() { return C.loanbook && C.loanbook.indexOf('0x0000') !== 0 && FtbRpc.current(); }
  function iarg(n) { return { type: 'Integer', value: String(n) }; }
  function needAcct() { if (FtbState.account) return true; if (window.FtbWallet && FtbWallet.needConnect) FtbWallet.needConnect(); return false; }

  // ---- unit helpers: 8dp for FTB/GAS, whole units for NEO ----
  function toBase(dec, decimals) {
    dec = String(dec == null ? '' : dec).trim();
    if (!/^\d+(\.\d+)?$/.test(dec)) return null;
    var p = dec.split('.'), whole = p[0] || '0', frac = p[1] || '';
    if (decimals === 0) { if (/[1-9]/.test(frac)) return null; return BigInt(whole); }
    frac = (frac + '00000000').slice(0, decimals);
    return BigInt(whole) * (10n ** BigInt(decimals)) + BigInt(frac || '0');
  }
  function fromBase(big, decimals) {
    big = BigInt(big);
    if (decimals === 0) return big.toString();
    var d = 10n ** BigInt(decimals), w = big / d, f = big % d;
    var fs = f.toString().padStart(decimals, '0').replace(/0+$/, '');
    return fs ? (w.toString() + '.' + fs) : w.toString();
  }
  function amt(big, decimals) { return grp(fromBase(big, decimals)); }

  function tokenOf(beLower) {
    if (beLower === String(AS.neo).toLowerCase()) return { sym: 'NEO', dec: 0, hash: AS.neo };
    if (beLower === String(AS.gas).toLowerCase()) return { sym: 'GAS', dec: 8, hash: AS.gas };
    return { sym: '?', dec: 0, hash: beLower };
  }

  function decodeOffer(id, arr) {
    function int(i) { var x = arr[i]; return (x && x.value != null) ? BigInt(x.value) : 0n; }
    function party(i) {
      var x = arr[i];
      if (!x || x.value == null || x.value === '') return null;
      if (!U) return { le: '', be: '' };
      var leHex = U.b64ToHex(x.value);
      return { le: '0x' + leHex, be: '0x' + U.reverseHex(leHex) };
    }
    var colTok = party(2), tok = colTok ? tokenOf(colTok.be.toLowerCase()) : { sym: '?', dec: 0 };
    return {
      id: id, funder: party(0), borrower: party(1), colTok: colTok, tok: tok,
      colAmt: int(3), principal: int(4), rate: int(5), term: int(6),
      maturity: int(7), repay: int(8), state: Number(int(9))
    };
  }

  function rateStr(bps) { return (Number(bps) / 100) + '%'; }
  function termStr(ms) {
    var d = Number(ms) / 86400000;
    if (d >= 1) return (Math.round(d * 10) / 10) + ' d';
    var h = Number(ms) / 3600000; if (h >= 1) return (Math.round(h * 10) / 10) + ' h';
    return Math.round(Number(ms) / 60000) + ' min';
  }
  function dueStr(maturityMs) {
    var left = Number(maturityMs) - Date.now();
    if (left <= 0) return 'matured';
    var d = left / 86400000; if (d >= 1) return 'due in ' + Math.round(d) + ' d';
    var h = left / 3600000; if (h >= 1) return 'due in ' + Math.round(h) + ' h';
    return 'due in ' + Math.max(1, Math.round(left / 60000)) + ' min';
  }
  function mine(party) { return !!(party && FtbState.acctHash160() && party.le === FtbState.acctHash160()); }

  // ---------- read + render the book ----------
  function loadBook() {
    var off = $('lendOff');
    if (!live()) { if (off) off.hidden = false; setStat('-'); if ($('loansBody')) $('loansBody').textContent = 'standby'; return; }
    if (off) off.hidden = true;
    FtbRpc.readInt(C.loanbook, 'offerCount', []).then(function (cntB) {
      var n = Number(cntB), CAP = 300, start = Math.max(0, n - CAP), ids = [];
      for (var i = n - 1; i >= start; i--) ids.push(i);
      return Promise.all(ids.map(function (id) {
        return FtbRpc.read(C.loanbook, 'offer', [iarg(id)])
          .then(function (st) { return (st && st[0] && st[0].value) ? decodeOffer(id, st[0].value) : null; })
          .catch(function () { return null; });
      })).then(function (list) { render(list.filter(function (o) { return o && o.state >= 1; }), n > CAP); });
    }).catch(function (e) { if ($('loansBody')) $('loansBody').textContent = 'book unavailable: ' + FtbUtil.errText(e); });
    loadProceeds();
  }

  function setStat(v) { ['lnOpen', 'lnActive', 'lnOffered', 'lnProceeds'].forEach(function (id) { var n = $(id); if (n) n.textContent = v; }); }

  function loadProceeds() {
    var p = $('lnProceeds'); if (!p) return;
    if (!FtbState.account) { p.textContent = '-'; return; }
    FtbRpc.readInt(C.loanbook, 'claimable', [{ type: 'Hash160', value: FtbState.acctHash160() }, { type: 'Hash160', value: C.ftb }])
      .then(function (v) { p.textContent = amt(v, 8) + ' FTB'; }).catch(function () { p.textContent = '-'; });
  }

  function render(offers, trunc) {
    var open = 0, active = 0, offered = 0n;
    offers.forEach(function (o) {
      if (o.state === 1) { open++; offered += o.principal; }
      else if (o.state === 2) active++;
    });
    if ($('lnOpen')) $('lnOpen').textContent = String(open);
    if ($('lnActive')) $('lnActive').textContent = String(active);
    if ($('lnOffered')) $('lnOffered').textContent = amt(offered, 8) + ' FTB';
    loadProceeds();

    var body = $('loansBody'); if (!body) return;
    body.replaceChildren();
    if (!offers.length) { body.appendChild(el('div', 'small muted', 'no offers yet - be the first to lend (LEND FTB).')); return; }

    offers.forEach(function (o) {
      var row = el('div', 'lnrow');
      var meta = el('div', 'lnmeta small');
      var st = el('span', 'lnst ' + (ST[o.state] ? ST[o.state].c : ''), ST[o.state] ? ST[o.state].t : '?');
      meta.appendChild(el('span', 'lnid', '#' + o.id + ' '));
      meta.appendChild(st);
      var line = '  ' + amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym + ' collateral  ·  ' +
        amt(o.principal, 8) + ' FTB  ·  ' + rateStr(o.rate) + '  ·  ' + termStr(o.term);
      if (o.state === 2) line += '  ·  ' + dueStr(o.maturity) + '  ·  repay ' + amt(o.repay, 8) + ' FTB';
      meta.appendChild(el('span', 'lntxt', line));
      row.appendChild(meta);

      var acts = el('div', 'row lnacts');
      if (o.state === 1) {
        acts.appendChild(keyBtn('take →', function () { planTake(o); }));
        if (mine(o.funder)) acts.appendChild(keyBtn('cancel →', function () { planCancel(o); }));
      } else if (o.state === 2) {
        if (mine(o.borrower)) acts.appendChild(keyBtn('repay →', function () { planRepay(o); }));
        if (Date.now() >= Number(o.maturity)) acts.appendChild(keyBtn('close →', function () { planClose(o); }));
      }
      if (acts.childNodes.length) row.appendChild(acts);
      body.appendChild(row);
    });
    body.appendChild(el('div', 'small muted', open + ' open · ' + active + ' active · ' + (offers.length - open - active) + ' settled - enumerated from contract storage, not a server.'));
    if (trunc) body.appendChild(el('div', 'small muted', 'book truncated at 300 newest offers - point the page at your own node for the full enumeration.'));
  }

  function keyBtn(label, fn) { var b = el('button', 'linkish', label); b.addEventListener('click', fn); return b; }

  // ---------- plans (review + sign) ----------
  function planTake(o) {
    if (!needAcct()) return;
    FtbShell.showPlan('take offer #' + o.id,
      'you lock ' + amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym + ' as collateral and receive ' + amt(o.principal, 8) +
      ' FTB. repay ' + amt(o.repay > 0n ? o.repay : (o.principal + o.principal * o.rate / 10000n), 8) +
      ' FTB before maturity to unlock your collateral; miss it and the collateral is the lender\'s.',
      [['you lock', amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym],
       ['you receive', amt(o.principal, 8) + ' FTB'],
       ['you will repay', amt(o.principal + o.principal * o.rate / 10000n, 8) + ' FTB'],
       ['term', termStr(o.term)],
       ['what you sign', o.tok.sym + '.transfer(you → loanbook, ' + amt(o.colAmt, o.tok.dec) + ', "take #' + o.id + '")']],
      FtbState.dapi.loanTake(FtbState.account, o.colTok.be, o.colAmt.toString(), o.id));
  }
  function planRepay(o) {
    if (!needAcct()) return;
    var due = o.repay > 0n ? o.repay : (o.principal + o.principal * o.rate / 10000n);
    FtbShell.showPlan('repay loan #' + o.id,
      'transfers ' + amt(due, 8) + ' FTB to clear the loan; your ' + amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym +
      ' collateral returns to you. any overpay is refunded by the contract.',
      [['you repay', amt(due, 8) + ' FTB'],
       ['unlocks', amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym],
       ['what you sign', 'FTB.transfer(you → loanbook, ' + amt(due, 8) + ', "repay #' + o.id + '")']],
      FtbState.dapi.loanRepay(FtbState.account, due.toString(), o.id));
  }
  function planClose(o) {
    if (!needAcct()) return;
    FtbShell.showPlan('close loan #' + o.id,
      'the term has elapsed and the loan is unrepaid. closing credits the ' + amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym +
      ' collateral to the lender\'s pull ledger. anyone may call this; it moves no tokens to you.',
      [['settles', 'loan #' + o.id],
       ['collateral → lender', amt(o.colAmt, o.tok.dec) + ' ' + o.tok.sym],
       ['what you sign', 'loanbook.close(' + o.id + ')']],
      FtbState.dapi.loanClose(o.id));
  }
  function planCancel(o) {
    if (!needAcct()) return;
    FtbShell.showPlan('cancel offer #' + o.id,
      'withdraws your un-taken offer and refunds the ' + amt(o.principal, 8) + ' FTB you escrowed.',
      [['refunds', amt(o.principal, 8) + ' FTB'],
       ['what you sign', 'loanbook.cancelOffer(' + o.id + ')']],
      FtbState.dapi.loanCancel(o.id));
  }

  // ---------- A) lend FTB (post an offer) ----------
  function wizLend() {
    if (!needAcct()) return;
    clearWiz();
    var box = el('div', 'advbody');
    box.appendChild(el('p', 'advnote', 'You escrow FTB and set the terms. A borrower who locks the collateral you name receives your FTB and must repay principal + interest before the term ends; if they do not, the collateral becomes yours. You set the price of the collateral - there is no oracle.'));
    var r1 = el('div', 'row');
    var tokSel = el('select'); tokSel.id = 'lnTok';
    [['NEO', 'NEO'], ['GAS', 'GAS']].forEach(function (p) { var op = el('option', null, p[0]); op.value = p[1]; tokSel.appendChild(op); });
    var cCol = mkIn('collateral amount');
    var cPrin = mkIn('FTB to lend');
    var cRate = mkIn('rate % (e.g. 5)');
    var cTerm = mkIn('term in days (e.g. 30)');
    r1.appendChild(labelled('collateral', tokSel));
    r1.appendChild(labelled('amount', cCol));
    r1.appendChild(labelled('lend FTB', cPrin));
    r1.appendChild(labelled('rate %', cRate));
    r1.appendChild(labelled('term (days)', cTerm));
    box.appendChild(r1);
    var note = el('p', 'advnote', '');
    box.appendChild(note);
    var keys = el('div', 'row');
    keys.appendChild(keyBtn('review offer →', function () {
      var tok = tokenOf(String(tokSel.value === 'GAS' ? AS.gas : AS.neo).toLowerCase());
      var colBase = toBase(cCol.value, tok.dec);
      var prinBase = toBase(cPrin.value, 8);
      var rateN = Number(cRate.value), daysN = Number(cTerm.value);
      if (colBase === null || colBase <= 0n) { note.textContent = 'enter a positive collateral amount' + (tok.dec === 0 ? ' (NEO is whole units)' : ''); return; }
      if (prinBase === null || prinBase <= 0n) { note.textContent = 'enter a positive FTB amount to lend'; return; }
      if (!(rateN >= 0) || rateN > 10000) { note.textContent = 'rate must be between 0% and 10,000%'; return; }
      if (!(daysN > 0) || daysN > 3650) { note.textContent = 'term must be between 1 and 3650 days'; return; }
      var rateBps = Math.round(rateN * 100), termMs = Math.round(daysN * 86400000);
      if (termMs < 60000) { note.textContent = 'term too short (minimum 1 minute)'; return; }
      var repay = prinBase + prinBase * BigInt(rateBps) / 10000n;
      FtbShell.showPlan('lend ' + fromBase(prinBase, 8) + ' FTB',
        'escrows your FTB into an open offer. a borrower who locks ' + fromBase(colBase, tok.dec) + ' ' + tok.sym +
        ' receives it and owes you ' + fromBase(repay, 8) + ' FTB by maturity. you can cancel any time before it is taken.',
        [['you lend', fromBase(prinBase, 8) + ' FTB'],
         ['collateral required', fromBase(colBase, tok.dec) + ' ' + tok.sym],
         ['interest', rateN + '% flat → ' + fromBase(repay, 8) + ' FTB repaid'],
         ['term', daysN + ' days'],
         ['what you sign', 'FTB.transfer(you → loanbook, ' + fromBase(prinBase, 8) + ', "offer")']],
        FtbState.dapi.loanPostOffer(FtbState.account, prinBase.toString(), tok.hash, colBase.toString(), rateBps, termMs));
    }));
    box.appendChild(keys);
    wiz.appendChild(box);
    cCol.focus();
  }

  // ---------- B) withdraw proceeds (pull ledger, per token) ----------
  function wizWithdraw() {
    if (!needAcct()) return;
    clearWiz();
    var box = el('div', 'advbody');
    box.appendChild(el('p', 'advnote', 'Repaid FTB and collateral from defaulted loans accrue to your pull ledger. Withdraw each token you are owed.'));
    var keys = el('div', 'row');
    var acct = { type: 'Hash160', value: FtbState.acctHash160() };
    [['FTB', C.ftb, 8], ['NEO', AS.neo, 0], ['GAS', AS.gas, 8]].forEach(function (t) {
      FtbRpc.readInt(C.loanbook, 'claimable', [acct, { type: 'Hash160', value: t[1] }]).then(function (v) {
        if (v <= 0n) return;
        keys.appendChild(keyBtn('withdraw ' + amt(v, t[2]) + ' ' + t[0] + ' →', function () {
          FtbShell.showPlan('withdraw ' + fromBase(v, t[2]) + ' ' + t[0],
            'pays out your accrued ' + t[0] + ' proceeds. the balance is zeroed before the send.',
            [['you receive', fromBase(v, t[2]) + ' ' + t[0]], ['what you sign', 'loanbook.withdraw(you, ' + t[0] + ')']],
            FtbState.dapi.loanWithdraw(FtbState.account, t[1]));
        }));
        if (keys.childNodes.length === 1) box.insertBefore(keys, null);
      }).catch(function () {});
    });
    setTimeout(function () { if (!keys.childNodes.length) box.appendChild(el('p', 'advnote', 'nothing to withdraw right now.')); }, 1200);
    box.appendChild(keys);
    wiz.appendChild(box);
  }

  // ---------- small UI helpers ----------
  function clearWiz() { if (wiz) wiz.replaceChildren(); }
  function mkIn(ph) { var i = el('input'); i.type = 'text'; i.autocomplete = 'off'; i.spellcheck = false; i.placeholder = ph || ''; return i; }
  function labelled(lbl, node) { var w = el('div', 'lnfield'); w.appendChild(el('span', 'lnlbl', lbl)); if (node && node.setAttribute) node.setAttribute('aria-label', lbl); w.appendChild(node); return w; }

  // ---------- wiring ----------
  function syncAct() { var a = $('lendAct'); if (a) a.hidden = !FtbState.account; }
  function flash(b) { if (!b) return; var t = b.textContent; b.textContent = 'reading…'; b.disabled = true; setTimeout(function () { b.textContent = t; b.disabled = false; }, 1200); }

  var loadC = FtbUtil.coalesce(loadBook, 3000);
  var lw = $('lnWizLend'); if (lw) lw.addEventListener('click', wizLend);
  var ww = $('lnWizWithdraw'); if (ww) ww.addEventListener('click', wizWithdraw);
  var lb = $('lendRefresh'); if (lb) lb.addEventListener('click', function () { flash(lb); loadC(); });
  var lr = $('loansRefresh'); if (lr) lr.addEventListener('click', function () { flash(lr); loadC(); });
  FtbState.on('rpc', loadC);
  FtbState.on('refresh', loadC);
  FtbState.on('account', loadC);
  FtbState.on('account', syncAct);
  FtbState.on('refresh', clearWiz);
  syncAct();
  setInterval(function () { if (FtbRpc.current()) loadC(); }, 60000);
})();
