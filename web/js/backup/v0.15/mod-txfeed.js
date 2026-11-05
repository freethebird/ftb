// mod-txfeed.js - recent-transactions feed on the explorer page (explorer.html).
// Walks back from the chain tip and lists transactions (newest first) for movement
// transparency. Read-only: getblockcount + getblock, no wallet, no writes. Bounded
// (stops at ~100 tx or MAXBLOCKS scanned) and fail-closed - a read error shows a note,
// never a fabricated feed. DOM via textContent / element construction only.
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('txFeed')) return;
  if (!window.FtbState || !window.FtbRpc) return;
  var $ = FtbUtil.$, el = FtbUtil.el, short = FtbUtil.short;
  var TARGET = 100, MAXBLOCKS = 120, BATCH = 10;

  function when(ms) {
    var s = (Date.now() - Number(ms)) / 1000;
    if (!(s >= 0)) return '';
    if (s < 60) return Math.max(1, Math.round(s)) + 's ago';
    if (s < 3600) return Math.round(s / 60) + 'm ago';
    if (s < 86400) return Math.round(s / 3600) + 'h ago';
    return Math.round(s / 86400) + 'd ago';
  }

  function run() {
    var body = $('txFeed'); if (!body) return;
    FtbRpc.call('getblockcount').then(function (cnt) {
      var tip = (+cnt) - 1; if (!(tip >= 0)) throw new Error('bad height');
      var heights = [];
      for (var h = tip; h > tip - MAXBLOCKS && h >= 0; h--) heights.push(h);
      var txs = [], scanned = 0;
      function batch(i) {
        if (i >= heights.length || txs.length >= TARGET) return Promise.resolve();
        return Promise.all(heights.slice(i, i + BATCH).map(function (h) {
          return FtbRpc.call('getblock', [h, 1]).catch(function () { return null; });
        })).then(function (blocks) {
          blocks.forEach(function (b) {
            if (!b) return; scanned++;
            (b.tx || []).forEach(function (t) { txs.push({ hash: t.hash, sender: t.sender, time: b.time, index: b.index }); });
          });
          if (txs.length >= TARGET) return;
          return batch(i + BATCH);
        });
      }
      return batch(0).then(function () { render(body, txs, scanned); });
    }).catch(function (e) {
      body.replaceChildren();
      body.appendChild(el('div', 'small muted', 'transaction feed unavailable: ' + FtbUtil.errText(e)));
      var n = $('txCount'); if (n) n.textContent = 'unavailable';
    });
  }

  function render(body, txs, scanned) {
    body.replaceChildren();
    if (!txs.length) { body.appendChild(el('div', 'small muted', 'no transactions in the last ' + scanned + ' blocks.')); var n0 = $('txCount'); if (n0) n0.textContent = '0 tx'; return; }
    txs.slice(0, TARGET + 40).forEach(function (t) {
      var row = el('div', 'txrow');
      var a = el('a', 'txid', short(t.hash)); a.href = 'explorer.html?q=' + encodeURIComponent(t.hash); a.title = t.hash;
      row.appendChild(a);
      row.appendChild(el('span', 'txblk', '#' + t.index));
      row.appendChild(el('span', 'txwho', t.sender ? short(t.sender) : '—'));
      row.appendChild(el('span', 'txwhen', when(t.time)));
      body.appendChild(row);
    });
    var n = $('txCount'); if (n) n.textContent = txs.length + ' tx · last ' + scanned + ' blocks';
  }

  var runC = FtbUtil.coalesce(run, 5000);
  var rb = $('txRefresh'); if (rb) rb.addEventListener('click', function () { runC(); });
  FtbState.on('rpc', runC);
  if (FtbRpc.current()) runC();
})();
