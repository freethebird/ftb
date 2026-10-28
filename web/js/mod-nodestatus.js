// mod-nodestatus.js — public node health on the analytics page (MODULES.md R2/R3/R5/R8).
// Renders health badges for both nodes + the self-healing watchdog from the node's own
// /status.json. The read goes through the FtbState.status() seam (R3 — no module fetch);
// the URL is config-only (R5); on any failure it shows a named standby (R8). Dot colour is
// grayscale + the negative key only — R10 reserves the phosphor green for clickables, so a
// healthy node is the bright mark, not green. No innerHTML, no inline handlers — textContent
// and classList only. This is the node reporting its OWN health, the same class as the /rpc
// it already serves: not a visitor tracker and not an indexer, so the page's claim holds.
(function () {
  'use strict';
  if (!window.FtbState || !window.FtbUtil || !FtbUtil.$('nsFoot')) return;   // R2: inert without the page
  var $ = FtbUtil.$, grp = FtbUtil.grp;

  function paintNode(dotId, txtId, node) {
    var dot = $(dotId), txt = $(txtId); if (!dot || !txt) return;
    dot.classList.remove('nsup', 'nsdn');
    if (!node || node.status === 'down') { dot.classList.add('nsdn'); txt.textContent = 'offline'; return; }
    if (node.status === 'synced') { dot.classList.add('nsup'); txt.textContent = 'synced · ' + grp(node.height); return; }
    txt.textContent = 'syncing ' + (node.pct != null ? node.pct + '%' : '') + ' · ' + grp(node.height);
  }

  function render(d) {
    var n = (d && d.nodes) || {};
    paintNode('nsTestDot', 'nsTestTxt', n.testnet);
    paintNode('nsMainDot', 'nsMainTxt', n.mainnet);
    var wd = $('nsWatchDot'), wt = $('nsWatchTxt');
    if (wd && wt) {
      var active = !!(d && d.watchdog && d.watchdog.active);
      wd.classList.remove('nsup', 'nsdn'); wd.classList.add(active ? 'nsup' : 'nsdn');
      wt.textContent = active ? 'active' : 'inactive';
    }
    var foot = $('nsFoot');
    if (foot) foot.textContent = 'live from the node · updated ' +
      (d && d.updated ? d.updated.replace('T', ' ').replace('Z', ' UTC') : '—') + ' · refreshes each minute';
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
