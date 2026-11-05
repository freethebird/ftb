// mod-wallet.js - module 9: wallet connect (MODULES.md).
// The dApi adapter lifecycle: connect, account-changed, disconnect. The page
// never holds keys; every write is review-and-sign in the wallet.
//
// Owns (writes only these): #connectBtn, #disconnectBtn, #idstate. Publishes
// FtbState.account and FtbState.dapi, and emits 'account' whenever the wallet
// state changes.
//
// Session persistence: connecting once is remembered in this browser (a single
// flag - no address, no key). On every later page load the adapter does a
// SILENT re-attach: NeoLine returns the already-authorised account for this
// origin without a prompt, so the whole stack stays connected across
// navigation. First-time visitors are never auto-prompted - the flag is set
// only after a real connect and cleared on detach, so the page asks once.
(function () {
  'use strict';
  if (!FtbUtil.$('connectBtn')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, short = FtbUtil.short;
  var CFG = FtbState.CFG;
  var NET = CFG.network || 'TestNet';
  var LSK = 'ftb.wallet.linked'; // per-browser intent flag (never an address or key)

  function linked(v) {
    try {
      if (v === undefined) return localStorage.getItem(LSK) === '1';
      if (v) localStorage.setItem(LSK, '1'); else localStorage.removeItem(LSK);
    } catch (e) { return false; }
  }

  function setState() {
    var account = FtbState.account;
    $('idstate').textContent = account ? ('[ ' + short(account) + ' @ ' + NET + ' ]') : '[ no wallet ]';
    $('connectBtn').textContent = account ? 'connected' : 'connect ->';
    $('connectBtn').classList.toggle('connected', !!account); // red when disconnected, green when connected
    var d = $('disconnectBtn'); if (d) d.hidden = !account;
  }

  function adopt() {
    FtbState.account = FtbState.dapi ? FtbState.dapi.account : null;
    setState();
    FtbState.emit('account', FtbState.account);
  }

  $('connectBtn').addEventListener('click', function () {
    if (FtbState.account || !FtbState.dapi) return;
    FtbState.dapi.connect().then(function () {
      linked(true);              // remember intent so the rest of the stack stays connected
      adopt();
    }).catch(function (e) {
      $('idstate').textContent = '[ ' + (e.message || 'wallet error') + ' ]';
    });
  });

  // UI-level detach: NeoLine has no programmatic disconnect, so this clears the
  // page's account state and forgets the per-browser link - the wallet itself
  // stays authorised to the site until the user revokes it in NeoLine.
  if ($('disconnectBtn')) $('disconnectBtn').addEventListener('click', function () {
    linked(false);
    FtbState.account = null;
    if (FtbState.dapi) FtbState.dapi.account = null;
    setState();
    $('idstate').textContent = '[ detached - revoke in NeoLine to fully disconnect ]';
    FtbState.emit('account', null);
  });

  if (typeof FtbDapi !== 'undefined' && FtbState.configured) {
    var dapi = new FtbDapi(Object.assign({}, CFG));
    FtbState.dapi = dapi;
    dapi.init().then(function () {
      // silent re-attach, only when the user has connected before on this
      // browser (so a first-time visitor is never auto-prompted). NeoLine
      // returns the already-authorised account with no popup; if authorisation
      // was revoked, connect() rejects and we quietly forget the flag.
      if (!linked() || FtbState.account) return;
      $('idstate').textContent = '[ reconnecting... ]';
      return dapi.connect().then(function () { adopt(); })
        .catch(function () { linked(false); setState(); });
    }).catch(function () { $('idstate').textContent = '[ install NeoLine or a dApi wallet ]'; });
    dapi.on && dapi.on('accountChanged', function (a) {
      FtbState.account = a && a.address ? a.address : null;
      if (FtbState.dapi) FtbState.dapi.account = FtbState.account;
      linked(!!FtbState.account); // keep the per-browser flag in step with live state
      setState();
      FtbState.emit('account', FtbState.account);
    });
  } else {
    $('idstate').textContent = FtbState.configured ? '[ adapter missing ]' : '[ hashes not configured ]';
  }

  // the seam other modules use to nudge the user toward connecting - they never
  // write #idstate directly (R1: one writer per element).
  window.FtbWallet = {
    needConnect: function () { $('idstate').textContent = '[ connect first ]'; }
  };

  setState();
})();
