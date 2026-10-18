// mod-wallet.js — module 9: wallet connect (MODULES.md).
// The dApi adapter lifecycle: connect, account-changed, disconnect. The page
// never holds keys; every write is review-and-sign in the wallet.
//
// Owns (writes only these): #connectBtn, #disconnectBtn, #idstate. Publishes
// FtbState.account and FtbState.dapi, and emits 'account' whenever the wallet
// state changes.
(function () {
  'use strict';
  if (!FtbUtil.$('connectBtn')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, short = FtbUtil.short;
  var CFG = FtbState.CFG;
  var NET = CFG.network || 'TestNet';

  function setState() {
    var account = FtbState.account;
    $('idstate').textContent = account ? ('[ ' + short(account) + ' @ ' + NET + ' ]') : '[ no wallet ]';
    $('connectBtn').textContent = account ? 'connected' : 'connect ->';
    var d = $('disconnectBtn'); if (d) d.hidden = !account;
  }

  $('connectBtn').addEventListener('click', function () {
    if (FtbState.account || !FtbState.dapi) return;
    FtbState.dapi.connect().then(function () {
      FtbState.account = FtbState.dapi.account;
      setState();
      FtbState.emit('account', FtbState.account);
    }).catch(function (e) {
      $('idstate').textContent = '[ ' + (e.message || 'wallet error') + ' ]';
    });
  });

  // UI-level detach: NeoLine has no programmatic disconnect, so this clears the
  // page's account state only — the wallet itself stays connected to the site
  // until the user revokes it in NeoLine.
  if ($('disconnectBtn')) $('disconnectBtn').addEventListener('click', function () {
    FtbState.account = null;
    if (FtbState.dapi) FtbState.dapi.account = null;
    setState();
    $('idstate').textContent = '[ detached — revoke in NeoLine to fully disconnect ]';
    FtbState.emit('account', null);
  });

  if (typeof FtbDapi !== 'undefined' && FtbState.configured) {
    var dapi = new FtbDapi(Object.assign({}, CFG));
    FtbState.dapi = dapi;
    dapi.init().catch(function () { $('idstate').textContent = '[ install NeoLine or a dApi wallet ]'; });
    dapi.on && dapi.on('accountChanged', function (a) {
      FtbState.account = a && a.address ? a.address : null;
      if (FtbState.dapi) FtbState.dapi.account = FtbState.account;
      setState();
      FtbState.emit('account', FtbState.account);
    });
  } else {
    $('idstate').textContent = FtbState.configured ? '[ adapter missing ]' : '[ hashes not configured ]';
  }
  // the seam other modules use to nudge the user toward connecting — they never
  // write #idstate directly (R1: one writer per element).
  window.FtbWallet = {
    needConnect: function () { $('idstate').textContent = '[ connect first ]'; }
  };

  setState();
})();
