// mod-stake.js - STAKE page (stake.html). Split out of the former #earn card.
// Staked FTB earns the holders' share of every fee (claimed on the claim page). This
// page owns only the position: global pool, your stake, your % share, and stake /
// unstake. Dividends revenue, APY and the claim action live on the claim page
// (mod-dividends.js). Read-only for the pool; every action is a review-and-sign plan.
//
// Owns (writes only these): #stakeWrap - #stTotal, #stMine, #stShare, #stPos
// (#stStakeAmt/#stStakeGo, #stUnstakeAmt/#stUnstakeGo, #stHint), #stOff, #stRefresh.
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('stakeWrap')) return;
  if (!window.FtbState || !window.FtbRpc) return;

  var $ = FtbUtil.$, fmt = FtbUtil.fmt;
  var C = FtbState.C;

  function divReady() { return !!(C.dividends && C.dividends.indexOf('0x0000') !== 0); }
  function notWired() {
    FtbShell.showPlan('staking not wired', 'the dividends/stake module hash is a placeholder in this deployment. staking goes live when the hash lands in config.js.',
      [['status', 'dividends hash not configured']], null);
  }
  function amtOf(id) {
    var v = (($(id) && $(id).value) || '').trim();
    if (!/^[0-9]+(\.[0-9]+)?$/.test(v) || parseFloat(v) <= 0) return null;
    return v;
  }

  function load() {
    var dv = C.dividends;
    if (!divReady() || !FtbRpc.current()) { var o0 = $('stOff'); if (o0) o0.hidden = false; return; }
    var off = $('stOff'); if (off) off.hidden = true;
    FtbRpc.readInt(dv, 'totalStaked').then(function (v) { var n = $('stTotal'); if (n) n.textContent = fmt(v) + ' FTB'; }).catch(function () {});
    var acct = FtbState.acctHash160();
    if (!acct) {
      var p0 = $('stPos'); if (p0) p0.hidden = true;
      ['stMine', 'stShare'].forEach(function (id) { var n = $(id); if (n) n.textContent = '-'; });
      return;
    }
    var pos = $('stPos'); if (pos) pos.hidden = false;
    var hu = [{ type: 'Hash160', value: acct }];
    Promise.all([
      FtbRpc.readInt(dv, 'stakedOf', hu),
      FtbRpc.readInt(dv, 'totalStaked', [])
    ]).then(function (vs) {
      var m = $('stMine'); if (m) m.textContent = fmt(vs[0]) + ' FTB';
      var s = $('stShare'); if (s) s.textContent = vs[1] > 0n ? (Number(vs[0] * 10000n / vs[1]) / 100).toFixed(2) + '%' : '-';
    }).catch(function () {});
  }

  if ($('stStakeGo')) {
    $('stStakeGo').addEventListener('click', function () {
      if (!divReady()) { notWired(); return; }
      if (!FtbState.account) { FtbWallet.needConnect(); return; }
      var amt = amtOf('stStakeAmt'); if (!amt) { setHint('enter an amount of FTB to stake'); return; }
      var plan = FtbState.dapi.stakeFtb(FtbState.account, FtbDapi.util.toBaseUnits(amt));
      FtbShell.showPlan('stake ' + amt + ' FTB',
        'staked FTB earns the holders\' share of every fee. unstake any time; dividends already earned stay claimable on the claim page.',
        [['you stake', amt + ' FTB'],
         ['earns', 'pro-rata share of every sweep into the dividends module'],
         ['lockup', 'none - unstake any time'],
         ['what you sign', plan.summary]],
        plan);
    });
  }
  if ($('stUnstakeGo')) {
    $('stUnstakeGo').addEventListener('click', function () {
      if (!divReady()) { notWired(); return; }
      if (!FtbState.account) { FtbWallet.needConnect(); return; }
      var amt = amtOf('stUnstakeAmt'); if (!amt) { setHint('enter an amount of FTB to unstake'); return; }
      var plan = FtbState.dapi.unstakeFtb(FtbState.account, FtbDapi.util.toBaseUnits(amt));
      FtbShell.showPlan('unstake ' + amt + ' FTB',
        'principal returns to your wallet. dividends earned up to this block stay claimable on the claim page.',
        [['you unstake', amt + ' FTB'],
         ['dividends', 'unaffected - claim whenever you like'],
         ['what you sign', plan.summary]],
        plan);
    });
  }
  function setHint(t) { var h = $('stHint'); if (h) h.textContent = t; }

  function flash(b) { if (!b) return; var t = b.textContent; b.textContent = 'reading…'; b.disabled = true; setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500); }
  var loadC = FtbUtil.coalesce(load, 3000);
  FtbState.on('refresh', function () { var s = $('stStakeAmt'), u = $('stUnstakeAmt'); if (s) s.value = ''; if (u) u.value = ''; });
  var rb = $('stRefresh'); if (rb) rb.addEventListener('click', function () { flash(rb); loadC(); });
  FtbState.on('rpc', loadC);
  FtbState.on('account', loadC);
  FtbState.on('refresh', loadC);
  setInterval(function () { if (FtbRpc.current()) loadC(); }, 60000);
})();
