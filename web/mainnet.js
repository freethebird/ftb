// mainnet.js — the MainNet-window countdown, externalized from an inline <script> so the page
// can keep a strict script-src 'self' CSP (F6 remediation). Same target as shell.js.
(function () {
  var target = Date.UTC(2026, 10, 7, 0, 0, 0); // 7 Nov 2026 00:00 UTC
  function pad(n) { return (n < 10 ? '0' : '') + n; }
  function render(ms) {
    if (ms <= 0) return 'T-0 - MAIN-NET WINDOW OPEN';
    var sec = Math.floor(ms / 1000);
    var d = Math.floor(sec / 86400); sec -= d * 86400;
    var h = Math.floor(sec / 3600); sec -= h * 3600;
    var m = Math.floor(sec / 60); sec -= m * 60;
    return 'T-' + d + 'D ' + pad(h) + ':' + pad(m) + ':' + pad(sec);
  }
  var small = document.getElementById('tminus'), big = document.getElementById('bigminus');
  function tick() { var s = render(target - Date.now()); if (small) small.textContent = s; if (big) big.textContent = s; }
  tick(); setInterval(tick, 1000);
})();
