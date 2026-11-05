// mod-mirrors.js - the "mirrors live" tile (#tMirrors) on the main page.
// Shows how many distinct nodes are currently PROVIDING this exact front-end
// release on the IPFS DHT: the real, permissionless mirror count. Every node
// that pins and announces the build self-registers on the DHT, so this counts
// ALL of them (not a fixed list of base URLs): the decentralization / safety
// signal for the site. Source: the node's own /status.json ("mirrors" field),
// written each minute by ftb-status and served by Caddy OUTSIDE the IPFS bundle
// (so it is live between deploys). textContent + title only; no innerHTML, no
// handlers (R8). Inert on any page without the tile (R2). If the feed is stale
// or unreachable it never passes an old number off as fresh (R5): it keeps the
// last count but marks it in the hover, or shows "-" when nothing is trustworthy.
(function () {
  'use strict';
  if (!window.FtbState || !window.FtbUtil || !FtbUtil.$('tMirrors')) return;   // R2
  var $ = FtbUtil.$;
  var STALE_MS = 180000;   // feed refreshes every 60s; >3 missed updates = stale

  function render(d) {
    var tile = $('tMirrors'); if (!tile) return;
    var upd = d && d.updated ? d.updated : null;
    var ageMs = upd ? (Date.now() - Date.parse(upd)) : NaN;
    var stale = !upd || !isFinite(ageMs) || ageMs > STALE_MS;
    var n = d ? d.mirrors : null;
    if (n == null) {                       // feed reachable but count unavailable
      tile.textContent = '-';
      tile.title = 'mirror count temporarily unavailable';
      return;
    }
    tile.textContent = String(n);
    tile.title = (n === 1 ? '1 node is' : n + ' nodes are')
      + ' serving this exact build on the IPFS DHT'
      + (stale
          ? ' · last reported ' + Math.max(1, Math.round(ageMs / 60000)) + ' min ago'
          : ' · live, refreshes each minute');
  }

  function fail() {
    var tile = $('tMirrors'); if (!tile) return;
    tile.textContent = '-';
    tile.title = 'node status endpoint unreachable';
  }

  function load() { FtbState.status().then(render).catch(fail); }
  load();
  setInterval(load, 60000);   // live heartbeat, matched to the node's 60s refresh
})();
