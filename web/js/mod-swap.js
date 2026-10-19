// mod-swap.js - module 1: swap / pools (MODULES.md).
// The swap ticket (external Flamingo router), the pool-ratio board, the market
// ticker, and the TVL tile - ratios only, no fiat, no oracle.
//
// Owns (writes only these): #swap (#swAmt, #swFromSel, #swToSel, #swOut,
// #swMeta, #swGo), #market (#ratioBody, #ratioHint, #ratioBtn), #ticker, and the
// TVL tile (#tTvl, three stacked lines: FTB / NEO / GAS). TVL is protocol-wide: lend pool assets
// headline, NEO/GAS collateral subline, pool reserves add when a router is live.
(function () {
  'use strict';
  if (!FtbUtil.$('swap')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;

  // ---------- pair lookup (cached): the router resolves token pairs to pair contracts ----------
  var pairCache = {};
  function pairOf(a, b) {
    var key = a + '/' + b;
    if (key in pairCache) return Promise.resolve(pairCache[key]);
    return FtbRpc.read(C.router, 'getExchangePairWithAssert', [{ type: 'Hash160', value: a }, { type: 'Hash160', value: b }]).then(function (st) {
      var it = st && st[0];
      if (!it || !it.value) throw new Error('pair not found');
      var h = '0x' + FtbDapi.util.reverseHex(FtbDapi.util.b64ToHex(it.value));
      pairCache[key] = h;
      return h;
    }).catch(function (e) { delete pairCache[key]; throw e; });
  }

  // getReserves returns ONE stack item - a VM Array of two Integers - not two
  // separate items. Unwrap it once here; every reader (quote, ratios, LP) uses this.
  function reservesOf(st) {
    var v = (st && st[0] && st[0].value) || [];
    if (!Array.isArray(v) || v.length < 2 || v[0].value == null) throw new Error('bad reserves shape');
    return [BigInt(v[0].value), BigInt(v[1].value)];
  }

  // a pair that was never deployed faults inside the factory lookup; say which
  // pools DO exist instead of leaking a VM ASSERT at the user.
  function pairErr(e, label) {
    var m = errText(e);
    if (/assert|not found|fault|paircontract/i.test(m)) {
      var live = (CFG.pairs || []).map(function (p) { return p.label; }).join(' · ');
      return 'no pool for ' + label + ' yet - live pools: ' + live;
    }
    return m;
  }

  // ---------- swap ticket (enabled only when the router is configured) ----------
  // NEO is 0 decimals (whole, indivisible units) - every parse/format goes through
  // fmtT/parseT with the token's own decimals; quote math stays in raw base units.
  var TOKENS = { 'NEO': function () { return (CFG.assets || {}).neo; }, 'GAS': function () { return (CFG.assets || {}).gas; }, 'FTB': function () { return C.ftb; } };
  var DEC = { NEO: 0, GAS: 8, FTB: 8 };
  function decOf(sym) { return DEC[sym] != null ? DEC[sym] : 8; }
  function whole(raw, sym) { return Number(raw) / Math.pow(10, decOf(sym)); }
  function fmtT(raw, sym) { return decOf(sym) === 0 ? FtbUtil.grp(raw.toString()) : fmt(raw); }
  function parseT(str, sym) {
    try {
      if (decOf(sym) === 0) { if (!/^\d+$/.test(str)) return null; var n = BigInt(str); return n > 0n ? n : null; }
      var v = BigInt(FtbDapi.util.toBaseUnits(str)); return v > 0n ? v : null;
    } catch (e) { return null; }
  }
  // max-fill string for an input: raw decimal, NO thousands separators (grp output
  // would fail parseT). Round-trips exactly through parseT.
  function fillStr(raw, sym) { return decOf(sym) === 0 ? raw.toString() : FtbDapi.util.fromBaseUnits(raw.toString()); }
  // 0.5% slippage floor. For 0-decimal tokens (NEO) floor() would zero small
  // amounts - no protection at all - so round UP: the min equals the intended
  // whole unit and any drift faults the transaction instead of slipping it.
  function slipMin(amt, sym) {
    return decOf(sym) === 0 ? (amt * 995n + 999n) / 1000n : amt * 995n / 1000n;
  }
  var swFrom = 'GAS', swTo = 'FTB', swQuote = null, qSeq = 0;
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
  // MAX box on the pay row: reads the wallet's live balance of the selected pay
  // asset and fills the input with all of it. Balances already show in the
  // wallet strip above - no duplicate number under the input.
  function fillMax(sym) {
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    if (!FtbRpc.current()) return;
    FtbRpc.readInt(TOKENS[sym](), 'balanceOf', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(function (v) {
      if (sym !== swFrom) return; // user switched the pay asset while reading - discard
      if (v <= 0n) { $('swMeta').textContent = 'no ' + sym + ' in the connected wallet'; return; }
      $('swAmt').value = fillStr(v, sym);
      quote();
    }).catch(function () {});
  }
  var swAmtEl = $('swAmt');
  var maxBox = el('button', 'maxbox', 'max');
  maxBox.type = 'button';
  maxBox.title = 'fill the whole wallet balance of the selected pay asset';
  maxBox.addEventListener('click', function () { fillMax(swFrom); });
  if (swAmtEl && swAmtEl.parentNode) swAmtEl.parentNode.insertBefore(maxBox, swAmtEl.nextSibling);
  // wallet strip boxes (owned by mod-token): a click selects that asset as the
  // pay token and fills the ticket with the full balance - one gesture from
  // "I see my balance" to "ticket ready to quote".
  document.querySelectorAll('#balStrip .wbox').forEach(function (box) {
    box.addEventListener('click', function () {
      var sym = box.getAttribute('data-sym');
      if (!TOKENS[sym]) return;
      if (sym !== swFrom) setFrom(sym);
      fillMax(sym);
    });
  });
  function quote() {
    swQuote = null; $('swOut').value = '';
    var seq = ++qSeq; // stale-response guard: a quote that resolves after the user
    // changed tokens or retyped is discarded - never paint an old pair's numbers
    if (!routerReady()) { $('swMeta').textContent = 'loads when the router hash is configured for this network'; return; }
    var raw = $('swAmt').value.trim();
    if (!raw) { $('swMeta').textContent = 'quote from live pool reserves · 0.3% pool fee · min received at 0.5% slippage'; return; }
    var amtIn = parseT(raw, swFrom);
    if (amtIn === null) { $('swMeta').textContent = decOf(swFrom) === 0 ? 'bad amount - NEO moves in whole units' : 'bad amount'; return; }
    var a = TOKENS[swFrom](), b = TOKENS[swTo]();
    $('swMeta').textContent = 'quoting…';
    FtbRpc.read(C.router, 'getReserves', [{ type: 'Hash160', value: a }, { type: 'Hash160', value: b }]).then(function (st) {
      if (seq !== qSeq) return; // superseded by a newer quote request
      var rr = reservesOf(st), rA = rr[0], rB = rr[1]; // order matches the queried tokens
      if (rA === 0n || rB === 0n) throw new Error('pool empty');
      var out = (amtIn * 997n * rB) / (rA * 1000n + amtIn * 997n);
      var minOut = slipMin(out, swTo);
      var spot = whole(rB, swTo) / whole(rA, swFrom);
      var impact = (1 - (whole(out, swTo) / whole(amtIn, swFrom)) / spot) * 100;
      swQuote = { amtIn: amtIn, out: out, minOut: minOut, rA: rA, rB: rB };
      $('swOut').value = fmtT(out, swTo);
      var m = $('swMeta'); m.innerHTML = '';
      m.appendChild(el('span', null, 'rate 1 ' + swFrom + ' = ' + spot.toPrecision(6) + ' ' + swTo + ' · min received ' + fmtT(minOut, swTo) + ' ' + swTo + ' · '));
      m.appendChild(el('span', impact > 5 ? 'impact-warn' : null, 'price impact ' + impact.toFixed(2) + '%'));
    }).catch(function (e) { if (seq !== qSeq) return; $('swMeta').textContent = 'quote failed: ' + pairErr(e, swFrom + '/' + swTo); });
  }
  var swT = null;
  $('swAmt').addEventListener('input', function () { clearTimeout(swT); swT = setTimeout(quote, 250); });
  $('swGo').addEventListener('click', function () {
    if (!routerReady()) {
      FtbShell.showPlan('swap not wired', 'the router hash is a placeholder in this deployment. the swap contracts are the reused, audited Flamingo Factory/Pair/Router - verify the method and signer scope against the deployed manifest before enabling.',
        [['status', 'router hash not configured']], null);
      return;
    }
    if (!FtbState.account || !FtbState.dapi) { FtbWallet.needConnect(); return; }
    if (!swQuote) { $('swMeta').textContent = 'no quote yet - enter an amount'; return; }
    var a = TOKENS[swFrom](), b = TOKENS[swTo]();
    var deadline = Date.now() + 10 * 60 * 1000;
    // CalledByEntry cannot cross the router boundary: the token contracts witness-check
    // the sender while the caller is the router, so sign with CustomContracts naming
    // the router, both tokens, and the pair.
    $('swMeta').textContent = 'preparing the signature…';
    var q = swQuote; // capture BEFORE quote() re-runs — it nulls swQuote synchronously
    pairOf(a, b).then(function (pair) {
      var allowed = [C.router, a, b]; if (pair) allowed.push(pair);
      var signer = FtbState.dapi.signerFor(FtbState.account, allowed);
      var plan = FtbState.dapi.swap(FtbState.account, q.amtIn.toString(), q.minOut.toString(), [a, b], deadline, signer);
      plan.summary = 'Swap ' + fmtT(q.amtIn, swFrom) + ' ' + swFrom + ' for at least ' + fmtT(q.minOut, swTo) + ' ' + swTo;
      FtbShell.showPlan('swap ' + fmtT(q.amtIn, swFrom) + ' ' + swFrom,
        'external flamingo router. quote from live reserves; min received is 0.5% under the quote. verify the router hash in section IV first.',
        [['you pay', fmtT(q.amtIn, swFrom) + ' ' + swFrom],
         ['you receive (est.)', fmtT(q.out, swTo) + ' ' + swTo],
         ['min received', fmtT(q.minOut, swTo) + ' ' + swTo],
         ['route', swFrom + ' → ' + swTo],
         ['what you sign', plan.summary]],
        plan);
    }).catch(function (e) { $('swMeta').textContent = pairErr(e, swFrom + '/' + swTo); });
  });
  paintSels();

  // ---------- pool ratios (asset vs asset; no fiat, no oracle) ----------
  var tick = {}, tickBlock = null;
  function paintTicker() {
    var t = $('ticker'); if (!t) return;
    var parts = [];
    Object.keys(tick).forEach(function (k) { parts.push('<b>' + k + '</b> ' + tick[k]); });
    if (tickBlock) parts.push('<b>block</b> ' + Number(tickBlock).toLocaleString('en-US'));
    t.innerHTML = parts.length ? '// market&nbsp; ' + parts.join('<span class="sep">·</span>') : '// market: waiting for the pools';
  }
  function resolveTok(t) { return t === 'FTB' ? C.ftb : (t === 'GAS' ? ((CFG.assets || {}).gas) : (t === 'NEO' ? ((CFG.assets || {}).neo) : t)); }
  // ---------- fiat reference rates (Decision 12: display only, never protocol input) ----------
  // Prices come from a public market API (CoinGecko) in the browser and are crossed
  // with the on-chain pool ratio to derive the FTB leg. The contracts never see a
  // fiat figure - ratios remain the only protocol truth. 5-minute cache; if the
  // rate source is unreachable the fiat line simply stays empty.
  var fiatRates = null, fiatAt = 0;
  function refreshFiat() {
    var now = Date.now();
    if (fiatRates && now - fiatAt < 300000) return Promise.resolve(fiatRates);
    try {
      var c = JSON.parse(localStorage.getItem('ftb_fiat') || 'null');
      if (c && c.rates && now - c.at < 300000) { fiatRates = c.rates; fiatAt = c.at; return Promise.resolve(fiatRates); }
    } catch (e) {}
    return fetch('https://api.coingecko.com/api/v3/simple/price?ids=neo,gas,flamingo-finance&vs_currencies=usd,cny')
      .then(function (r) { if (!r.ok) throw new Error('rate source ' + r.status); return r.json(); })
      .then(function (j) {
        fiatRates = {
          GAS: { usd: j.gas.usd, cny: j.gas.cny },
          NEO: { usd: j.neo.usd, cny: j.neo.cny },
          bNEO: { usd: j.neo.usd, cny: j.neo.cny }, // bNEO tracks NEO 1:1
          FLM: { usd: j['flamingo-finance'].usd, cny: j['flamingo-finance'].cny }
        };
        fiatAt = Date.now();
        try { localStorage.setItem('ftb_fiat', JSON.stringify({ at: fiatAt, rates: fiatRates })); } catch (e) {}
        return fiatRates;
      }).catch(function () { return null; }); // ratios still render; fiat stays blank
  }
  function fiatFmt(v, cur) {
    if (!(v > 0)) return '-';
    return cur + (v >= 100 ? v.toFixed(2) : v >= 1 ? v.toFixed(3) : v.toPrecision(3));
  }

  // ---------- the one canonical FTB price (stat-band tile, next to TVL) ----------
  // Each FTB pair derives an FTB fiat figure from its other leg; when pools
  // disagree the tile must not pick whichever card resolved last. The GAS leg
  // wins - GAS is the unit of account on Neo N3 - and the tooltip says which
  // pool derived the number. Reference only; never a protocol input.
  var ftbPx = { gas: null, other: null };
  function paintFtbPx() {
    var e = $('tFtbPx'); if (!e) return;
    var v = ftbPx.gas || ftbPx.other;
    if (!v) return;
    e.textContent = fiatFmt(v.usd, '$') + ' · ' + fiatFmt(v.cny, '¥');
    e.title = '1 FTB - derived from the ' + (ftbPx.gas ? 'GAS/FTB' : 'FTB/NEO') +
      ' pool ratio x the coingecko reference rate. display only, never a protocol input.';
  }
  function noteFtbPx(symA, fiatOfFtb) {
    if (!fiatOfFtb) return;
    if (symA === 'GAS') ftbPx.gas = fiatOfFtb; else ftbPx.other = fiatOfFtb;
    paintFtbPx();
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
      var other = sym[0] === 'FTB' ? sym[1] : sym[0]; // the non-FTB leg
      var card = el('div', 'rcard');
      card.appendChild(el('div', 'rp', sym[0] + ' / ' + sym[1] + ' pool'));
      // pooled reserves: two boxes under the pool title, one per leg, painted
      // from the same getReserves read that feeds the ratio boxes below.
      var prow = el('div', 'lpoolrow');
      var poolA = el('div', 'lpoolbox'); poolA.appendChild(el('div', 'lpl', 'pooled ' + sym[0]));
      var poolAv = el('div', 'lpv', '-'); poolA.appendChild(poolAv); prow.appendChild(poolA);
      var poolB = el('div', 'lpoolbox'); poolB.appendChild(el('div', 'lpl', 'pooled ' + sym[1]));
      var poolBv = el('div', 'lpv', '-'); poolB.appendChild(poolBv); prow.appendChild(poolB);
      card.appendChild(prow);
      var wrap = el('div', 'rwrap');
      var left = el('div', 'rcol');
      left.appendChild(el('div', 'rcolh', 'ratio · price'));
      var num = el('div', 'rbox', 'reading…'); left.appendChild(num);
      var inv = el('div', 'rbox', ''); left.appendChild(inv);
      var usd = el('div', 'rbox', ''); left.appendChild(usd);
      var cny = el('div', 'rbox', ''); left.appendChild(cny);
      wrap.appendChild(left);
      var right = el('div', 'rcol');
      right.appendChild(el('div', 'rcolh', 'change'));
      var pctBox = el('div', 'rchgbox', '-'); right.appendChild(pctBox);
      var arrBox = el('div', 'rchgbox arr', ''); right.appendChild(arrBox);
      wrap.appendChild(right);
      card.appendChild(wrap);
      body.appendChild(card);
      FtbRpc.read(C.router, 'getReserves', [{ type: 'Hash160', value: a }, { type: 'Hash160', value: b }]).then(function (st) {
        var rr = reservesOf(st);
        var rA = whole(rr[0], sym[0]), rB = whole(rr[1], sym[1]);
        if (!(rA > 0 && rB > 0)) throw new Error('pool empty');
        poolAv.textContent = fmtT(rr[0], sym[0]);
        poolBv.textContent = fmtT(rr[1], sym[1]);
        var ratio = rB / rA; // display-only: whole units of B per 1 A
        var h = hist[p.label] || []; h.push(ratio); if (h.length > 40) h = h.slice(h.length - 40);
        hist[p.label] = h;
        try { localStorage.setItem('ftb_ratios', JSON.stringify(hist)); } catch (e) {}
        num.textContent = '1 ' + sym[0] + ' = ' + ratio.toPrecision(6) + ' ' + sym[1];
        inv.textContent = '1 ' + sym[1] + ' = ' + (1 / ratio).toPrecision(6) + ' ' + sym[0];
        num.title = num.textContent; inv.title = inv.textContent; // full precision on hover if ellipsized
        // fiat reference: the leg with a public market rate prices the other leg
        // through the on-chain ratio (FTB has no market rate of its own - derived).
        // the card shows the NON-FTB leg only: FTB's price lives exactly once,
        // in the stat-band tile - never per card, never two different figures.
        var ra = fiatRates && fiatRates[sym[0]], rb = fiatRates && fiatRates[sym[1]];
        if (!rb && ra) rb = { usd: ra.usd / ratio, cny: ra.cny / ratio };
        if (!ra && rb) ra = { usd: rb.usd * ratio, cny: rb.cny * ratio };
        var ro = sym[0] === 'FTB' ? rb : ra;
        usd.textContent = '1 ' + other + ' = ' + (ro ? fiatFmt(ro.usd, '$') : '-');
        cny.textContent = '1 ' + other + ' = ' + (ro ? fiatFmt(ro.cny, '¥') : '-');
        usd.title = usd.textContent; cny.title = cny.textContent;
        // the canonical FTB price tile: FTB leg of this pair feeds it
        if (sym[1] === 'FTB') noteFtbPx(sym[0], rb);
        if (sym[0] === 'FTB') noteFtbPx(sym[1], ra);
        var chg = h.length > 1 ? (ratio / h[0] - 1) * 100 : 0;
        var cls = chg > 0.005 ? 'up' : (chg < -0.005 ? 'dn' : 'flat');
        var dnCls = cls === 'dn' ? ' dn' : '';
        pctBox.className = 'rchgbox' + dnCls;
        pctBox.textContent = (chg >= 0 ? '+' : '-') + Math.abs(chg).toFixed(2) + '%';
        pctBox.title = 'ratio change across ' + h.length + ' reads in this browser';
        arrBox.className = 'rchgbox arr' + dnCls;
        arrBox.textContent = cls === 'up' ? '▲' : cls === 'dn' ? '▼' : '=';
        arrBox.title = pctBox.title;
        tick[p.label] = ratio.toPrecision(6) + ' ' + (cls === 'up' ? '▲' : cls === 'dn' ? '▼' : '=');
        paintTicker();
      }).catch(function (e) {
        num.textContent = '-';
        inv.textContent = e.message || 'unavailable';
      });
    });
  }

  function flash(b) {
    if (!b) return;
    var t = b.textContent; b.textContent = 'reading…'; b.disabled = true;
    setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500);
  }
  var rtb = $('ratioBtn'); rtb.addEventListener('click', function () { flash(rtb); refreshRatios(); });

  // ---------- your liquidity: add / remove LP per configured pair ----------
  // One .lpair card per CFG.pairs entry. Reads: router.getReserves + the pair's own
  // NEP-17 (totalSupply, balanceOf) - the LP token IS the pair contract. Writes go
  // through the router's sender-variant add/removeLiquidity with a CustomContracts
  // signer: the token (and pair) contracts witness-check the sender while the caller
  // is the router, so CalledByEntry cannot cross that boundary.
  var lpStates = [];
  function lpReady() { return $('lpBody') && routerReady() && (CFG.pairs || []).length && FtbRpc.current(); }
  function stat(l) { var d = el('div', 'fs'); d.appendChild(el('div', 'l', l)); var v = el('div', 'v', '-'); d.appendChild(v); return v; }
  function buildLpPanels() {
    var body = $('lpBody'); if (!body || !lpReady()) return;
    if (lpStates.length) return; // built once; refresh paints
    body.innerHTML = '';
    (CFG.pairs || []).forEach(function (p) {
      var sym = p.label.split('/');
      var st = {
        p: p, symA: sym[0], symB: sym[1],
        a: resolveTok(p.a), b: resolveTok(p.b),
        pair: null, rA: 0n, rB: 0n, ts: 0n, lpBal: 0n,
        addQ: null, rmQ: null
      };
      var card = el('div', 'lpair');
      card.appendChild(el('div', 'lpp', p.label + ' pool'));
      var stats = el('div', 'lpstats');
      st.vBal = stat('lp balance'); stats.appendChild(st.vBal.parentNode);
      st.vShare = stat('pool share'); stats.appendChild(st.vShare.parentNode);
      // pooled reserves moved to the tokens-on-FTB card (the getReserves read
      // there already lands both legs); the LP card keeps balance and share.
      card.appendChild(stats);
      st.vYours = el('div', 'lpsub', 'your share: -'); card.appendChild(st.vYours);

      card.appendChild(el('div', 'lplabel', 'add liquidity'));
      var r1 = el('div', 'trow');
      r1.appendChild(el('label', null, st.symA + ' in')); r1.firstChild.htmlFor = 'lpAddA';
      st.inAddA = el('input'); st.inAddA.inputMode = 'decimal'; st.inAddA.autocomplete = 'off'; st.inAddA.placeholder = '0.0'; r1.appendChild(st.inAddA);
      r1.appendChild(FtbUtil.balChip(st.a, st.symA, decOf(st.symA), function (raw) { st.inAddA.value = fillStr(raw, st.symA); quoteAdd(st, 'A'); }));
      card.appendChild(r1);
      var r2 = el('div', 'trow');
      r2.appendChild(el('label', null, st.symB + ' in')); r2.firstChild.htmlFor = 'lpAddB';
      st.inAddB = el('input'); st.inAddB.inputMode = 'decimal'; st.inAddB.autocomplete = 'off'; st.inAddB.placeholder = '0.0'; r2.appendChild(st.inAddB);
      r2.appendChild(FtbUtil.balChip(st.b, st.symB, decOf(st.symB), function (raw) { st.inAddB.value = fillStr(raw, st.symB); quoteAdd(st, 'B'); }));
      card.appendChild(r2);
      st.addMeta = el('div', 'tmeta', 'amounts quote each other from live reserves · mins at 0.5% slippage'); card.appendChild(st.addMeta);
      st.addGo = el('button', 'go', 'add liquidity -&gt;'); st.addGo.textContent = 'add liquidity ->'; card.appendChild(st.addGo);

      card.appendChild(el('div', 'lplabel', 'remove liquidity'));
      var r3 = el('div', 'trow');
      r3.appendChild(el('label', null, 'lp to burn'));
      st.inRm = el('input'); st.inRm.inputMode = 'decimal'; st.inRm.autocomplete = 'off'; st.inRm.placeholder = '0.0'; r3.appendChild(st.inRm);
      // the LP token IS the pair contract - its hash resolves asynchronously, so
      // the chip takes a getter and starts drawing once the pair is known.
      r3.appendChild(FtbUtil.balChip(function () { return st.pair; }, 'LP', 8, function (raw) { st.inRm.value = FtbDapi.util.fromBaseUnits(raw.toString()); quoteRm(st); }));
      card.appendChild(r3);
      st.rmMeta = el('div', 'tmeta', 'burns lp tokens, returns both pool assets · mins at 0.5% slippage'); card.appendChild(st.rmMeta);
      st.rmGo = el('button', 'go', 'remove liquidity -&gt;'); st.rmGo.textContent = 'remove liquidity ->'; card.appendChild(st.rmGo);

      body.appendChild(card);

      var tA = null, tB = null, tR = null;
      st.inAddA.addEventListener('input', function () { clearTimeout(tA); tA = setTimeout(function () { quoteAdd(st, 'A'); }, 250); });
      st.inAddB.addEventListener('input', function () { clearTimeout(tB); tB = setTimeout(function () { quoteAdd(st, 'B'); }, 250); });
      st.inRm.addEventListener('input', function () { clearTimeout(tR); tR = setTimeout(function () { quoteRm(st); }, 250); });
      st.addGo.addEventListener('click', function () { goAdd(st); });
      st.rmGo.addEventListener('click', function () { goRm(st); });

      lpStates.push(st);
    });
  }
  function parseAmt(v) { try { var n = BigInt(FtbDapi.util.toBaseUnits(v)); return n > 0n ? n : null; } catch (e) { return null; } }
  function quoteAdd(st, side) {
    st.addQ = null;
    var rawA = st.inAddA.value.trim(), rawB = st.inAddB.value.trim();
    if (!rawA && !rawB) { st.addMeta.textContent = 'amounts quote each other from live reserves · mins at 0.5% slippage'; return; }
    if (st.rA === 0n || st.rB === 0n) { st.addMeta.textContent = 'pool is empty - the first deposit sets the ratio; add via the seeded pool only'; return; }
    var amtA, amtB;
    if (side === 'A') {
      amtA = parseT(rawA, st.symA); if (amtA === null) { st.addMeta.textContent = rawA ? 'bad ' + st.symA + ' amount' + (decOf(st.symA) === 0 ? ' - whole units only' : '') : 'enter a ' + st.symA + ' amount'; return; }
      amtB = amtA * st.rB / st.rA; st.inAddB.value = fmtT(amtB, st.symB);
    } else {
      amtB = parseT(rawB, st.symB); if (amtB === null) { st.addMeta.textContent = rawB ? 'bad ' + st.symB + ' amount' + (decOf(st.symB) === 0 ? ' - whole units only' : '') : 'enter a ' + st.symB + ' amount'; return; }
      amtA = amtB * st.rA / st.rB; st.inAddA.value = fmtT(amtA, st.symA);
    }
    if (amtA <= 0n || amtB <= 0n) { st.addMeta.textContent = 'amount too small for the current ratio'; return; }
    var minA = slipMin(amtA, st.symA), minB = slipMin(amtB, st.symB);
    var lpEst = st.ts > 0n ? (amtA * st.ts / st.rA < amtB * st.ts / st.rB ? amtA * st.ts / st.rA : amtB * st.ts / st.rB) : 0n;
    st.addQ = { amtA: amtA, amtB: amtB, minA: minA, minB: minB, lpEst: lpEst };
    st.addMeta.textContent = 'rate 1 ' + st.symA + ' = ' + (whole(st.rB, st.symB) / whole(st.rA, st.symA)).toPrecision(6) + ' ' + st.symB +
      ' · you receive ≈ ' + fmt(lpEst) + ' LP · mins ' + fmtT(minA, st.symA) + ' ' + st.symA + ' / ' + fmtT(minB, st.symB) + ' ' + st.symB;
  }
  function quoteRm(st) {
    st.rmQ = null;
    var lp = parseAmt(st.inRm.value.trim());
    if (lp === null) { st.rmMeta.textContent = st.inRm.value.trim() ? 'bad lp amount' : 'burns lp tokens, returns both pool assets · mins at 0.5% slippage'; return; }
    if (st.ts === 0n) { st.rmMeta.textContent = 'pool state not loaded yet'; return; }
    if (st.lpBal > 0n && lp > st.lpBal) { st.rmMeta.textContent = 'more than your lp balance (' + fmt(st.lpBal) + ')'; return; }
    var outA = lp * st.rA / st.ts, outB = lp * st.rB / st.ts;
    var minA = slipMin(outA, st.symA), minB = slipMin(outB, st.symB);
    st.rmQ = { lp: lp, outA: outA, outB: outB, minA: minA, minB: minB };
    st.rmMeta.textContent = 'you receive ≈ ' + fmtT(outA, st.symA) + ' ' + st.symA + ' + ' + fmtT(outB, st.symB) + ' ' + st.symB +
      ' · mins ' + fmtT(minA, st.symA) + ' / ' + fmtT(minB, st.symB);
  }
  function goAdd(st) {
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    if (!st.addQ) { st.addMeta.textContent = 'no quote yet - enter an amount'; return; }
    var deadline = Date.now() + 10 * 60 * 1000;
    var signer = FtbState.dapi.signerFor(FtbState.account, [C.router, st.a, st.b]);
    var plan = FtbState.dapi.addLiquidity(FtbState.account, st.a, st.b,
      st.addQ.amtA.toString(), st.addQ.amtB.toString(), st.addQ.minA.toString(), st.addQ.minB.toString(), deadline, signer);
    plan.summary = 'Add liquidity: ' + fmtT(st.addQ.amtA, st.symA) + ' ' + st.symA + ' + ' + fmtT(st.addQ.amtB, st.symB) + ' ' + st.symB;
    FtbShell.showPlan('add liquidity ' + st.p.label,
      'external flamingo router. both assets move into the pair contract in one transaction; lp tokens mint to your wallet. mins guard 0.5% of drift before confirmation.',
      [[st.symA + ' in', fmtT(st.addQ.amtA, st.symA)],
       [st.symB + ' in', fmtT(st.addQ.amtB, st.symB)],
       ['you receive (est.)', '≈ ' + fmt(st.addQ.lpEst) + ' LP'],
       ['mins', fmtT(st.addQ.minA, st.symA) + ' ' + st.symA + ' / ' + fmtT(st.addQ.minB, st.symB) + ' ' + st.symB],
       ['what you sign', plan.summary]],
      plan);
  }
  function goRm(st) {
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    if (!st.rmQ) { st.rmMeta.textContent = 'no quote yet - enter an lp amount'; return; }
    var deadline = Date.now() + 10 * 60 * 1000;
    st.rmMeta.textContent = 'preparing the signature…';
    pairOf(st.a, st.b).then(function (pair) {
      if (!pair) { st.rmMeta.textContent = 'pair not found on this network'; return; }
      var signer = FtbState.dapi.signerFor(FtbState.account, [C.router, pair]);
      var plan = FtbState.dapi.removeLiquidity(FtbState.account, st.a, st.b,
        st.rmQ.lp.toString(), st.rmQ.minA.toString(), st.rmQ.minB.toString(), deadline, signer);
      plan.summary = 'Remove liquidity: burn ' + fmt(st.rmQ.lp) + ' LP for at least ' +
        fmtT(st.rmQ.minA, st.symA) + ' ' + st.symA + ' + ' + fmtT(st.rmQ.minB, st.symB) + ' ' + st.symB;
      quoteRm(st);
      FtbShell.showPlan('remove liquidity ' + st.p.label,
        'external flamingo router. your lp tokens burn and both pool assets return to your wallet in one transaction. mins guard 0.5% of drift before confirmation.',
        [['lp to burn', fmt(st.rmQ.lp)],
         ['you receive (est.)', '≈ ' + fmtT(st.rmQ.outA, st.symA) + ' ' + st.symA + ' + ' + fmtT(st.rmQ.outB, st.symB) + ' ' + st.symB],
         ['mins', fmtT(st.rmQ.minA, st.symA) + ' ' + st.symA + ' / ' + fmtT(st.rmQ.minB, st.symB) + ' ' + st.symB],
         ['what you sign', plan.summary]],
        plan);
    }).catch(function (e) { st.rmMeta.textContent = 'pair lookup failed: ' + errText(e); });
  }
  function refreshLp() {
    if (!lpReady()) return;
    buildLpPanels();
    lpStates.forEach(function (st) {
      var h160 = function (h) { return { type: 'Hash160', value: h }; };
      FtbRpc.read(C.router, 'getReserves', [h160(st.a), h160(st.b)]).then(function (rs) {
        var rr = reservesOf(rs);
        st.rA = rr[0]; st.rB = rr[1];
        return pairOf(st.a, st.b);
      }).then(function (pair) {
        st.pair = pair;
        return Promise.all([
          FtbRpc.readInt(pair, 'totalSupply', []),
          FtbState.account ? FtbRpc.readInt(pair, 'balanceOf', [h160(FtbState.acctHash160())]) : Promise.resolve(0n)
        ]);
      }).then(function (rr) {
        st.ts = rr[0]; st.lpBal = rr[1];
        if (FtbState.account) {
          st.vBal.textContent = fmt(st.lpBal);
          var share = st.ts > 0n ? Number(st.lpBal * 10000n / st.ts) / 100 : 0;
          st.vShare.textContent = share.toFixed(2) + '%';
          var yA = st.ts > 0n ? st.lpBal * st.rA / st.ts : 0n, yB = st.ts > 0n ? st.lpBal * st.rB / st.ts : 0n;
          st.vYours.textContent = 'your share: ' + fmtT(yA, st.symA) + ' ' + st.symA + ' · ' + fmtT(yB, st.symB) + ' ' + st.symB;
        } else {
          st.vBal.textContent = '-'; st.vShare.textContent = '-'; st.vYours.textContent = 'connect a wallet to see your share';
        }
      }).catch(function (e) {
        st.vYours.textContent = 'pool read failed: ' + errText(e);
      });
    });
  }
  var lpb = $('lpRefresh'); if (lpb) lpb.addEventListener('click', function () { flash(lpb); refreshLp(); });

  // ---------- TVL: total value locked, per asset, read from the chain ----------
  // FTB headline: the lend pool's total assets (cash in the contract + outstanding
  // borrows); pool reserves add on top once a router is live. NEO / GAS subline:
  // what the lend contract holds - that IS the total collateral locked (one balance
  // read each, no enumeration). No fiat conversion anywhere (the invariant stands).
  function refreshTvl() {
    var e = $('tTvl'); if (!e) return;
    if (!FtbRpc.current()) return;
    var lend = C.lend, hasLend = !!(lend && lend.indexOf('0x0000') !== 0);
    var router = C.router, hasRouter = !!(router && router.indexOf('0x0000') !== 0);
    if (!hasLend && !hasRouter) {
      e.innerHTML = '<div class="tnline">0 FTB</div>';
      e.title = 'no lend module or router on this network yet - TVL reads live the moment value is locked.';
      return;
    }
    var h160 = function (h) { return { type: 'Hash160', value: h }; };
    var assets = CFG.assets || {};
    var all = [], tags = [];
    function job(tag, p) { all.push(p.catch(function () { return null; })); tags.push(tag); }
    if (hasLend) {
      job('ftb', FtbRpc.readInt(C.ftb, 'balanceOf', [h160(lend)]));
      job('ftb', FtbRpc.readInt(lend, 'totalBorrows', []));
      if (assets.neo) job('neo', FtbRpc.readInt(assets.neo, 'balanceOf', [h160(lend)]));
      if (assets.gas) job('gas', FtbRpc.readInt(assets.gas, 'balanceOf', [h160(lend)]));
    }
    if (hasRouter) {
      (CFG.pairs || []).forEach(function (p) {
        job('ftb', FtbRpc.read(router, 'getReserves', [h160(resolveTok(p.a)), h160(resolveTok(p.b))]).then(function (st) {
          var v = (st && st[0] && st[0].value) || [];
          var ra = BigInt(v[0] && v[0].value || 0), rb = BigInt(v[1] && v[1].value || 0);
          // FTB pairs contribute both legs in FTB terms; non-FTB pairs have no
          // price path without an oracle - excluded by design
          if (p.a === 'FTB') return ra * 2n;
          if (p.b === 'FTB') return rb * 2n;
          return 0n;
        }));
      });
    }
    Promise.all(all).then(function (rs) {
      var ftb = 0n, neo = 0n, gas = 0n, failed = false;
      rs.forEach(function (r, i) {
        if (r === null) { failed = true; return; }
        if (tags[i] === 'ftb') ftb += r;
        else if (tags[i] === 'neo') neo = r;
        else gas = r;
      });
      // three stacked lines, one size, one color: FTB headline, then the NEO
      // and GAS locked as loan collateral. exact figures in the tooltip.
      function short2(base) { var f = fmt(base).split('.'); return f.length > 1 && f[1].length > 2 ? f[0] + '.' + f[1].slice(0, 2) : f.join('.'); }
      e.innerHTML = '';
      e.appendChild(el('div', 'tnline', short2(ftb) + ' FTB'));
      if (hasLend) {
        e.appendChild(el('div', 'tnline', FtbUtil.grp(neo.toString()) + ' NEO'));
        e.appendChild(el('div', 'tnline', short2(gas) + ' GAS'));
      }
      e.title = 'total value locked, per asset: FTB in the lend pool (cash + outstanding borrows)' +
        (hasRouter ? ' + FTB-side pool reserves' : '') +
        (hasLend ? '; NEO and GAS locked as loan collateral' : '') +
        '. ratios only, no fiat.' + (failed ? ' (one leg unreadable right now)' : '');
    }).catch(function () { e.innerHTML = '<div class="tnline">-</div>'; e.title = 'TVL unreadable'; });
  }

  FtbState.on('rpc', function () {
    FtbRpc.call('getblockcount').then(function (h) { tickBlock = h; paintTicker(); }).catch(function () {});
    refreshFiat().then(function () { refreshRatios(); });
    refreshTvl();
    refreshLp();
  });
  FtbState.on('account', function () { refreshLp(); });
  FtbState.on('refresh', function () {
    refreshFiat().then(function () { refreshRatios(); });
    refreshTvl(); refreshLp();
    // a confirmed write clears the ticket - no stale amount suggesting nothing happened
    $('swAmt').value = ''; $('swOut').value = '';
    lpStates.forEach(function (st) {
      st.inAddA.value = ''; st.inAddB.value = ''; st.inRm.value = '';
      st.addQ = null; st.rmQ = null;
    });
  });
})();
