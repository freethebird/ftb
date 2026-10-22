// mod-gov.js - module 5: governance (MODULES.md). v0.11 - serialized ballot menu.
// The live ballots rendered as a disclosure menu: one row per ballot (number,
// question, running result, and whether the connected wallet voted), opening to
// that ballot's own option bars, turnout donut, and vote options. Weight is your
// snapshot balance in FLM (1 FLM = 1 vote), proven with the same merkle proof as
// the claim. Also owns the stat-band "active ballots" tile (#tBallots).
//
// Owns (writes only these): #vote card - #ballotMenu, #govRefresh - and the
// stat tile #tBallots. The single-ballot ids (#govQ/#govHint/#govBody/#govDonut/
// #govClock/#govLegend) are gone; the ballot set drives everything now.
(function () {
  'use strict';
  if (!FtbUtil.$('vote')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;

  // ---------- the ballot set: CFG.ballots[], with the legacy single-ballot fallback ----------
  function ballots() {
    if (CFG.ballots && CFG.ballots.length) return CFG.ballots.filter(function (b) { return b.hash && b.question; });
    if (C.ballot && C.ballot.indexOf('0x0000') !== 0 && CFG.ballot && CFG.ballot.question) {
      return [{ hash: C.ballot, question: CFG.ballot.question, options: CFG.ballot.options, endsAt: CFG.ballot.endsAt }];
    }
    return [];
  }
  var BL = ballots();
  var ballotConfigured = BL.length > 0;

  // the voting window (DECISIONS.md, Decision 14), from config - UI-enforced only;
  // the contracts have no expiry, so late votes still land on-chain but the page
  // won't offer them.
  var MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function endsMsOf(b) { return b.endsAt ? Date.parse(b.endsAt) : NaN; }
  function ballotOpen(b) { var ms = endsMsOf(b); return !(ms > 0) || Date.now() < ms; }
  function endsLabel(b) { var ms = endsMsOf(b); if (!(ms > 0)) return ''; var d = new Date(ms); return 'ends ' + d.getUTCDate() + ' ' + MON[d.getUTCMonth()]; }

  function decodeVoted(st) {
    var it = st && st[0];
    return it && (it.type === 'Boolean' ? it.value : (it.type === 'Integer' ? parseInt(it.value || '0', 10) > 0 : (it.value && it.value !== '')));
  }

  function electBase() {
    var elect = (CFG.governance && CFG.governance.electorate) || 0;
    return elect > 0 ? elect * 1e8 : 0; // tallies are base units (8 decimals)
  }

  var SEG = ['var(--ink)', 'var(--ink-2)']; // option 1 dark, option 2 muted

  // one donut per ballot: each option's share of the snapshot electorate; the
  // hair ring is the unvoted remainder, so the filled arc IS the turnout. The
  // dasharray numbers are computed floats (not untrusted text), so the SVG is
  // assembled as a constant-shaped string - same pattern as the legacy donut.
  function donutSvg(vals, turn) {
    var eb = electBase(), off = 0, arcs = '';
    vals.forEach(function (v, i) {
      var pct = eb ? Number(v) / eb * 100 : 0;
      if (pct > 0) {
        arcs += '<circle cx="21" cy="21" r="15.9155" fill="none" stroke="' + SEG[i % SEG.length] +
          '" stroke-width="6" stroke-dasharray="' + pct.toFixed(2) + ' ' + (100 - pct).toFixed(2) +
          '" stroke-dashoffset="' + (-off).toFixed(2) + '" transform="rotate(-90 21 21)"/>';
      }
      off += pct;
    });
    return '<svg class="bdonut" viewBox="0 0 42 42" role="img" aria-label="' + turn.toFixed(1) + '% turnout">' +
      '<circle cx="21" cy="21" r="15.9155" fill="none" stroke="var(--hair)" stroke-width="6"/>' + arcs + '</svg>';
  }

  // build one <details> ballot row from its live tallies
  function ballotRow(b, idx, vals, voted, isFirst) {
    var eb = electBase();
    var total = vals.reduce(function (s, v) { return s + v; }, 0n);
    var turn = eb ? Number(total) / eb * 100 : 0;
    var opts = b.options || [];
    // share of turnout per option, and the leading option
    var shares = vals.map(function (v) { return total > 0n ? Number(v * 10000n / total) / 100 : 0; });
    var lead = 0; shares.forEach(function (p, i) { if (p > shares[lead]) lead = i; });
    function pctStr(p) { return (p >= 99.95 ? '100' : (Math.round(p * 10) / 10)) + '%'; }

    var d = el('details', 'poolrow'); if (isFirst) d.setAttribute('open', '');
    var sm = el('summary');
    sm.appendChild(el('span', 'bnum', ('00' + (idx + 1)).slice(-3)));
    sm.appendChild(el('span', 'bq', b.question));
    // result: leader (bold) · runner-up, built with DOM so option labels stay text
    var res = el('span', 'bres');
    opts.forEach(function (o, i) {
      if (i) res.appendChild(document.createTextNode(' · '));
      var seg = i === lead ? el('b') : el('span');
      seg.textContent = o + ' ' + pctStr(shares[i]);
      res.appendChild(seg);
    });
    sm.appendChild(res);
    var did = el('span', 'bdid' + (voted ? ' voted' : ''), voted ? '✓ you voted' : '— not voted');
    sm.appendChild(did);
    sm.appendChild(el('span', 'pr-caret', '▸')).setAttribute('aria-hidden', 'true');
    d.appendChild(sm);

    var det = el('div', 'pr-detail bdetail');
    var bo = el('div', 'bopts');
    opts.forEach(function (o, i) {
      var row = el('div', 'bopt');
      row.appendChild(el('span', 'bo-l', o));
      var bar = el('span', 'bo-bar'); var fill = el('i'); fill.style.width = shares[i] + '%'; bar.appendChild(fill);
      row.appendChild(bar);
      row.appendChild(el('span', 'bo-p', pctStr(shares[i])));
      bo.appendChild(row);
    });
    det.appendChild(bo);

    var side = el('div', 'bside');
    var don = el('div'); don.innerHTML = donutSvg(vals, turn); side.appendChild(don.firstChild);
    var bt = el('div', 'bturn');
    bt.appendChild(el('b', null, turn.toFixed(1) + '%'));
    bt.appendChild(document.createTextNode(' turnout'));
    bt.appendChild(document.createElement('br'));
    var sub = el('span', 'muted2', 'of snapshot' + (endsLabel(b) ? ' · ' + endsLabel(b) : ''));
    bt.appendChild(sub);
    side.appendChild(bt);
    det.appendChild(side);

    var act = el('div', 'brow-act');
    var open = ballotOpen(b);
    opts.forEach(function (o, i) {
      var btn = el('button', null, 'vote ' + o + ' →'); btn.disabled = !open;
      btn.addEventListener('click', function () { voteGov(b, i + 1, o); });
      act.appendChild(btn);
    });
    det.appendChild(act);
    d.appendChild(det);
    return d;
  }

  // compact a long option label for the glance tile (the full label stays in the menu):

  // the stat-band "active ballots" tile: one line per ballot, leader bright / runner-up muted
  function paintBallotsTile(all) {
    var host = $('tBallots'); if (!host) return;
    host.innerHTML = '';
    BL.forEach(function (b, bi) {
      var vals = all[bi] || [];
      var total = vals.reduce(function (s, v) { return s + v; }, 0n);
      var shares = vals.map(function (v) { return total > 0n ? Number(v * 10000n / total) / 100 : 0; });
      var lead = 0; shares.forEach(function (p, i) { if (p > shares[lead]) lead = i; });
      var other = lead === 0 ? 1 : 0;
      function ps(p) { return (p >= 99.95 ? '100' : (Math.round(p * 10) / 10)) + '%'; }
      var line = el('div', 'tnline');
      line.appendChild(document.createTextNode(('00' + (bi + 1)).slice(-3) + ' ' + ps(shares[lead]) + ' '));
      line.appendChild(el('span', 'ag', '/ ' + ps(shares[other] || 0)));
      host.appendChild(line);
    });
  }

  function refreshGov() {
    var menu = $('ballotMenu'); if (!menu) return;
    if (!ballotConfigured) {
      menu.innerHTML = '';
      menu.appendChild(el('div', 'small muted', 'no live ballot - a ballot contract is deployed per proposal; its hash is the proposal id.'));
      return;
    }
    if (!FtbRpc.current()) return;
    // tally reads: every option of every ballot (on-chain options are 1-based)
    Promise.all(BL.map(function (b) {
      return Promise.all((b.options || []).map(function (_, i) {
        return FtbRpc.readInt(b.hash, 'tally', [{ type: 'Integer', value: String(i + 1) }]);
      }));
    })).then(function (all) {
      // did the connected wallet vote in each? (absent wallet -> all false)
      var votedP = FtbState.account
        ? Promise.all(BL.map(function (b) {
            return FtbRpc.read(b.hash, 'voted', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(decodeVoted).catch(function () { return false; });
          }))
        : Promise.resolve(BL.map(function () { return false; }));
      votedP.then(function (flags) {
        menu.innerHTML = '';
        BL.forEach(function (b, bi) { menu.appendChild(ballotRow(b, bi, all[bi], flags[bi], bi === 0)); });
        paintBallotsTile(all);
      });
    }).catch(function (e) {
      menu.innerHTML = ''; menu.appendChild(el('div', 'small', 'tally unavailable: ' + errText(e)));
    });
  }

  function voteGov(b, idx, label) {
    if (!ballotOpen(b)) {
      FtbShell.showPlan('voting closed', 'the window for this ballot has ended. the tally stands as counted on-chain.', [['question', b.question]], null);
      return;
    }
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    FtbState.proofs().then(function () {
      var en = FtbState.entryFor(FtbState.account);
      if (!en) {
        FtbShell.showPlan('no vote weight', 'this address is not in the published snapshot, so it carries no vote weight. only pre-exploit holders vote.', [['address', FtbState.account]], null);
        return;
      }
      FtbRpc.read(b.hash, 'voted', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(function (st) {
        if (decodeVoted(st)) {
          FtbShell.showPlan('already voted', 'one vote per snapshot address per ballot - the chain has recorded this address.', [['address', FtbState.account], ['question', b.question]], null);
          return;
        }
        var plan = FtbState.dapi.ballotVote(FtbState.account, en.amount, idx, en.proof, b.hash);
        FtbShell.showPlan('vote: ' + label,
          'your weight is your snapshot balance in FLM - 1 FLM = 1 vote - proven with the same merkle proof as your claim. one vote per address per ballot, counted on-chain.',
          [['question', b.question],
           ['option', '#' + idx + ' ' + label],
           ['weight', fmt(en.amount) + ' FLM'],
           ['ballot contract', b.hash],
           ['what you sign', plan.summary]],
          plan);
      });
    }).catch(function (e) {
      FtbShell.showPlan('vote unavailable', errText(e), [], null);
    });
  }

  function flash(b) {
    if (!b) return;
    var t = b.textContent; b.textContent = 'reading…'; b.disabled = true;
    setTimeout(function () { b.textContent = t; b.disabled = false; }, 1500);
  }
  var gb = $('govRefresh'); if (gb) gb.addEventListener('click', function () { flash(gb); refreshGov(); });

  FtbState.on('rpc', refreshGov);
  FtbState.on('account', refreshGov);
  FtbState.on('refresh', refreshGov);
})();
