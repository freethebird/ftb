// mod-earn.js — module 3: FTB dividends (MODULES.md).
// The holders' 2/9 of every fee: monthly revenue (this month vs last month, from
// the v2 on-chain ledger), lifetime received, the sampled APY, and the stake
// board — global pool, my stake, my % share, my claimable, with stake / unstake /
// claim each on its own line.
//
// Owns (writes only these): #earn card — #dMonth, #dLast, #dLife, #dApy, the fee-flow
// strip (#feeFlows: #fLp, #fStake, #fTres), the stake
// board (#dTotal, #dStaked, #dShare, #dPending), #earnPos (#dvStakeAmt, #dvStakeGo,
// #dvUnstakeAmt, #dvUnstakeGo, #dvClaimGo, #dvHint), #earnOff, #earnRefresh.
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
    // fee-flow strip (the constitution, live): 2/3 stays in the lend pool (its apy
    // is the lp leg's figure), 2/9 sweeps here (same sampled apy as #dApy), 1/9
    // sits in the treasury until a passed, auditor-attested proposal spends it.
    if (C.lend && C.lend.indexOf('0x0000') !== 0) {
      FtbRpc.readInt(C.lend, 'supplyApyBps').then(function (v) {
        $('fLp').textContent = 'lp apy ' + (Number(v) / 100).toFixed(2) + '%';
      }).catch(function () {});
    }
    if (C.treasury && C.treasury.indexOf('0x0000') !== 0) {
      FtbRpc.readInt(C.ftb, 'balanceOf', [{ type: 'Hash160', value: C.treasury }]).then(function (v) {
        $('fTres').textContent = fmt(v) + ' FTB held';
      }).catch(function () {});
    }
    FtbRpc.readInt(dv, 'totalStaked').then(function (v) { $('dTotal').textContent = fmt(v) + ' FTB'; }).catch(function () {});
    FtbRpc.readInt(dv, 'accPerShare').then(function (v) {
      // dividend APY: sample the accumulator, compare with a sample at least 20h old
      try {
        var now = Date.now(), prev = JSON.parse(localStorage.getItem('ftb_div_acc') || 'null');
        if (prev && prev.v && (now - prev.t) > 20 * 3600 * 1000) {
          var perDay = (Number(v) - Number(prev.v)) / 1e18 / ((now - prev.t) / (24 * 3600 * 1000));
          if (perDay >= 0) { var apy = (perDay * 365 * 100).toFixed(2) + '%'; $('dApy').textContent = apy; $('fStake').textContent = 'stake apy ' + apy; }
          localStorage.setItem('ftb_div_acc', JSON.stringify({ v: v.toString(), t: now }));
        } else if (!prev) {
          localStorage.setItem('ftb_div_acc', JSON.stringify({ v: v.toString(), t: now }));
          $('dApy').textContent = 'sampling — check back tomorrow';
          $('fStake').textContent = 'stake apy sampling — check back tomorrow';
        }
      } catch (e) {}
    }).catch(function () {});
    var acct = FtbState.acctHash160();
    if (!acct) {
      // disconnected: my-side figures are absent, never stale (R5)
      var p0 = $('earnPos'); if (p0) p0.hidden = true;
      $('dStaked').textContent = '—'; $('dShare').textContent = '—'; $('dPending').textContent = '—';
      return;
    }
    var pos = $('earnPos'); if (pos) pos.hidden = false;
    var hu = [{ type: 'Hash160', value: acct }];
    Promise.all([
      FtbRpc.readInt(dv, 'stakedOf', hu),
      FtbRpc.readInt(dv, 'pendingOf', hu),
      FtbRpc.readInt(dv, 'totalStaked', [])
    ]).then(function (vs) {
      $('dStaked').textContent = fmt(vs[0]) + ' FTB';
      $('dPending').textContent = fmt(vs[1]) + ' FTB';
      // my share of the pool = my stake ÷ total staked (chain numbers, one pass)
      $('dShare').textContent = vs[2] > 0n ? (Number(vs[0] * 10000n / vs[2]) / 100).toFixed(2) + '%' : '—';
    }).catch(function () {});
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
  // every load goes through the shared coalescer — a signed stake bursts 'refresh'
  // events, and without this a slower stale pass can paint over the fresh one.
  var loadEarnC = FtbUtil.coalesce(loadEarn, 3000);
  var eb = $('earnRefresh'); if (eb) eb.addEventListener('click', function () { flash(eb); loadEarnC(); });
  FtbState.on('rpc', loadEarnC);
  FtbState.on('account', loadEarnC);
  FtbState.on('refresh', loadEarnC);
  // keep the panel fresh while the page is open
  setInterval(function () { if (FtbRpc.current()) loadEarnC(); }, 60000);
})();
