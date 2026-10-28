// mod-discord-bond.js - Discord bond module (live). One root (#dbRoot).
//
// Reads go through FtbRpc (raw JSON-RPC, no wallet): live bond amount, ban
// threshold, total bonded stake, your bond status, open accusation cases.
// Writes go through FtbState.dapi + the shared review modal (FtbShell.showPlan):
// post a bond, file a case, vote. No keys here; the wallet signs every write and
// the page shows the exact invocation first. CSP-safe: no inline handlers, no
// innerHTML of untrusted data, values set via textContent / el().
//
// Deep-link hand-off (from the sentinel bot / #verify instructions):
//   #bond?discord=<id>                                  -> prefill the deposit id
//   #file-case?accused=<bondId>&reason=<code>&evidence=<hex>  -> prefill the file form
//   #vote?case=<id>&choice=<liquidate|dismiss>          -> prefill + surface the vote
(function () {
  'use strict';
  var root = document.getElementById('dbRoot');
  if (!root) return;                                   // R2 self-guard
  if (!window.FtbState || !window.FtbUtil) return;     // shared stack absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, short = FtbUtil.short, errText = FtbUtil.errText;
  var C = FtbState.C || {};
  var A = (window.FtbDapi && FtbDapi.args) || null;
  var util = (window.FtbDapi && FtbDapi.util) || null;

  var BOND_STATUS = { 1: 'active', 2: 'refunded', 3: 'liquidated' };
  var CASE_STATUS = { 0: 'open', 1: 'liquidated', 2: 'dismissed' };
  var REASONS = ['unspecified', 'rate / flood', 'near-duplicate', 'mass-mention', 'link-flood', 'invite-spam', 'post-join burst'];

  var bondAmountBase = null; // BigInt, once read from the chain
  var myBondId = null;       // bond id for the connected user's Discord id; set by checkMyBond

  function noop() {}
  function val(id) { var e = $(id); return e ? String(e.value || '').trim() : ''; }
  function setText(id, t) { var e = $(id); if (e) { e.hidden = false; e.textContent = t; } }
  function rpcReady() { return window.FtbRpc && FtbRpc.current(); }
  function need(name) { var h = C[name]; return (h && h.indexOf('0x0000') !== 0) ? h : null; }
  function connected() { return !!(FtbState.account && FtbState.dapi); }
  function nudgeConnect(msgId) {
    if (window.FtbWallet) FtbWallet.needConnect();
    if (msgId) setText(msgId, 'Connect your wallet first - the "connect" button, top right.');
  }

  // ---------------- live reads ----------------
  function refreshStats() {
    var vault = need('dbondvault');
    if (!vault || !rpcReady()) return;
    FtbRpc.readInt(vault, 'bondAmount', []).then(function (v) {
      bondAmountBase = v;
      setText('dbBondAmt', fmt(v) + ' FTB');
      setText('dbBondAmtBig', fmt(v) + ' FTB');
      setText('dbStatBond', fmt(v) + ' FTB');
    }).catch(noop);
    FtbRpc.readInt(vault, 'banBps', []).then(function (v) {
      setText('dbStatBan', (Number(v) / 100) + '% of bonded stake');
    }).catch(noop);
    FtbRpc.readInt(vault, 'totalActiveBondedStake', []).then(function (v) {
      setText('dbStatStake', fmt(v) + ' FTB');
    }).catch(noop);
  }

  function checkMyBond() {
    var vault = need('dbondvault'), id = val('dbDiscord');
    myBondId = null; if ($('dbRefundRow')) $('dbRefundRow').hidden = true;
    if (!rpcReady()) { setText('dbCheckMsg', 'Connecting to a node - try again in a moment.'); return; }
    if (!vault) { setText('dbCheckMsg', 'The bond vault is not configured on this site yet.'); return; }
    if (!/^\d{5,}$/.test(id)) { setText('dbCheckMsg', 'Enter your Discord user id above first (all digits).'); return; }
    setText('dbCheckMsg', 'Reading the chain...');
    FtbRpc.readInt(vault, 'getBondByDiscord', [A.bytesHex(util.asciiToHex(id))]).then(function (bid) {
      if (bid === 0n) { setText('dbCheckMsg', 'No active bond found for Discord id ' + id + '. Post one above, then check again.'); return; }
      return FtbRpc.read(vault, 'getBond', [A.int(bid.toString())]).then(function (st) {
        var v = st && st[0] && st[0].value;
        if (!v || !v.length) { setText('dbCheckMsg', 'Bond #' + bid + ' recorded. Run /verify in #verify to get your role.'); return; }
        var amount = BigInt(v[1].value), status = Number(BigInt(v[3].value));
        setText('dbCheckMsg', 'Bond #' + bid + ' - ' + (BOND_STATUS[status] || ('status ' + status)) +
          ' - ' + fmt(amount) + ' FTB. ' + (status === 1 ? 'Now run /verify in #verify to get your Bonded role.' : ''));
        if (status === 1) { myBondId = bid; if ($('dbRefundRow')) $('dbRefundRow').hidden = false; }
      });
    }).catch(function (e) { setText('dbCheckMsg', 'Could not read your bond: ' + errText(e)); });
  }

  function refreshCases() {
    var ball = need('accusation'), box = $('dbCases');
    if (!box) return;
    if (!ball || !rpcReady()) return;
    FtbRpc.read(ball, 'listOpenCases', []).then(function (st) {
      var arr = (st && st[0] && st[0].value) || [];
      box.textContent = '';
      if (!arr.length) { box.appendChild(el('p', 'bnote', 'No open cases right now. Any bonded member can file one below.')); return; }
      arr.forEach(function (it) { renderCase(ball, BigInt(it.value), box); });
    }).catch(noop);
  }

  function renderCase(ball, id, box) {
    FtbRpc.read(ball, 'getCase', [A.int(id.toString())]).then(function (st) {
      var v = st && st[0] && st[0].value;
      if (!v || v.length < 8) return;
      var accused = BigInt(v[0].value), reason = Number(BigInt(v[1].value)),
          forS = BigInt(v[3].value), againstS = BigInt(v[4].value),
          status = Number(BigInt(v[5].value)), deadline = BigInt(v[6].value), thr = BigInt(v[7].value);
      var total = forS + againstS;
      var liqPct = total > 0n ? Number(forS * 100n / total) : 0;

      var card = el('div', 'bcase');
      var head = el('div', 'bchead');
      head.appendChild(el('b', null, 'accused: bond #' + accused));
      head.appendChild(el('span', 'bcid', 'case #' + id + ' - ' + (CASE_STATUS[status] || status) + ' - closes block ' + deadline));
      card.appendChild(head);
      card.appendChild(el('div', 'bcreason', 'reason: ' + (REASONS[reason] || ('code ' + reason)) +
        ' - threshold to liquidate: ' + fmt(thr) + ' FTB of "liquidate" stake'));

      card.appendChild(voteRow('liquidate', 'liq', liqPct, fmt(forS) + ' FTB'));
      card.appendChild(voteRow('dismiss', 'keep', 100 - liqPct, fmt(againstS) + ' FTB'));

      var row = el('div', 'brow');
      var bLiq = el('button', 'bkey', 'vote liquidate');
      var bKeep = el('button', 'bkey ghost', 'vote dismiss');
      bLiq.type = 'button'; bKeep.type = 'button';
      bLiq.addEventListener('click', function () { startVote(id, accused, true); });
      bKeep.addEventListener('click', function () { startVote(id, accused, false); });
      row.appendChild(bLiq); row.appendChild(bKeep);
      card.appendChild(row);
      box.appendChild(card);
    }).catch(noop);
  }
  function voteRow(name, cls, pct, amt) {
    var r = el('div', 'bvote');
    r.appendChild(el('span', 'bvname', name));
    var wrap = el('span', 'bvwrap'); var bar = el('span', 'bvbar ' + cls);
    bar.style.width = Math.max(0, Math.min(100, pct)) + '%'; wrap.appendChild(bar);
    r.appendChild(wrap); r.appendChild(el('span', 'bvv', amt));
    return r;
  }

  // ---------------- writes (build a plan, hand to the review modal) ----------------
  function startDeposit() {
    if (!window.FtbShell) return;
    if (!connected()) { nudgeConnect('dbDepositMsg'); return; }
    var id = val('dbDiscord');
    if (!/^\d{5,}$/.test(id)) { setText('dbDepositMsg', 'Enter your Discord user id (all digits). In Discord: Settings -> Advanced -> Developer Mode on, then right-click your name -> Copy User ID.'); return; }
    if (bondAmountBase === null) { setText('dbDepositMsg', 'Reading the live bond amount - one moment.'); refreshStats(); return; }
    var plan;
    try { plan = FtbState.dapi.postBond(FtbState.account, bondAmountBase.toString(), id); }
    catch (e) { setText('dbDepositMsg', errText(e)); return; }
    FtbShell.showPlan('post your discord bond',
      'Transfers exactly ' + fmt(bondAmountBase) + ' FTB into the Discord bond vault, tagged with your Discord id so the bot can match it. No admin can move it - only an accusation vote can burn it. Refundable on a clean exit.',
      [['you post', fmt(bondAmountBase) + ' FTB'],
       ['into vault', short(need('dbondvault'))],
       ['tagged to discord id', id],
       ['then', 'run /verify in #verify to get your role'],
       ['what you sign', plan.summary]],
      plan);
    setText('dbDepositMsg', 'Review the panel, then sign in your wallet.');
  }

  function startRefund() {
    if (!window.FtbShell) return;
    if (!connected()) { nudgeConnect('dbRefundMsg'); return; }
    if (myBondId === null) { setText('dbRefundMsg', 'Check your bond above first, then reclaim it.'); return; }
    var plan;
    try { plan = FtbState.dapi.refundBond(myBondId.toString()); }
    catch (e) { setText('dbRefundMsg', errText(e)); return; }
    FtbShell.showPlan('reclaim your discord bond',
      'Refunds bond #' + myBondId + ' back to your wallet and frees your Discord id and wallet to bond again. Works only while the bond is active and not locked by an open accusation case or an open vote.',
      [['bond', '#' + myBondId],
       ['back to', short(FtbState.account)],
       ['what you sign', plan.summary]],
      plan);
    setText('dbRefundMsg', 'Review the panel, then sign in your wallet.');
  }

  function startFile() {
    if (!window.FtbShell) return;
    if (!connected()) { nudgeConnect('dbFileMsg'); return; }
    var accused = val('dbAcc').replace(/^#/, '');
    if (!/^\d+$/.test(accused)) { setText('dbFileMsg', 'Enter the accused bond id (a number, e.g. 41).'); return; }
    var reason = Number(val('dbReason') || '0');
    var evRaw = val('dbEvidence');
    var evHex = /^(0x)?[0-9a-fA-F]{64}$/.test(evRaw) ? evRaw.replace(/^0x/, '')
              : util.sha256hex(evRaw || ('case:' + accused + ':' + Date.now()));
    var plan;
    try { plan = FtbState.dapi.fileCase(FtbState.account, accused, reason, evHex); }
    catch (e) { setText('dbFileMsg', errText(e)); return; }
    FtbShell.showPlan('file an accusation',
      'Opens a dispute case against bond #' + accused + '. Bonded members then vote to liquidate (burn the bond) or dismiss. You must be bonded to file.',
      [['accused bond', '#' + accused],
       ['reason', REASONS[reason] || ('code ' + reason)],
       ['evidence hash', short('0x' + evHex)],
       ['what you sign', plan.summary]],
      plan);
    setText('dbFileMsg', 'Review the panel, then sign in your wallet.');
  }

  function startVote(caseId, accusedBond, liquidate) {
    if (!window.FtbShell) return;
    if (!connected()) { nudgeConnect(null); return; }
    var plan;
    try { plan = FtbState.dapi.voteCase(FtbState.account, caseId.toString(), liquidate); }
    catch (e) { return; }
    FtbShell.showPlan('vote on case #' + caseId,
      'Stake-weighted vote on the accusation against bond #' + accusedBond + '. One vote per bonded wallet; the accused cannot vote. Liquidate burns the bond if the threshold is met and liquidate outweighs dismiss.',
      [['case', '#' + caseId],
       ['accused bond', '#' + accusedBond],
       ['your vote', liquidate ? 'LIQUIDATE (burn)' : 'dismiss (keep)'],
       ['what you sign', plan.summary]],
      plan);
  }

  // ---------------- deep-link hand-off ----------------
  function parseHash() {
    var h = String(location.hash || '').replace(/^#\/?/, '');
    if (!h) return;
    var qi = h.indexOf('?'), path = qi >= 0 ? h.slice(0, qi) : h, qs = qi >= 0 ? h.slice(qi + 1) : '';
    var q = {};
    qs.split('&').forEach(function (kv) { if (!kv) return; var p = kv.split('='); q[decodeURIComponent(p[0])] = decodeURIComponent(p[1] || ''); });
    if (q.discord && $('dbDiscord')) $('dbDiscord').value = q.discord.replace(/[^\d]/g, '');
    if (path.indexOf('file-case') >= 0) {
      if (q.accused && $('dbAcc')) $('dbAcc').value = String(q.accused).replace(/^#/, '');
      if (q.reason && $('dbReason')) $('dbReason').value = String(Number(q.reason) || 0);
      if (q.evidence && $('dbEvidence')) $('dbEvidence').value = q.evidence;
      setText('dbFileMsg', 'Accusation pre-filled from your link. Connect, review, and sign below.');
      var fsec = $('dbAcc'); if (fsec && fsec.scrollIntoView) fsec.scrollIntoView({ behavior: 'smooth', block: 'center' });
    } else if (path.indexOf('vote') >= 0 && q['case']) {
      // surface the targeted case once cases are loaded; the per-case buttons do the signing
      pendingVote = { id: q['case'], choice: (q.choice === 'liquidate') };
    }
  }
  var pendingVote = null;

  // ---------------- live bonds board (#dbWall) ----------------
  // The vault is immutable and exposes no enumeration, but bond ids are dense
  // from 1 and getBond(id) returns every record (even refunded) until the first
  // null, so we walk ids in parallel batches until that null, keep the actives,
  // and render bond # + Discord id (public data only). Read-only, no wallet.
  var WALL_BATCH = 20, WALL_MAX = 400, wallSeq = 0;
  function b64Digits(b64) {
    try { return (window.atob ? atob(b64) : '').replace(/[^\d]/g, ''); }
    catch (e) { return ''; }
  }
  function renderBondWall() {
    var vault = need('dbondvault'), box = $('dbWall');
    if (!box || !vault || !rpcReady()) return;
    var active = [], seq = ++wallSeq;
    (function step(start) {
      if (seq !== wallSeq) return;
      var ids = [], i;
      for (i = start; i < start + WALL_BATCH && i <= WALL_MAX; i++) ids.push(i);
      if (!ids.length) { paintWall(active, true); return; }
      Promise.all(ids.map(function (id) {
        return FtbRpc.read(vault, 'getBond', [A.int(String(id))]).then(function (st) {
          var v = st && st[0] && st[0].value;
          if (!v || !v.length) return { id: id, end: true };
          return { id: id, status: Number(BigInt(v[3].value)), discord: b64Digits(v[2].value) };
        }).catch(function () { return { id: id, err: true }; });
      })).then(function (rows) {
        if (seq !== wallSeq) return;
        var ended = false, errored = false;
        rows.forEach(function (r) {
          if (r.end) { ended = true; return; }
          if (r.err) { errored = true; return; }
          if (r.status === 1 && r.discord) active.push({ id: r.id, discord: r.discord });
        });
        if (ended || errored || start + WALL_BATCH > WALL_MAX) {
          paintWall(active, !ended && !errored);
        } else {
          step(start + WALL_BATCH);
        }
      });
    })(1);
  }
  function paintWall(active, capped) {
    var box = $('dbWall'); if (!box) return;
    active.sort(function (a, b) { return a.id - b.id; });
    box.textContent = '';
    var head = el('div', 'bwrow bwhead');
    head.appendChild(el('span', 'bwnum', 'bond'));
    head.appendChild(el('span', 'bwid', 'discord user id'));
    box.appendChild(head);
    if (!active.length) {
      box.appendChild(el('p', 'bnote', 'No active bonds yet — post the first one above.'));
    } else {
      active.forEach(function (b) {
        var r = el('div', 'bwrow');
        r.appendChild(el('span', 'bwnum', '#' + b.id));
        r.appendChild(el('span', 'bwid', b.discord));
        box.appendChild(r);
      });
      if (capped) box.appendChild(el('p', 'bnote', 'Showing the first ' + WALL_MAX + ' bonds.'));
    }
    var label = active.length + (active.length === 1 ? ' active bond' : ' active bonds');
    setText('dbWallCount', label); setText('dbTopNum', label);
  }

  // ---------------- wiring ----------------
  function on(id, fn) { var e = $(id); if (e) e.addEventListener('click', fn); }
  on('dbDepositBtn', startDeposit);
  on('dbCheckBtn', checkMyBond);
  on('dbRefundBtn', startRefund);
  on('dbFileBtn', startFile);

  function refreshAll() { refreshStats(); refreshCases(); renderBondWall(); }
  FtbState.on('rpc', refreshAll);
  FtbState.on('refresh', refreshAll);
  FtbState.on('account', function () { /* writes re-check connect at click time */ });

  parseHash();
  window.addEventListener('hashchange', parseHash);
  if (rpcReady()) refreshAll(); // in case rpc was already picked before we loaded
})();
