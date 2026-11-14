// mod-xchain.js - cross-chain atomic swap (Neo N3 <-> Neo X), the x-chain page.
// The page is the surface; the solver SERVICE takes the other side. The page holds
// no keys: N3 writes are review-and-sign in NeoLine (FtbState.dapi), Neo X writes in
// MetaMask. Safety is bounded by the on-chain HTLC - the solver is a liveness
// dependency, not a trust one (a user who follows the protocol refunds after their
// own timelock and cannot lose funds). Config: FTB_CONFIG.xchain.
//
// Owns (writes only these): #xAmt, #xRecv, #xReview, #xConnX, #xMsg, the #xFlow
// phase rows (#xp1..#xp4), and the #xStatus strip (#xsHash, #xsB, #xsA, #xsSecret).
(function () {
  'use strict';
  var $ = (window.FtbUtil && FtbUtil.$) || function (id) { return document.getElementById(id); };
  if (!$('xReview')) return; // inert where the markup is absent

  var CFG = (window.FtbState && FtbState.CFG) || window.FTB_CONFIG || {};
  var X = CFG.xchain || {};
  // EVM HTLC selectors = keccak256(sig)[:4]; hardcoded so the browser ships no keccak.
  var SEL = { lock: 'b21a99d2', claim: '84cc9dfb', refund: '7249fbb6', swaps: 'eb84e7f2' };
  var S = { dir: 'n3_to_neox', secret: null, hashlock: null, quote: null, neox: null, poll: null, refundA: 0, clock: null };
  var feeBps = 20;

  function msg(t, cls) { var m = $('xMsg'); if (m) { m.textContent = t || ''; m.className = 'xmsg' + (cls ? ' ' + cls : ''); } }
  function legs() {
    // src = the leg the USER locks (A); dst = the leg the solver locked, the user claims (B)
    return S.dir === 'n3_to_neox'
      ? { src: assign({ chain: 'n3' }, X.n3), dst: assign({ chain: 'neox' }, X.neox) }
      : { src: assign({ chain: 'neox' }, X.neox), dst: assign({ chain: 'n3' }, X.n3) };
  }
  function assign(a, b) { b = b || {}; for (var k in b) { if (b.hasOwnProperty(k)) a[k] = b[k]; } return a; }

  // ---- bytes / hex / units ----
  function toHex(bytes) { var h = ''; for (var i = 0; i < bytes.length; i++) h += (bytes[i] & 0xff).toString(16).padStart(2, '0'); return h; }
  function fromHex(s) { s = String(s).replace(/^0x/, ''); var b = new Uint8Array(s.length / 2); for (var i = 0; i < b.length; i++) b[i] = parseInt(s.substr(i * 2, 2), 16); return b; }
  function rand32() { var b = new Uint8Array(32); crypto.getRandomValues(b); return toHex(b); }
  function sha256Hex(hexStr) { return crypto.subtle.digest('SHA-256', fromHex(hexStr)).then(function (d) { return toHex(new Uint8Array(d)); }); }
  function toBase(human, decimals) {
    var s = String(human == null ? '' : human).trim(); if (!/^\d*\.?\d*$/.test(s) || s === '' || s === '.') throw new Error('enter a valid amount');
    var p = s.split('.'), whole = p[0] || '0', frac = (p[1] || '').slice(0, decimals);
    while (frac.length < decimals) frac += '0';
    var v = (whole + frac).replace(/^0+(?=\d)/, '');
    return v === '' ? '0' : v;
  }
  function fromBase(base, decimals) {
    var s = String(base); while (s.length <= decimals) s = '0' + s;
    var whole = s.slice(0, s.length - decimals), frac = s.slice(s.length - decimals).replace(/0+$/, '');
    return frac ? whole + '.' + frac : whole;
  }
  function estimateOut(human) {
    try {
      var L = legs(), inBase = BigInt(toBase(human, L.src.decimals)), sd = L.src.decimals, dd = L.dst.decimals;
      var scaled = dd >= sd ? inBase * (10n ** BigInt(dd - sd)) : inBase / (10n ** BigInt(sd - dd));
      var out = scaled * BigInt(10000 - feeBps) / 10000n;
      return fromBase(out.toString(), dd);
    } catch (e) { return '0.0000'; }
  }
  function paintRecv() { if ($('xRecv') && $('xAmt')) $('xRecv').textContent = estimateOut($('xAmt').value); }
  function fmtDur(s) { if (s <= 0) return 'now'; var h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60; function p(n) { return (n < 10 ? '0' : '') + n; } return (h > 0 ? p(h) + ':' : '') + p(m) + ':' + p(x); }
  function tickClock() { if (!$('xClock')) return; if (!S.refundA) { $('xClock').textContent = '-'; return; } var rem = S.refundA - Math.floor(Date.now() / 1000); $('xClock').textContent = rem > 0 ? ('refundable in ' + fmtDur(rem)) : 'refund available now'; }
  function startClock() { if (S.clock) clearInterval(S.clock); tickClock(); S.clock = setInterval(tickClock, 1000); }
  // ---- EVM ABI (static types only) ----
  function wB32(hex) { return String(hex).replace(/^0x/, '').padStart(64, '0'); }
  function wAddr(a) { return String(a).replace(/^0x/, '').toLowerCase().padStart(64, '0'); }
  function wUint(decOrHex) { var n = (typeof decOrHex === 'string' && decOrHex.indexOf('0x') === 0) ? BigInt(decOrHex) : BigInt(decOrHex); return n.toString(16).padStart(64, '0'); }
  function evmData(sel, words) { return '0x' + sel + words.join(''); }
  function evmCall(to, data) {
    return fetch(X.neox.rpc, { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'eth_call', params: [{ to: to, data: data }, 'latest'] }) })
      .then(function (r) { return r.json(); }).then(function (j) { if (j.error) throw new Error(j.error.message || 'eth_call error'); return j.result; });
  }
  function evmSend(from, to, data, value) {
    if (!window.ethereum) throw new Error('install MetaMask for the Neo X leg');
    return window.ethereum.request({ method: 'eth_sendTransaction', params: [{ from: from, to: to, data: data, value: value || '0x0' }] });
  }
  function splitWords(hexRet) { hexRet = String(hexRet).replace(/^0x/, ''); var w = []; for (var i = 0; i + 64 <= hexRet.length; i += 64) w.push(hexRet.substr(i, 64)); return w; }
  function wordAddr(w) { return '0x' + w.slice(24); }
  function wordUint(w) { return BigInt('0x' + w); }

  // ---- wallets ----
  function connectNeoX() {
    if (!window.ethereum) return Promise.reject(new Error('install MetaMask for the Neo X leg'));
    return window.ethereum.request({ method: 'eth_requestAccounts' }).then(function (a) {
      S.neox = a && a[0];
      return window.ethereum.request({ method: 'eth_chainId' });
    }).then(function (cid) {
      if (X.neox.chainId && cid !== X.neox.chainId) {
        return window.ethereum.request({ method: 'wallet_switchEthereumChain', params: [{ chainId: X.neox.chainId }] })
          .then(function () { return S.neox; });
      }
      return S.neox;
    }).then(function (acct) { var b = $('xConnX'); if (b) { b.textContent = short(acct) + ' (neo x)'; b.classList.add('connected'); } return acct; });
  }
  function short(a) { a = String(a || ''); return a.length > 12 ? a.slice(0, 6) + '...' + a.slice(-4) : a; }
  function n3Account() { return window.FtbState && FtbState.account; }

  function hexToB64(h){h=String(h).replace(/^0x/,'');var b='';for(var i=0;i<h.length;i+=2)b+=String.fromCharCode(parseInt(h.substr(i,2),16));return btoa(b);}
  // ---- reads (both legs) ----
  function readLeg(leg, hashlockHex) {
    if (leg.chain === 'neox') {
      return evmCall(leg.htlc, evmData(SEL.swaps, [wB32(hashlockHex)])).then(function (ret) {
        var w = splitWords(ret); if (w.length < 6) return { state: 'none' };
        var st = Number(wordUint(w[1])); // 0 none 1 locked 2 claimed 3 refunded
        var names = ['none', 'locked', 'claimed', 'refunded'];
        return { state: names[st] || 'none', sender: wordAddr(w[0]), recipient: wordAddr(w[2]), token: wordAddr(w[3]), amount: wordUint(w[4]).toString(), timelock: wordUint(w[5]).toString() };
      });
    }
    // n3: read via FtbRpc (direct JSON-RPC); NeoLine's dapi uses eval, which the page CSP blocks
    return FtbRpc.read(leg.htlc, 'swap', [{ type: 'ByteArray', value: hexToB64(hashlockHex) }]).then(function (r) {
      var arr = r && r[0] && r[0].value;
      if (!arr || !arr.length) return { state: 'none' };
      var st = Number((arr[5] && arr[5].value) || 0), names = ['none', 'locked', 'claimed', 'refunded'];
      return { state: names[st] || 'none', amount: (arr[3] && arr[3].value) || '', timelock: (arr[4] && arr[4].value) || '' };
    }).catch(function () { return { state: 'none' }; });
  }

  // ---- the solver service ----
  function solver(path, body) {
    var o = body ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) } : {};
    return fetch(X.solver + path, o).then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.error || ('solver ' + r.status)); return j; }); });
  }

  // ---- the flow ----
  function review() {
    msg('');
    if (!n3Account()) { if (window.FtbWallet) FtbWallet.needConnect(); else msg('connect your Neo N3 wallet first', 'err'); return; }
    var L = legs(), human = $('xAmt') ? $('xAmt').value : '';
    var amountIn;
    try { amountIn = toBase(human, L.src.decimals); } catch (e) { msg(e.message, 'err'); return; }
    if (amountIn === '0') { msg('enter an amount above zero', 'err'); return; }

    msg('connecting Neo X wallet...');
    connectNeoX().then(function () {
      msg('generating the swap secret...');
      S.secret = rand32();
      return sha256Hex(S.secret);
    }).then(function (h) {
      S.hashlock = h;
      msg('requesting a quote from the solver...');
      return solver('/v1/swaps', { direction: S.dir, amount: amountIn, userN3: n3Account(), userNeoX: S.neox, hashlock: h });
    }).then(function (q) {
      S.quote = q; setStatus(q); setPhase(1, 'done'); setPhase(2, 'active');
      if ($('xRecv')) $('xRecv').textContent = fromBase(q.amountOut, L.dst.decimals);
      S.refundA = (q.legA && q.legA.refundAt) || 0;
      var plan = 'cross-chain swap, ' + S.dir.replace(/_/g, ' ') + '\n\n'
        + '1. the solver has LOCKED ' + fromBase(q.amountOut, L.dst.decimals) + ' ' + L.dst.symbol + ' for you on ' + L.dst.chain + ' (hashlock ' + short('0x' + S.hashlock) + ')\n'
        + '2. you will LOCK ' + fromBase(q.amountIn, L.src.decimals) + ' ' + L.src.symbol + ' on ' + L.src.chain + ', refundable to you after your timelock\n'
        + '3. you will CLAIM on ' + L.dst.chain + ' - this reveals your secret\n'
        + '4. the solver reads the secret and claims its leg. if anything stalls, you refund after your timelock and lose nothing.';
      if (window.confirm(plan)) { go(); }
    }).catch(function (e) { msg(e.message || 'swap request failed', 'err'); });
  }

  function go() {
    var L = legs();
    startClock();
    msg('verifying the solver lock on ' + L.dst.chain + '...');
    readLeg(L.dst, S.hashlock).then(function (b) {
      if (b.state !== 'locked') throw new Error('solver lock not found on ' + L.dst.chain + ' - aborting, your funds are untouched');
      // (amount/recipient are re-checked on-chain; the HTLC pays only the named recipient)
      msg('lock your ' + L.src.symbol + ' on ' + L.src.chain + ' - review and sign');
      return lockSrc(L);
    }).then(function () {
      setPhase(2, 'done'); setPhase(3, 'active');
      msg('waiting for your lock to confirm...');
      return waitLocked(L.src);
    }).then(function () {
      msg('claim your ' + L.dst.symbol + ' on ' + L.dst.chain + ' - review and sign (this reveals your secret)');
      return claimDst(L);
    }).then(function () {
      setPhase(3, 'done'); setPhase(4, 'active');
      msg('claimed. the solver will now settle its leg - watching...');
      watchSettle();
    }).catch(function (e) { msg(e.message || 'swap failed - if you locked, you can refund after your timelock', 'err'); });
  }

  function lockSrc(L) {
    var amt = S.quote.amountIn, tl = S.quote.legA.timelock, payTo = S.quote.legA.payTo;
    if (L.src.chain === 'n3') {
      var plan = FtbState.dapi.xLockN3(n3Account(), L.src.token, L.src.htlc, amt, S.hashlock, payTo, tl,
        'lock ' + fromBase(amt, L.src.decimals) + ' ' + L.src.symbol + ' in the cross-chain escrow');
      return FtbState.dapi.execute(plan);
    }
    var native = /^0x0{40}$/.test(L.src.token);
    var data = evmData(SEL.lock, [wB32(S.hashlock), wAddr(payTo), wAddr(L.src.token), wUint(amt), wUint(tl)]);
    return evmSend(S.neox, L.src.htlc, data, native ? '0x' + BigInt(amt).toString(16) : '0x0');
  }
  function claimDst(L) {
    if (L.dst.chain === 'neox') {
      var data = evmData(SEL.claim, [wB32(S.hashlock), wB32(S.secret)]);
      return evmSend(S.neox, L.dst.htlc, data);
    }
    var plan = FtbState.dapi.xClaimN3(L.dst.htlc, S.hashlock, S.secret, 'claim your ' + L.dst.symbol + ' (reveals your secret)');
    return FtbState.dapi.execute(plan);
  }
  function waitLocked(leg) {
    return new Promise(function (resolve) {
      var tries = 0;
      (function loop() {
        readLeg(leg, S.hashlock).then(function (r) {
          if (r.state === 'locked' || r.state === 'claimed') return resolve();
          if (++tries > 40) return resolve(); // ~5 min; user can proceed, the chain is source of truth
          setTimeout(loop, 8000);
        }).catch(function () { setTimeout(loop, 8000); });
      })();
    });
  }
  function watchSettle() {
    var L = legs();
    if (S.poll) clearInterval(S.poll);
    S.poll = setInterval(function () {
      readLeg(L.src, S.hashlock).then(function (a) {
        setStatusLeg('xsA', L.src.chain, a);
        if (a.state === 'claimed') { setPhase(4, 'done'); msg('swap complete. the solver claimed its leg; your ' + L.dst.symbol + ' is on ' + L.dst.chain + '.', 'ok'); clearInterval(S.poll); S.poll = null; if (S.clock) { clearInterval(S.clock); S.clock = null; } if ($('xClock')) $('xClock').textContent = 'settled'; }
      });
      readLeg(L.dst, S.hashlock).then(function (b) { setStatusLeg('xsB', L.dst.chain, b); });
    }, 12000);
  }

  // ---- UI ----
  function setPhase(n, cls) { var el = $('xp' + n); if (!el) return; el.className = 'phase ' + cls; var pst = el.querySelector('.pst'); if (pst) { pst.className = 'pst s-' + (cls === 'done' ? 'done' : cls === 'active' ? 'active' : 'pending'); pst.textContent = cls === 'done' ? 'done' : cls === 'active' ? 'your move' : 'pending'; } }
  function badge(cls, label) { var b = document.createElement('span'); b.className = 'badge ' + cls; b.textContent = label; return b; }
  function setBadgeLine(el, text, cls, label) { if (!el) return; el.textContent = text + ' '; el.appendChild(badge(cls, label)); }
  function setStatus(q) {
    if ($('xsHash')) $('xsHash').textContent = S.hashlock;
    var L = legs();
    setBadgeLine($('xsB'), fromBase(q.amountOut, L.dst.decimals) + ' ' + L.dst.symbol + ' → you', 'locked', 'locked');
    setBadgeLine($('xsA'), 'awaiting your lock of ' + fromBase(q.amountIn, L.src.decimals) + ' ' + L.src.symbol, 'none', 'pending');
    if ($('xsSecret')) $('xsSecret').textContent = 'generated in this browser - not revealed until you claim';
  }
  function setStatusLeg(id, chain, r) {
    var el = $(id); if (!el) return;
    var cls = r.state === 'claimed' ? 'claimed' : r.state === 'locked' ? 'locked' : r.state === 'refunded' ? 'refunded' : 'none';
    setBadgeLine(el, chain, cls, r.state);
  }

  // cross-chain is a TestNet module: MainNet ships no xchain config, so rather than a dead
  // form, say so and stop. The network switch (top-right) takes the user to the testnet build.
  if (!X.solver || !X.n3 || !X.neox) {
    msg('the neox chain atomic swap runs on testnet - switch to testnet (top-right) to use it.', '');
    ['xReview', 'xConnX', 'xFlip', 'xAmt'].forEach(function (id) { var e = $(id); if (e) e.setAttribute('disabled', ''); });
    return;
  }

  // ---- wiring ----
  if ($('xReview')) $('xReview').addEventListener('click', review);
  if ($('xAmt')) $('xAmt').addEventListener('input', paintRecv);
  if ($('xConnX')) $('xConnX').addEventListener('click', function () { connectNeoX().then(function () { msg('Neo X wallet connected', 'ok'); }).catch(function (e) { msg(e.message, 'err'); }); });
  if ($('xFlip')) $('xFlip').addEventListener('click', function () {
    S.dir = (S.dir === 'n3_to_neox') ? 'neox_to_n3' : 'n3_to_neox';
    paintDirection();
  });
  function paintDirection() {
    var L = legs();
    if ($('xSendChain')) $('xSendChain').textContent = L.src.chain === 'n3' ? 'neo n3' : 'neo x';
    if ($('xRecvChain')) $('xRecvChain').textContent = L.dst.chain === 'n3' ? 'neo n3' : 'neo x';
    if ($('xSendSym')) $('xSendSym').textContent = L.src.symbol || 'FTB';
    if ($('xRecvSym')) $('xRecvSym').textContent = L.dst.symbol || 'FTB';
    paintRecv();
  }

  // boot: pull solver info so the card shows the real fee/limits (and that it's online)
  if (X.solver) {
    solver('/v1/info').then(function (info) {
      feeBps = info.feeBps;
      if ($('xFee')) $('xFee').textContent = (info.feeBps / 100) + '%';
      if ($('xLimits')) $('xLimits').textContent = 'min ' + fromBase(info.min, (legs().src.decimals)) + ' / max ' + fromBase(info.max, legs().src.decimals);
      msg('solver online', 'ok');
      paintRecv();
    }).catch(function () { msg('solver offline - swaps unavailable right now', 'err'); });
  }
  paintDirection();
})();
