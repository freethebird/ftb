// connect-check.js - dependency-free wallet-injection diagnostic.
// Reports, on the visible page, which Neo wallet interface (if any) the browser
// injected: the legacy window.NEOLineN3 (desktop extension) or the NEP-21
// Neo.DapiProvider (modern NeoLine, incl. its mobile in-app browser). No fetch,
// no external code, textContent only (Canon Book IV: no markup sink).
(function () {
  'use strict';
  var out = document.getElementById('out');
  out.textContent = '';
  function line(text, cls) {
    var d = document.createElement('div');
    d.textContent = text;
    if (cls) d.className = cls;
    out.appendChild(d);
  }
  function head(text) { line(text, 'hd'); }
  function yn(label, cond) { line((cond ? 'YES  ' : 'no   ') + label, cond ? 'yes' : 'no'); }

  head('environment');
  line('userAgent: ' + navigator.userAgent);
  line('time: ' + new Date().toISOString());

  head('legacy  window.NEOLineN3  (desktop extension API)');
  yn('window.NEOLineN3 present', typeof window.NEOLineN3 !== 'undefined');
  try { yn('NEOLineN3.Init is a constructor', !!(window.NEOLineN3 && typeof window.NEOLineN3.Init === 'function')); }
  catch (e) { line('NEOLineN3.Init check error: ' + e.message); }
  yn('window.NEOLine present', typeof window.NEOLine !== 'undefined');

  var legacyReady = false;
  window.addEventListener('NEOLine.N3.EVENT.READY', function () {
    legacyReady = true; yn('NEOLine.N3.EVENT.READY fired', true);
  });

  head('NEP-21  Neo.DapiProvider  (modern injected provider)');
  var nep21 = false, provRef = null;
  window.addEventListener('Neo.DapiProvider.ready', function (e) {
    nep21 = true;
    var p = e && e.detail && e.detail.provider;
    provRef = p;
    yn('Neo.DapiProvider.ready fired', true);
    if (p) {
      line('provider.name: ' + String(p.name));
      line('provider.version: ' + String(p.version || p.dapiVersion || '(none)'));
      var methods = [];
      for (var k in p) { try { if (typeof p[k] === 'function') methods.push(k); } catch (e2) {} }
      line('provider method keys: ' + (methods.join(', ') || '(none enumerable)'));
      ['request', 'getAccount', 'getAccounts', 'pickAddress', 'invoke', 'invokeRead', 'call', 'getNetworks', 'getNetwork', 'signMessage'].forEach(function (m) {
        yn('provider.' + m + '()', typeof p[m] === 'function');
      });
    }
  });
  try { window.dispatchEvent(new Event('Neo.DapiProvider.request')); line('dispatched Neo.DapiProvider.request: ok'); }
  catch (e) { line('dispatch error: ' + e.message); }

  head('other wallet-ish globals');
  try {
    var keys = [];
    for (var wk in window) { try { if (/neo|dapi|wallet|o3|onegate|walletconnect|ethereum|tron/i.test(wk)) keys.push(wk); } catch (e3) {} }
    line(keys.join(', ') || '(none found)');
  } catch (e) { line('key scan error: ' + e.message); }

  // injection can be late; re-check and re-announce.
  [1500, 4000, 8000].forEach(function (ms) {
    setTimeout(function () {
      head('recheck @ ' + ms + 'ms');
      yn('window.NEOLineN3 present', typeof window.NEOLineN3 !== 'undefined');
      yn('legacy READY fired', legacyReady);
      yn('NEP-21 ready fired', nep21);
      if (!nep21) { try { window.dispatchEvent(new Event('Neo.DapiProvider.request')); } catch (e) {} }
    }, ms);
  });
})();
