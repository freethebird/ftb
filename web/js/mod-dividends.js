// mod-dividends.js - DIVIDENDS section on the claim page (claim.html). Split out of the
// former #earn card. The holders' share of every fee: monthly revenue (this month vs
// last, from the v2 on-chain ledger), lifetime received, the sampled APY, the fee split,
// your claimable, and the claim action. Staking (stake/unstake) lives on the stake page
// (mod-stake.js). Read-only except the claim plan, which is review-and-sign.
//
// Owns (writes only these): #divWrap - #dApy, #dMonth, #dLast, #dLife, #dPending, the
// fee-flow strip (#fStake, #fTres; the LP leg is descriptive - no pooled-lend APY exists
// under the oracleless P2P model), #divPos (#dvClaimGo, #dvHint), #divOff, #divRefresh.
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('divWrap')) return;
  if (!window.FtbState || !window.FtbRpc) return;

  var $ = FtbUtil.$, fmt = FtbUtil.fmt;
  var C = FtbState.C;

  function divReady() { return !!(C.dividends && C.dividends.indexOf('0x0000') !== 0); }
  function notWired() {
    FtbShell.showPlan('dividends not wired', 'the dividends module hash is a placeholder in this deployment. the panel goes live when the hash lands in config.js.',
      [['status', 'dividends hash not configured']], null);
  }

  function load() {
    var dv = C.dividends;
    if (!divReady() || !FtbRpc.current()) { var o0 = $('divOff'); if (o0) o0.hidden = false; return; }
    var off = $('divOff'); if (off) off.hidden = true;
    FtbRpc.readInt(dv, 'monthIndex').then(function (mi) {
      Promise.all([
        FtbRpc.readInt(dv, 'monthTotal', [{ type: 'Integer', value: mi.toString() }]),
        mi > 0n ? FtbRpc.readInt(dv, 'monthTotal', [{ type: 'Integer', value: (mi - 1n).toString() }]) : Promise.resolve(0n)
      ]).then(function (ms) {
        var a = $('dMonth'); if (a) a.textContent = fmt(ms[0]) + ' FTB';
        var b = $('dLast'); if (b) b.textContent = fmt(ms[1]) + ' FTB';
      }).catch(function () {});
    }).catch(function () {});
    FtbRpc.readInt(dv, 'totalReceived').then(function (v) { var n = $('dLife'); if (n) n.textContent = fmt(v) + ' FTB'; }).catch(function () {});
    if (C.treasury && C.treasury.indexOf('0x0000') !== 0) {
      FtbRpc.readInt(C.ftb, 'balanceOf', [{ type: 'Hash160', value: C.treasury }]).then(function (v) { var n = $('fTres'); if (n) n.textContent = fmt(v) + ' FTB held'; }).catch(function () {});
    }
    FtbRpc.readInt(dv, 'accPerShare').then(function (v) {
      try {
        var now = Date.now(), prev = JSON.parse(localStorage.getItem('ftb_div_acc') || 'null');
        if (prev && prev.v && (now - prev.t) > 20 * 3600 * 1000) {
          var perDay = (Number(v) - Number(prev.v)) / 1e18 / ((now - prev.t) / (24 * 3600 * 1000));
          if (perDay >= 0) { var apy = (perDay * 365 * 100).toFixed(2) + '%'; setTxt('dApy', apy); setTxt('fStake', 'stake apy ' + apy); }
          localStorage.setItem('ftb_div_acc', JSON.stringify({ v: v.toString(), t: now }));
        } else if (!prev) {
          localStorage.setItem('ftb_div_acc', JSON.stringify({ v: v.toString(), t: now }));
          setTxt('dApy', 'sampling - check back tomorrow');
          setTxt('fStake', 'stake apy sampling - check back tomorrow');
        }
      } catch (e) {}
    }).catch(function () {});
    var acct = FtbState.acctHash160();
    if (!acct) { var p0 = $('divPos'); if (p0) p0.hidden = true; setTxt('dPending', '-'); return; }
    var pos = $('divPos'); if (pos) pos.hidden = false;
    FtbRpc.readInt(dv, 'pendingOf', [{ type: 'Hash160', value: acct }]).then(function (v) { setTxt('dPending', fmt(v) + ' FTB'); }).catch(function () {});
  }
  function setTxt(id, t) { var n = $(id); if (n) n.textContent = t; }

  if ($('dvClaimGo')) {
    $('dvClaimGo').addEventListener('click', function () {
      if (!divReady()) { notWired(); return; }
      if (!FtbState.account) { FtbWallet.needConnect(); return; }
      var plan = FtbState.dapi.claimDividends(FtbState.account);
      FtbShell.showPlan('claim dividends',
        'pays out everything your stake has earned up to this block.',
        [['what you sign', plan.summary]], plan);
    });
  }

  function flash(b) { if (!b) return; var t = b.textContent; b.textContent = 'reading…'; b.disabled = true; setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500); }
  var loadC = FtbUtil.coalesce(load, 3000);
  var rb = $('divRefresh'); if (rb) rb.addEventListener('click', function () { flash(rb); loadC(); });
  FtbState.on('rpc', loadC);
  FtbState.on('account', loadC);
  FtbState.on('refresh', loadC);
  setInterval(function () { if (FtbRpc.current()) loadC(); }, 60000);
})();
