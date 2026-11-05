// mod-analytics.js — chain-only analytics dashboard (MODULES.md R2/R3/R5/R8/III.9).
// One root (#anRoot). Boots the RPC seam itself (no shell.js on this page) and
// renders live reads: block, supply, cap, %minted, holders, lend, dividends,
// treasury, and per-ballot vote shares. No tracker, no indexer — visit/user
// counts and historical volumes are deliberately absent (page explains why).
// Ratio charts are illustrative in this preview; live sampling wires on accept.
// No innerHTML, no inline handlers — DOM is built node by node (gate-clean).
(function () {
  'use strict';
  if (!window.FtbUtil || !FtbUtil.$('anRoot')) return;   // R2: inert without the page
  var $ = FtbUtil.$, fmt = FtbUtil.fmt, grp = FtbUtil.grp, tileNum = FtbUtil.tileNum, el = FtbUtil.el;
  var CFG = FtbState.CFG, C = FtbState.C;
  var NS = 'http://www.w3.org/2000/svg';

  function set(id, t) { var e = $(id); if (e) e.textContent = t; }
  function live(h) { return h && h.indexOf('0x0000') !== 0; }
  function bps(v) { return (Number(v) / 100).toFixed(2) + '%'; }
  function clear(node) { while (node && node.firstChild) node.removeChild(node.firstChild); }

  function refresh() {
    if (!window.FtbRpc || !FtbRpc.current()) return;
    FtbRpc.call('getblockcount').then(function (h) { set('anBlock', Number(h).toLocaleString('en-US')); }).catch(function () {});
    FtbState.proofs().then(function (j) { set('anHolders', grp(((j.entries || []).length).toString())); }).catch(function () {});
    if (!FtbState.configured) return;

    Promise.all([
      FtbRpc.readInt(C.ftb, 'totalSupply').catch(function () { return null; }),
      FtbRpc.readInt(C.ftb, 'maxSupply').catch(function () { return null; })
    ]).then(function (r) {
      if (r[0] != null) tileNum($('anSupply'), r[0]);
      if (r[1] != null) tileNum($('anCap'), r[1]);
      if (r[0] != null && r[1] != null && r[1] > 0n) set('anMinted', (Number(r[0] * 10000n / r[1]) / 100).toFixed(2) + '%');
    });

    if (live(C.loanbook)) {
      FtbRpc.readInt(C.loanbook, 'offerCount').then(function (v) { tileNum($('anLoanOffers'), v); }).catch(function () {});
      FtbRpc.readInt(C.ftb, 'balanceOf', [{ type: 'Hash160', value: C.loanbook }]).then(function (v) { tileNum($('anLoanTvl'), v, ' FTB'); tileNum($('anTvl'), v, ' FTB'); }).catch(function () {});
    }
    if (live(C.dividends)) {
      FtbRpc.readInt(C.dividends, 'totalStaked').then(function (v) { tileNum($('anStaked'), v, ' FTB'); }).catch(function () {});
    }
    if (live(C.treasury)) {
      FtbRpc.readInt(C.ftb, 'balanceOf', [{ type: 'Hash160', value: C.treasury }]).then(function (v) { tileNum($('anTreasury'), v, ' FTB'); }).catch(function () {});
    }
    renderBallots();
    readDoors();
  }

  // Governance: one card per configured ballot. Bars are SHARES of votes cast
  // (v / total) — unit-agnostic and honest. Turnout vs the electorate is left to
  // mod-gov's exact weighting on the live page; here we show the contest.
  function renderBallots() {
    var wrap = $('anBallots'); if (!wrap) return;
    var ballots = CFG.ballots || [];
    if (!ballots.length) { clear(wrap); wrap.appendChild(el('div', 'annote', 'no live ballots configured for this network.')); return; }
    if (!wrap.dataset.built) {
      wrap.dataset.built = '1'; clear(wrap);
      ballots.forEach(function (b, bi) {
        var row = el('div', 'anballot');
        row.appendChild(el('div', 'anbq', b.question || ('ballot ' + (bi + 1))));
        (b.options || []).forEach(function (opt, oi) {
          var o = el('div', 'anopt');
          o.appendChild(el('span', 'anoptname', opt));
          var bw = el('span', 'anbarwrap');
          var bar = el('span', 'anbar'); bar.id = 'anb_' + bi + '_' + oi; bw.appendChild(bar);
          o.appendChild(bw);
          var vv = el('span', 'anoptv', '—'); vv.id = 'anv_' + bi + '_' + oi; o.appendChild(vv);
          row.appendChild(o);
        });
        var ft = el('div', 'anfoot', 'shares of votes cast · live'); ft.id = 'anfoot_' + bi; row.appendChild(ft);
        wrap.appendChild(row);
      });
    }
    ballots.forEach(function (b, bi) {
      if (!FtbRpc.current()) return;
      var n = (b.options || []).length, reads = [];
      for (var k = 1; k <= n; k++) reads.push(FtbRpc.readInt(b.hash, 'tally', [{ type: 'Integer', value: String(k) }]).catch(function () { return 0n; }));
      Promise.all(reads).then(function (vals) {
        var total = vals.reduce(function (a, v) { return a + v; }, 0n);
        vals.forEach(function (v, oi) {
          var share = total > 0n ? Number(v * 10000n / total) / 100 : 0;
          var bar = $('anb_' + bi + '_' + oi); if (bar) bar.style.width = share + '%';
          var vv = $('anv_' + bi + '_' + oi); if (vv) vv.textContent = total > 0n ? share.toFixed(1) + '%' : '—';
        });
        var ft = $('anfoot_' + bi);
        if (ft) ft.textContent = total > 0n ? 'shares of votes cast · live' : 'no votes recorded yet · live';
      });
    });
  }

  // Minimal dependency-free SVG line+area chart, built with createElementNS.
  function spark(id, series, up) {
    var host = $(id); if (!host) return;
    clear(host);
    var w = 320, h = 92, pad = 6;
    var min = Math.min.apply(null, series), max = Math.max.apply(null, series), rng = (max - min) || 1;
    var pts = series.map(function (v, i) {
      var x = pad + i * (w - 2 * pad) / (series.length - 1);
      var y = h - pad - (v - min) / rng * (h - 2 * pad);
      return x.toFixed(1) + ',' + y.toFixed(1);
    });
    var col = up ? 'var(--ok)' : 'var(--dn)';
    var svg = document.createElementNS(NS, 'svg');
    svg.setAttribute('viewBox', '0 0 ' + w + ' ' + h);
    svg.setAttribute('preserveAspectRatio', 'none');
    svg.setAttribute('width', '100%'); svg.setAttribute('height', '92'); svg.setAttribute('aria-hidden', 'true');
    var area = document.createElementNS(NS, 'path');
    area.setAttribute('d', 'M' + pad + ',' + (h - pad) + ' L' + pts.join(' L') + ' L' + (w - pad) + ',' + (h - pad) + ' Z');
    area.setAttribute('fill', col); area.setAttribute('opacity', '0.08');
    var line = document.createElementNS(NS, 'path');
    line.setAttribute('d', 'M' + pts.join(' L'));
    line.setAttribute('fill', 'none'); line.setAttribute('stroke', col); line.setAttribute('stroke-width', '1.5');
    svg.appendChild(area); svg.appendChild(line); host.appendChild(svg);
  }
  function charts() {
    spark('anChartGasFtb', [40, 39.2, 38.6, 37.9, 38.4, 36.8, 35.1, 34.0, 33.35, 33.9, 34.2, 33.6], false);
    spark('anChartNeoFtb', [11.8, 12.0, 12.3, 12.1, 12.6, 12.9, 12.7, 13.1, 13.0, 13.3, 13.2, 13.45], true);
  }

  // ---- decentralization meter: irreversible on-chain one-way doors ----
  // Measured, never asserted. Each door is a power the protocol gives up for good;
  // we read each live. A door we cannot CONFIRM crossed scores zero, and the
  // denominator is the full weight (100) — so unknown and still-open both depress
  // the figure. The meter can only over-read by seeing a door as crossed, and every
  // read fails safe to 'unknown'. No indexer, no event scan: pure current-state reads.
  var DOORS = [
    { key: 'renounce', name: 'owner key renounced', w: 55 },   // token immutable: no Update, no re-point, no re-mint
    { key: 'mint',     name: 'mint permanently sealed', w: 30 },// supply can never grow again (CloseMigration)
    { key: 'gov',      name: 'governance bootstrap finished', w: 15 } // seams move only through the timelock
  ];
  // true iff a base64 storage value decodes to a nonzero byte sequence (Neo stores an
  // integer 0 as empty bytes, so empty/all-zero == a cleared flag; atob is dependency-free).
  function isMainnet() { return (CFG && CFG.network) === 'MainNet'; } // meters are MainNet-only
  function b64nz(v) {
    if (v == null || v === '') return false;
    try { var s = atob(v); for (var i = 0; i < s.length; i++) { if (s.charCodeAt(i) !== 0) return true; } return false; } catch (e) { return false; }
  }
  function buildDoors() {
    var host = $('dcDoors'); if (!host || host.dataset.built) return;
    host.dataset.built = '1'; clear(host);
    DOORS.forEach(function (d) {
      var row = el('div', 'mtdoor');
      var nm = el('span', 'mtdname', d.name); nm.appendChild(el('span', 'mtdw', d.w + '%'));
      row.appendChild(nm);
      var stt = el('span', 'mtdstate');
      var dot = el('span', 'mtdot'); dot.id = 'dcdot_' + d.key;
      var txt = el('span', '', '…'); txt.id = 'dcstate_' + d.key;
      stt.appendChild(dot); stt.appendChild(txt); row.appendChild(stt);
      host.appendChild(row);
    });
  }
  function doorState(key, s) {
    var dot = $('dcdot_' + key), txt = $('dcstate_' + key);
    if (txt) txt.textContent = (s === 'crossed') ? 'crossed' : (s === 'open' ? 'not yet' : 'unknown');
    if (dot) dot.className = 'mtdot' + (s === 'crossed' ? ' mtok' : (s === 'unknown' ? ' mtunk' : ''));
    var par = dot && dot.parentNode; if (par) par.className = 'mtdstate' + (s === 'crossed' ? ' mtok' : '');
  }
  function readDoors() {
    if (!isMainnet()) return;                 // never read/show on TestNet
    if (!window.FtbRpc || !FtbRpc.current() || !FtbState.configured) return;
    buildDoors();
    var st = {};
    function fin() {
      var crossed = 0, nCrossed = 0, any = false, nTot = DOORS.length;
      DOORS.forEach(function (d) {
        var s = st[d.key] || 'unknown';
        if (s !== 'unknown') any = true;
        if (s === 'crossed') { crossed += d.w; nCrossed++; }
        doorState(d.key, s);
      });
      var fill = $('dcFill');
      if (!any) { set('dcPct', '—'); set('dcSub', 'chain unreadable — retrying'); if (fill) fill.style.width = '0%'; return; }
      set('dcPct', crossed + '%'); if (fill) fill.style.width = crossed + '%';
      set('dcSub', nCrossed === nTot ? ('fully ownerless — all ' + nTot + ' one-way doors closed')
               : (nCrossed === 0 ? 'keys still held — no doors closed yet (expected pre-launch)'
               : (nCrossed + ' of ' + nTot + ' one-way doors closed — control not yet fully retired')));
    }
    // Read raw contract STORAGE, not getter methods: storage is the canonical, version-
    // independent ground truth (older deployments may lack a 'MigrationOpen'/'Finished'
    // getter, but the one-way-door keys below are fixed and set at deploy). Keys, from the
    // contract source: token keyOwner=0x11 ('EQ=='), token keyOpen=0x13 ('Ew=='),
    // registry keyFinished=0x03 ('Aw=='). Each read fails safe to 'unknown'; a door is only
    // ever 'crossed' on a definitive value, so the meter cannot over-read decentralization.
    Promise.all([
      // owner renounced == owner slot emptied (b64nz false); a 20-byte owner is still held.
      FtbRpc.call('getstorage', [C.ftb, 'EQ==']).then(function (v) {
        st.renounce = (v == null) ? 'unknown' : (b64nz(v) ? 'open' : 'crossed');
      }).catch(function () { st.renounce = 'unknown'; }),
      // mint sealed == keyOpen integer 0 (stored empty); 1 ('AQ==') while still open.
      FtbRpc.call('getstorage', [C.ftb, 'Ew==']).then(function (v) {
        st.mint = (v == null) ? 'unknown' : (b64nz(v) ? 'open' : 'crossed');
      }).catch(function () { st.mint = 'unknown'; }),
      // bootstrap finished == keyFinished nonzero; absent/zero == not yet (open).
      (C.registry && live(C.registry))
        ? FtbRpc.call('getstorage', [C.registry, 'Aw==']).then(function (v) { st.gov = b64nz(v) ? 'crossed' : 'open'; }).catch(function () { st.gov = 'unknown'; })
        : Promise.resolve().then(function () { st.gov = 'unknown'; })
    ]).then(fin);
  }

  var refreshC = FtbUtil.coalesce(refresh, 3000);
  FtbState.on('rpc', refreshC);
  FtbState.on('refresh', refreshC);
  charts();                                    // illustrative shapes render immediately
  renderBallots();                             // ballot cards from config (shares fill on rpc)
  if (isMainnet()) {                             // reveal the MainNet-only safety-meter section
    var _sms = $('safetyMeterSec'); if (_sms) _sms.classList.remove('mnet-only');
    buildDoors();                                // door skeleton; states fill on rpc
  }
  if (window.FtbRpc && !FtbRpc.current()) {     // boot RPC ourselves (shell.js does this on index)
    FtbRpc.pick().then(function (url) { FtbState.emit('rpc', url); }).catch(function () { /* standby: tiles stay '-' per R8 */ });
  } else { refreshC(); }
})();
