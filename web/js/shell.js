// shell.js - page chrome and the single review modal (MODULES.md R4).
//
// Owns (writes only these): #shiftLine, #tminus (launchbar countdown),
// #dateline, and the #ov review modal (#ovTitle, #ovWarn, #ovBody, #ovMsg, #ovGo).
// All modal close plumbing ([data-close], overlay click, Escape) lives here.
// Modules request a review via FtbShell.showPlan(title, warn, rows, plan);
// #ovGo executes the plan through the wallet and emits 'refresh' on success.
(function () {
  'use strict';

  var $ = FtbUtil.$, el = FtbUtil.el, errText = FtbUtil.errText;
  var CFG = window.FTB_CONFIG || {};
  var NET = CFG.network || 'TestNet';

  // ---------- theme: one shift only ----------
  // night shift (dark) is the ONLY theme - decided 18 Oct; the pink alternative
  // is retired. There is no toggle and no theme key in localStorage.
  var sl = $('shiftLine'); if (sl) sl.textContent = '// night shift';

  // ---------- launchbar countdown to the MainNet window, 7 Nov 2026 00:00 UTC ----------
  (function () {
    var t = $('tminus'); if (!t) return;
    var target = Date.UTC(2026, 10, 7, 0, 0, 0);
    function pad(n) { return (n < 10 ? '0' : '') + n; }
    function tick() {
      var ms = target - Date.now();
      if (ms <= 0) { t.textContent = 'T-0 - MAIN-NET WINDOW OPEN'; return; }
      var sec = Math.floor(ms / 1000);
      var d = Math.floor(sec / 86400); sec -= d * 86400;
      var h = Math.floor(sec / 3600); sec -= h * 3600;
      var m = Math.floor(sec / 60); sec -= m * 60;
      t.textContent = 'T-' + d + 'D ' + pad(h) + ':' + pad(m) + ':' + pad(sec);
    }
    tick(); setInterval(tick, 1000);
  })();

  // ---------- modal plumbing ----------
  Array.prototype.forEach.call(document.querySelectorAll('[data-close]'), function (b) {
    b.addEventListener('click', function () { b.closest('.overlay').hidden = true; });
  });
  Array.prototype.forEach.call(document.querySelectorAll('.overlay'), function (ov) {
    ov.addEventListener('click', function (e) { if (e.target === ov) ov.hidden = true; });
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') Array.prototype.forEach.call(document.querySelectorAll('.overlay'), function (o) { o.hidden = true; });
  });

  var pendingPlan = null;
  if ($('ovGo')) $('ovGo').addEventListener('click', function () {
    if (!pendingPlan || !FtbState.dapi) return;
    var plan = pendingPlan;
    var msg = $('ovMsg'); msg.innerHTML = ''; msg.appendChild(el('div', 'small muted', 'waiting for your wallet...'));
    $('ovGo').disabled = true;
    FtbState.dapi.execute(plan).then(function (res) {
      var txid = res && res.txid;
      // the confirmation stays open: amount (from the plan summary) + tx id,
      // then a live on-chain confirmation once the tx lands in a block.
      // "ok & refresh" closes the panel and forces a module refresh.
      msg.innerHTML = '';
      msg.appendChild(el('div', 'err ok', 'approved and relayed - ' + (plan.summary || 'see your wallet')));
      if (txid) msg.appendChild(el('div', 'keybox', 'tx: ' + txid));
      var st = el('div', 'small muted', 'waiting for a block - this panel updates itself, no need to refresh.');
      msg.appendChild(st);
      var done = el('button', null, 'ok & refresh ->');
      done.style.marginTop = '10px';
      done.addEventListener('click', function () { $('ov').hidden = true; FtbState.emit('refresh'); });
      msg.appendChild(done);
      $('ovGo').style.display = 'none';
      FtbState.emit('refresh'); // optimistic; the confirmed one lands below
      var tries = 0;
      var iv = setInterval(function () {
        tries++;
        FtbState.emit('refresh');
        if (!txid || !FtbRpc.current()) { if (tries >= 8) clearInterval(iv); return; }
        FtbRpc.call('getapplicationlog', [txid]).then(function (log) {
          var ex = log && log.executions && log.executions[0];
          if (!ex) return;
          clearInterval(iv);
          if (ex.vmstate === 'HALT') {
            st.textContent = 'confirmed on-chain - the modules now show it.';
            st.className = 'err ok';
          } else {
            st.textContent = 'the chain rejected it: ' + (ex.exception || ex.vmstate);
            st.className = 'err';
          }
          FtbState.emit('refresh');
        }).catch(function () {
          if (tries >= 15) {
            clearInterval(iv);
            st.textContent = 'still waiting for the node - the modules will catch up on their own.';
          }
        });
      }, 5000);
    }).catch(function (e) {
      msg.innerHTML = ''; msg.appendChild(el('div', 'err', 'not sent: ' + errText(e)));
    }).finally(function () { $('ovGo').disabled = false; });
  });

  // rows: [[label, value], ...]. plan null = information-only modal (no sign button).
  function showPlan(title, warn, rows, plan) {
    $('ovTitle').textContent = title;
    $('ovWarn').textContent = warn;
    var body = $('ovBody'); body.innerHTML = '';
    rows.forEach(function (r) {
      var kv = el('div', 'kv');
      var lbl = el('div', 'lbl'); lbl.appendChild(el('span', null, r[0]));
      var box = el('div', 'keybox', r[1]);
      kv.appendChild(lbl); kv.appendChild(box); body.appendChild(kv);
    });
    $('ovMsg').innerHTML = '';
    pendingPlan = plan;
    $('ovGo').style.display = '';
    $('ovGo').disabled = !plan;
    $('ov').hidden = false;
  }

  window.FtbShell = { showPlan: showPlan };

  // ---------- boot: pick a healthy endpoint, then let every module read ----------
  if ($('dateline')) $('dateline').textContent = '// CONNECTING //';
  FtbRpc.pick().then(function (url) {
    if ($('dateline')) $('dateline').textContent = '// NEO N3 // ' + NET.toUpperCase() + ' // MMXXVI //';
    FtbState.emit('rpc', url);
  }).catch(function () { /* rpcNote already explains; the own-node modal is the way out */ });
})();
