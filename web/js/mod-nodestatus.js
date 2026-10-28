// mod-nodestatus.js — public node health on the analytics page (MODULES.md R2/R3/R5/R8/R10).
// Reads the node's own /status.json — written every 60s by ftb-status on the node
// and served by Caddy OUTSIDE the IPFS bundle (so it updates live between deploys).
// Paints health badges for both nodes + the self-healing watchdog. This is the node
// reporting its OWN health (same class as the /rpc it already serves) — not a visitor
// tracker and not a history indexer, so the page's "no tracker / no indexer" holds.
// If the feed goes stale (the writer stopped), it says so rather than passing old
// numbers off as live (R5: never cached-as-fresh). No innerHTML, no inline handlers —
// textContent + classList only. Dot state classes are namespaced (nsup/nsdn) to avoid
// p2-shell's .ok/.dn utilities; a healthy node is the bright mark, not green (R10).
(function () {
  'use strict';
  if (!window.FtbState || !window.FtbUtil || !FtbUtil.$('nsFoot')) return;   // R2: inert without the page
  var $ = FtbUtil.$, grp = FtbUtil.grp;
  var STALE_MS = 180000;   // the feed refreshes every 60s; >3 missed updates = stale

  function paintNode(dotId, txtId, node, stale) {
    var dot = $(dotId), txt = $(txtId); if (!dot || !txt) return;
    dot.classList.remove('nsup', 'nsdn');
    if (!node || node.status === 'down') { dot.classList.add('nsdn'); txt.textContent = 'offline'; return; }
    var label = node.status === 'synced'
      ? 'synced · ' + grp(node.height)
      : 'syncing ' + (node.pct != null ? node.pct + '%' : '') + ' · ' + grp(node.height);
    if (stale) { txt.textContent = label + ' (stale)'; return; }   // feed dead: neutral dot, marked
    if (node.status === 'synced') dot.classList.add('nsup');
    txt.textContent = label;
  }

  function render(d) {
    var upd = d && d.updated ? d.updated : null;
    var ageMs = upd ? (Date.now() - Date.parse(upd)) : NaN;
    var stale = !upd || !isFinite(ageMs) || ageMs > STALE_MS;
    var n = (d && d.nodes) || {};
    paintNode('nsTestDot', 'nsTestTxt', n.testnet, stale);
    paintNode('nsMainDot', 'nsMainTxt', n.mainnet, stale);
    var wd = $('nsWatchDot'), wt = $('nsWatchTxt');
    if (wd && wt) {
      wd.classList.remove('nsup', 'nsdn');
      if (stale) {
        wt.textContent = 'unknown';                       // neutral dot: indeterminate, not confirmed-down
      } else {
        var active = !!(d && d.watchdog && d.watchdog.active);
        wd.classList.add(active ? 'nsup' : 'nsdn');
        wt.textContent = active ? 'active' : 'inactive';
      }
    }
    var foot = $('nsFoot');
    if (foot) {
      if (stale && upd) foot.textContent = 'feed stale — node last reported ' + Math.max(1, Math.round(ageMs / 60000)) + ' min ago (status writer may be down)';
      else if (upd) foot.textContent = 'live from the node · updated ' + upd.replace('T', ' ').replace('Z', ' UTC') + ' · refreshes each minute';
      else foot.textContent = 'node status endpoint unreachable';
    }
  }

  function fail() {
    ['nsTest', 'nsMain', 'nsWatch'].forEach(function (p) {
      var dot = $(p + 'Dot'), txt = $(p + 'Txt');
      if (dot) { dot.classList.remove('nsup'); dot.classList.add('nsdn'); }
      if (txt) txt.textContent = 'unreachable';
    });
    var foot = $('nsFoot'); if (foot) foot.textContent = 'node status endpoint unreachable';
  }

  function load() { FtbState.status().then(render).catch(fail); }

  load();
  setInterval(load, 60000);   // live heartbeat, matched to the node's 60s refresh
})();
