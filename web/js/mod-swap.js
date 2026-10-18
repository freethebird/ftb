// mod-swap.js — module 1: swap / pools (MODULES.md).
// The swap ticket (external Flamingo router), the pool-ratio board, the market
// ticker, and the TVL tile — ratios only, no fiat, no oracle.
//
// Owns (writes only these): #swap (#swAmt, #swFromSel, #swToSel, #swOut,
// #swMeta, #swGo), #market (#ratioBody, #ratioHint, #ratioBtn), #ticker, #tTvl.
(function () {
  'use strict';
  if (!FtbUtil.$('swap')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;

  // ---------- swap ticket (enabled only when the router is configured) ----------
  var TOKENS = { 'bNEO': function () { return C.bneo; }, 'GAS': function () { return (CFG.assets || {}).gas; }, 'FTB': function () { return C.ftb; } };
  var swFrom = 'bNEO', swTo = 'FTB', swQuote = null;
  function routerReady() { return !!(FtbState.configured && C.router && C.router.indexOf('0x0000') !== 0); }
  function paintSel(id, cur, setter) {
    var box = $(id); box.innerHTML = '';
    Object.keys(TOKENS).forEach(function (sym) {
      var b = el('button', sym === cur ? 'on' : null, sym);
      b.addEventListener('click', function () { setter(sym); });
      box.appendChild(b);
    });
  }
  function setFrom(s) { if (s === swTo) swTo = swFrom; swFrom = s; paintSels(); quote(); }
  function setTo(s) { if (s === swFrom) swFrom = swTo; swTo = s; paintSels(); quote(); }
  function paintSels() { paintSel('swFromSel', swFrom, setFrom); paintSel('swToSel', swTo, setTo); }
  function quote() {
    swQuote = null; $('swOut').value = '';
    if (!routerReady()) { $('swMeta').textContent = 'loads when the router hash is configured for this network'; return; }
    var raw = $('swAmt').value.trim();
    if (!raw) { $('swMeta').textContent = 'quote from live pool reserves · 0.3% pool fee · min received at 0.5% slippage'; return; }
    var amtIn;
    try { amtIn = BigInt(FtbDapi.util.toBaseUnits(raw)); } catch (e) { $('swMeta').textContent = 'bad amount'; return; }
    if (amtIn <= 0n) { $('swMeta').textContent = 'enter an amount above zero'; return; }
    var a = TOKENS[swFrom](), b = TOKENS[swTo]();
    $('swMeta').textContent = 'quoting…';
    FtbRpc.read(C.router, 'getReserves', [{ type: 'Hash160', value: a }, { type: 'Hash160', value: b }]).then(function (st) {
      var rA = BigInt(st[0].value), rB = BigInt(st[1].value); // verify order against the deployed pair
      if (rA === 0n || rB === 0n) throw new Error('pool empty');
      var out = (amtIn * 997n * rB) / (rA * 1000n + amtIn * 997n);
      var minOut = out * 995n / 1000n;
      var spot = Number(fmt(rB)) / Number(fmt(rA));
      var impact = (1 - (Number(fmt(out)) / Number(fmt(amtIn))) / spot) * 100;
      swQuote = { amtIn: amtIn, out: out, minOut: minOut, rA: rA, rB: rB };
      $('swOut').value = fmt(out);
      var m = $('swMeta'); m.innerHTML = '';
      m.appendChild(el('span', null, 'rate 1 ' + swFrom + ' = ' + spot.toPrecision(6) + ' ' + swTo + ' · min received ' + fmt(minOut) + ' ' + swTo + ' · '));
      m.appendChild(el('span', impact > 5 ? 'impact-warn' : null, 'price impact ' + impact.toFixed(2) + '%'));
    }).catch(function (e) { $('swMeta').textContent = 'quote failed: ' + errText(e); });
  }
  var swT = null;
  $('swAmt').addEventListener('input', function () { clearTimeout(swT); swT = setTimeout(quote, 250); });
  $('swGo').addEventListener('click', function () {
    if (!routerReady()) {
      FtbShell.showPlan('swap not wired', 'the router hash is a placeholder in this deployment. the swap contracts are the reused, audited Flamingo Factory/Pair/Router — verify the method and signer scope against the deployed manifest before enabling.',
        [['status', 'router hash not configured']], null);
      return;
    }
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    if (!swQuote) { $('swMeta').textContent = 'no quote yet — enter an amount'; return; }
    var deadline = Date.now() + 10 * 60 * 1000;
    var plan = FtbState.dapi.swap(FtbState.account, swQuote.amtIn.toString(), swQuote.minOut.toString(), [TOKENS[swFrom](), TOKENS[swTo]()], deadline);
    FtbShell.showPlan('swap ' + fmt(swQuote.amtIn) + ' ' + swFrom,
      'external flamingo router. quote from live reserves; min received is 0.5% under the quote. verify the router hash in section IV first.',
      [['you pay', fmt(swQuote.amtIn) + ' ' + swFrom],
       ['you receive (est.)', fmt(swQuote.out) + ' ' + swTo],
       ['min received', fmt(swQuote.minOut) + ' ' + swTo],
       ['route', swFrom + ' → ' + swTo],
       ['what you sign', plan.summary]],
      plan);
  });
  paintSels();

  // ---------- pool ratios (asset vs asset; no fiat, no oracle) ----------
  var SPARK = '▁▂▃▄▅▆▇█';
  var tick = {}, tickBlock = null;
  function paintTicker() {
    var t = $('ticker'); if (!t) return;
    var parts = [];
    Object.keys(tick).forEach(function (k) { parts.push('<b>' + k + '</b> ' + tick[k]); });
    if (tickBlock) parts.push('<b>block</b> ' + Number(tickBlock).toLocaleString('en-US'));
    t.innerHTML = parts.length ? '// market&nbsp; ' + parts.join('<span class="sep">·</span>') : '// market: waiting for the pools';
  }
  function resolveTok(t) { return t === 'FTB' ? C.ftb : (t === 'GAS' ? ((CFG.assets || {}).gas) : t); }
  function sparkLine(vals) {
    if (!vals.length) return '';
    var lo = Math.min.apply(null, vals), hi = Math.max.apply(null, vals), span = (hi - lo) || 1;
    return vals.map(function (v) { return SPARK[Math.min(7, Math.floor((v - lo) / span * 8))]; }).join('');
  }
  function refreshRatios() {
    var body = $('ratioBody'); if (!body) return;
    var pairs = CFG.pairs || [];
    if (!FtbRpc.current() || !FtbState.configured || !C.router || C.router.indexOf('0x0000') === 0 || !pairs.length) {
      var hint = $('ratioHint'); if (hint) hint.textContent = 'loads when the router and pairs are configured for this network';
      return;
    }
    var hist = {}; try { hist = JSON.parse(localStorage.getItem('ftb_ratios') || '{}'); } catch (e) {}
    body.innerHTML = '';
    pairs.forEach(function (p) {
      var sym = p.label.split('/'), a = resolveTok(p.a), b = resolveTok(p.b);
      var card = el('div', 'rcard');
      card.appendChild(el('div', 'rp', '1 ' + sym[0] + ' ='));
      var num = el('div', 'rnum', 'reading…'); card.appendChild(num);
      var inv = el('div', 'rinv', ''); card.appendChild(inv);
      var foot = el('div', 'rspark'); card.appendChild(foot);
      body.appendChild(card);
      FtbRpc.read(C.router, 'getReserves', [{ type: 'Hash160', value: a }, { type: 'Hash160', value: b }]).then(function (st) {
        var rA = Number(fmt(BigInt(st[0].value))), rB = Number(fmt(BigInt(st[1].value)));
        if (!(rA > 0 && rB > 0)) throw new Error('pool empty');
        var ratio = rB / rA; // display-only: whole units of B per 1 A
        var h = hist[p.label] || []; h.push(ratio); if (h.length > 40) h = h.slice(h.length - 40);
        hist[p.label] = h;
        try { localStorage.setItem('ftb_ratios', JSON.stringify(hist)); } catch (e) {}
        num.textContent = ratio.toPrecision(6) + ' ' + sym[1];
        inv.textContent = '1 ' + sym[1] + ' = ' + (1 / ratio).toPrecision(6) + ' ' + sym[0];
        var chg = h.length > 1 ? (ratio / h[0] - 1) * 100 : 0;
        var cls = chg > 0.005 ? 'up' : (chg < -0.005 ? 'dn' : 'flat');
        tick[p.label] = ratio.toPrecision(6) + ' ' + (cls === 'up' ? '▲' : cls === 'dn' ? '▼' : '=');
        paintTicker();
        foot.appendChild(el('span', null, sparkLine(h) || '·'));
        foot.appendChild(el('span', 'rchg ' + cls, (chg >= 0 ? '+' : '') + chg.toFixed(2) + '% · ' + h.length + ' samples'));
      }).catch(function (e) {
        num.textContent = '—';
        inv.textContent = e.message || 'unavailable';
      });
    });
  }
  $('ratioBtn').addEventListener('click', refreshRatios);

  // ---------- TVL: total value locked in liquidity, in FTB terms ----------
  // No AMM exists on TestNet (neo#3285), so the router is a placeholder there and
  // TVL is honestly zero; once C.router points at a live router, sum the FTB-side
  // reserves of every configured pair (both sides counted when the pair trades
  // against FTB).
  function refreshTvl() {
    var e = $('tTvl'); if (!e) return;
    if (!C.router || C.router.indexOf('0x0000') === 0) {
      e.textContent = '0 FTB';
      e.title = 'no liquidity pools on this network yet — the router is a placeholder. TVL reads live the moment pools exist.';
      return;
    }
    if (!FtbRpc.current()) return;
    var pairs = (CFG.pairs || []);
    if (!pairs.length) { e.textContent = '0 FTB'; return; }
    Promise.all(pairs.map(function (p) {
      return FtbRpc.read(C.router, 'getReserves', [
        { type: 'Hash160', value: resolveTok(p.a) }, { type: 'Hash160', value: resolveTok(p.b) }]).then(function (st) {
        var v = (st && st[0] && st[0].value) || [];
        var ra = BigInt(v[0] && v[0].value || 0), rb = BigInt(v[1] && v[1].value || 0);
        // count the FTB side twice-side style: FTB pairs contribute both legs in FTB terms
        if (p.a === 'FTB') return ra * 2n;
        if (p.b === 'FTB') return rb * 2n;
        return 0n; // non-FTB pairs: no price path without an oracle — excluded by design
      });
    })).then(function (parts) {
      var t = parts.reduce(function (s, v) { return s + v; }, 0n);
      e.textContent = fmt(t) + ' FTB';
      e.title = 'sum of FTB-side pool reserves across ' + pairs.length + ' pair(s) — ratios only, no fiat';
    }).catch(function () { e.textContent = '—'; e.title = 'router set but reserves unreadable'; });
  }

  FtbState.on('rpc', function () {
    FtbRpc.call('getblockcount').then(function (h) { tickBlock = h; paintTicker(); }).catch(function () {});
    refreshRatios();
    refreshTvl();
  });
  FtbState.on('refresh', function () { refreshRatios(); refreshTvl(); });
})();
