// mod-earn.js — module 3: FTB dividends (MODULES.md).
// The holders' 2/9 of every fee: monthly revenue (this month vs last month, from
// the v2 on-chain ledger), lifetime received, stake, unstake, claim, APY sampler.
//
// Owns (writes only these): #earn card — #dMonth, #dLast, #dLife, #dApy, #dTotal,
// #dPending, #earnPos (#dStaked, #dvStakeAmt, #dvStakeGo, #dvUnstakeAmt,
// #dvUnstakeGo, #dvClaimGo, #dvHint), #earnOff, #earnRefresh.
(function () {
  'use strict';
  if (!FtbUtil.$('earn')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, fmt = FtbUtil.fmt;
  var C = FtbState.C;

  function divReady() { return !!(C.dividends && C.dividends.indexOf('0x0000') !== 0); }
  function divNotWired() {
    FtbShell.showPlan('dividends not wired', 'the dividends module hash is a placeholder in this deployment. the panel goes live when the TestNet hash lands in config.js.',
      [['status', 'dividends hash not configured']], null);
  }
  function dvAmount(id) {
    var v = ($(id).value || '').trim();
    if (!/^[0-9]+(\.[0-9]+)?$/.test(v) || parseFloat(v) <= 0) return null;
    return v;
  }

  function loadEarn() {
    var dv = C.dividends;
    if (!divReady() || !FtbRpc.current()) { var off0 = $('earnOff'); if (off0) off0.hidden = false; return; }
    var off = $('earnOff'); if (off) off.hidden = true;
    // monthly revenue ledger (v2): dividends GENERATED this month vs last month,
    // counted at receipt — no indexer, straight from contract storage.
    FtbRpc.readInt(dv, 'monthIndex').then(function (mi) {
      Promise.all([
        FtbRpc.readInt(dv, 'monthTotal', [{ type: 'Integer', value: mi.toString() }]),
        mi > 0n ? FtbRpc.readInt(dv, 'monthTotal', [{ type: 'Integer', value: (mi - 1n).toString() }]) : Promise.resolve(0n)
      ]).then(function (ms) {
        $('dMonth').textContent = fmt(ms[0]) + ' FTB';
        $('dLast').textContent = fmt(ms[1]) + ' FTB';
      }).catch(function () {});
    }).catch(function () {});
    FtbRpc.readInt(dv, 'totalReceived').then(function (v) { $('dLife').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(dv, 'totalStaked').then(function (v) { $('dTotal').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(dv, 'accPerShare').then(function (v) {
      // dividend APY: sample the accumulator, compare with a sample at least 20h old
      try {
        var now = Date.now(), prev = JSON.parse(localStorage.getItem('ftb_div_acc') || 'null');
        if (prev && prev.v && (now - prev.t) > 20 * 3600 * 1000) {
          var perDay = (Number(v) - Number(prev.v)) / 1e18 / ((now - prev.t) / (24 * 3600 * 1000));
          if (perDay >= 0) $('dApy').textContent = (perDay * 365 * 100).toFixed(2) + '%';
          localStorage.setItem('ftb_div_acc', JSON.stringify({ v: v.toString(), t: now }));
        } else if (!prev) {
          localStorage.setItem('ftb_div_acc', JSON.stringify({ v: v.toString(), t: now }));
          $('dApy').textContent = 'sampling — check back tomorrow';
        }
      } catch (e) {}
    }).catch(function () {});
    var acct = FtbState.acctHash160();
    if (!acct) return;
    var pos = $('earnPos'); if (pos) pos.hidden = false;
    var hu = [{ type: 'Hash160', value: acct }];
    FtbRpc.readInt(dv, 'stakedOf', hu).then(function (v) { $('dStaked').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(dv, 'pendingOf', hu).then(function (v) { $('dPending').textContent = fmt(v) + ' FTB'; }).catch(function () {});
  }

  if ($('dvStakeAmt')) {
    $('dvStakeGo').addEventListener('click', function () {
      if (!divReady()) { divNotWired(); return; }
      if (!FtbState.account) { FtbWallet.needConnect(); return; }
      var amt = dvAmount('dvStakeAmt');
      if (!amt) { $('dvHint').textContent = 'enter an amount of FTB to stake'; return; }
      var plan = FtbState.dapi.stakeFtb(FtbState.account, FtbDapi.util.toBaseUnits(amt));
      FtbShell.showPlan('stake ' + amt + ' FTB',
        'staked FTB earns the holders\' 2/9 of every fee. unstake any time; dividends already earned stay claimable.',
        [['you stake', amt + ' FTB'],
         ['earns', 'pro-rata share of every sweep into the dividends module'],
         ['lockup', 'none — unstake any time'],
         ['what you sign', plan.summary]],
        plan);
    });
    $('dvUnstakeGo').addEventListener('click', function () {
      if (!divReady()) { divNotWired(); return; }
      if (!FtbState.account) { FtbWallet.needConnect(); return; }
      var amt = dvAmount('dvUnstakeAmt');
      if (!amt) { $('dvHint').textContent = 'enter an amount of FTB to unstake'; return; }
      var plan = FtbState.dapi.unstakeFtb(FtbState.account, FtbDapi.util.toBaseUnits(amt));
      FtbShell.showPlan('unstake ' + amt + ' FTB',
        'principal returns to your wallet. dividends earned up to this block stay claimable.',
        [['you unstake', amt + ' FTB'],
         ['dividends', 'unaffected — claim whenever you like'],
         ['what you sign', plan.summary]],
        plan);
    });
    $('dvClaimGo').addEventListener('click', function () {
      if (!divReady()) { divNotWired(); return; }
      if (!FtbState.account) { FtbWallet.needConnect(); return; }
      var plan = FtbState.dapi.claimDividends(FtbState.account);
      FtbShell.showPlan('claim dividends',
        'pays out everything your stake has earned up to this block.',
        [['what you sign', plan.summary]],
        plan);
    });
  }


  function flash(b) {
    if (!b) return;
    var t = b.textContent; b.textContent = 'reading…'; b.disabled = true;
    setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500);
  }
  var eb = $('earnRefresh'); if (eb) eb.addEventListener('click', function () { flash(eb); loadEarn(); });
  FtbState.on('rpc', loadEarn);
  FtbState.on('account', loadEarn);
  FtbState.on('refresh', loadEarn);
  // keep the panel fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadEarn(); }, 60000);
})();
