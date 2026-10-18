// mod-gov.js — module 5: governance (MODULES.md).
// The live ballot: snapshot-weighted, counted on-chain. Weight is your snapshot
// balance in FLM — 1 FLM = 1 vote — proven with the same merkle proof as the
// claim. Also owns the "weight voted" stat tile.
//
// Owns (writes only these): #vote card (#govQ, #govHint, #govBody, #govDonut,
// #govClock, #govLegend, #govRefresh) and the tile #tVoted.
(function () {
  'use strict';
  if (!FtbUtil.$('vote')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;
  var ballotConfigured = !!(C.ballot && C.ballot.indexOf('0x0000') !== 0 && CFG.ballot && CFG.ballot.question);

  // the 7-day voting window (DECISIONS.md 18 Oct), from config — UI-enforced only;
  // the contract has no expiry, so late votes still land but the page won't offer them.
  var endsMs = ballotConfigured && CFG.ballot.endsAt ? Date.parse(CFG.ballot.endsAt) : NaN;
  function votingOpen() { return !(endsMs > 0) || Date.now() < endsMs; }

  function decodeVoted(st) {
    var it = st && st[0];
    return it && (it.type === 'Boolean' ? it.value : (it.type === 'Integer' ? parseInt(it.value || '0', 10) > 0 : (it.value && it.value !== '')));
  }

  // ---------- pie: option weights vs the snapshot supply (the electorate) ----------
  // One donut, three segments: option 1, option 2, and the unvoted remainder —
  // every slice denominated in the 300,359,297.9466036 FLM snapshot supply.
  var SEG_COLS = ['var(--ink)', 'var(--ink-2)'];
  function renderDonut(vals) {
    var host = $('govDonut'); if (!host) return;
    var elect = (CFG.governance && CFG.governance.electorate) || 0;
    if (!(elect > 0)) { host.innerHTML = ''; return; }
    var electBase = elect * 1e8; // tallies are base units (8 decimals)
    var total = vals.reduce(function (s, v) { return s + v; }, 0n);
    var segs = [], off = 0;
    vals.forEach(function (v, i) {
      var pct = Number(v) / electBase * 100;
      if (pct > 0) {
        segs.push('<circle cx="21" cy="21" r="15.9155" fill="none" stroke="' + SEG_COLS[i % SEG_COLS.length] +
          '" stroke-width="7" stroke-dasharray="' + pct.toFixed(3) + ' ' + (100 - pct).toFixed(3) +
          '" stroke-dashoffset="' + (-off).toFixed(3) + '" transform="rotate(-90 21 21)"/>');
      }
      off += pct;
    });
    var turn = Number(total) / electBase * 100;
    host.innerHTML =
      '<svg viewBox="0 0 42 42" role="img" aria-label="vote split">' +
      '<circle cx="21" cy="21" r="15.9155" fill="none" stroke="var(--hair)" stroke-width="7"/>' +
      segs.join('') +
      '<text x="21" y="20.5" text-anchor="middle" fill="var(--ink)" style="font-family:var(--disp); font-size:5px">' + turn.toFixed(1) + '%</text>' +
      '<text x="21" y="26" text-anchor="middle" fill="var(--faint)" style="font-size:2.2px; letter-spacing:.02em">of supply</text>' +
      '</svg>';
    var leg = $('govLegend'); if (!leg) return;
    leg.innerHTML = '';
    var opts = CFG.ballot.options || [];
    vals.forEach(function (v, i) {
      var pct = Number(v) / electBase * 100;
      var row = el('div', 'govleg-row');
      var left = el('span');
      var sw = el('span', 'sw'); sw.style.background = SEG_COLS[i % SEG_COLS.length];
      left.appendChild(sw);
      left.appendChild(document.createTextNode(opts[i] || ('option ' + (i + 1))));
      row.appendChild(left);
      row.appendChild(el('span', null, pct.toFixed(2) + '%'));
      leg.appendChild(row);
    });
    var un = el('div', 'govleg-row');
    var ul = el('span');
    var usw = el('span', 'sw'); usw.style.background = 'var(--hair)';
    ul.appendChild(usw);
    ul.appendChild(document.createTextNode('not yet voted'));
    un.appendChild(ul);
    un.appendChild(el('span', null, (100 - turn).toFixed(2) + '%'));
    leg.appendChild(un);
    leg.appendChild(el('div', null, 'denominator: ' + elect.toLocaleString('en-US', { maximumFractionDigits: 2 }) + ' FLM snapshot supply'));
  }

  // ---------- countdown to the end of the 7-day window ----------
  function pad2(n) { return (n < 10 ? '0' : '') + n; }
  function tickClock() {
    var t = $('govClock'); if (!t) return;
    if (!ballotConfigured || !(endsMs > 0)) { t.textContent = '—'; return; }
    var ms = endsMs - Date.now();
    if (ms <= 0) {
      t.textContent = 'CLOSED';
      Array.prototype.forEach.call(document.querySelectorAll('#govBody .govote'), function (b) { b.disabled = true; });
      return;
    }
    var sec = Math.floor(ms / 1000);
    var d = Math.floor(sec / 86400); sec -= d * 86400;
    var h = Math.floor(sec / 3600); sec -= h * 3600;
    var m = Math.floor(sec / 60); sec -= m * 60;
    t.textContent = d + 'D ' + pad2(h) + ':' + pad2(m) + ':' + pad2(sec);
  }
  tickClock(); setInterval(tickClock, 1000);

  function refreshGov() {
    var body = $('govBody'); if (!body) return; body.innerHTML = '';
    if (!ballotConfigured) {
      $('govQ').textContent = 'no live ballot';
      $('govHint').textContent = 'a ballot contract is deployed per proposal; its hash is the proposal id.';
      return;
    }
    if (!FtbRpc.current()) return;
    $('govQ').textContent = CFG.ballot.question;
    var opts = CFG.ballot.options || [];
    Promise.all(opts.map(function (_, i) {
      // on-chain options are 1-based (tally keys are byte-wide; 0 faults, FTB-31)
      return FtbRpc.readInt(C.ballot, 'tally', [{ type: 'Integer', value: String(i + 1) }]);
    })).then(function (vals) {
      var total = vals.reduce(function (s, v) { return s + v; }, 0n);
      renderDonut(vals);
      var open = votingOpen();
      body.innerHTML = '';
      vals.forEach(function (v, i) {
        var pct = total > 0n ? (Number(v * 10000n / total) / 100) : 0;
        var opt = el('div', 'gopt');
        var row = el('div', 'grow');
        row.appendChild(el('span', 'gname', '#' + (i + 1) + ' ' + opts[i]));
        row.appendChild(el('span', 'gnum', fmt(v) + ' FLM'));
        var btn = el('button', 'govote', 'vote ->');
        btn.disabled = !open;
        btn.addEventListener('click', function () { voteGov(i + 1, opts[i]); });
        row.appendChild(btn);
        opt.appendChild(row);
        var bar = el('div', 'gbar'); var fill = el('i'); fill.style.width = pct + '%'; bar.appendChild(fill);
        opt.appendChild(bar);
        var meta = el('div', 'gmeta');
        meta.appendChild(el('span', null, pct.toFixed(2) + '% of weight voted'));
        meta.appendChild(el('span', null, 'option ' + (i + 1) + ' on-chain'));
        opt.appendChild(meta);
        body.appendChild(opt);
      });
      // turnout is measured against the snapshot electorate (FLM-equiv, from config —
      // DECISIONS.md 18 Oct), NOT the FTB cap: vote weight is denominated in FLM.
      var elect = (CFG.governance && CFG.governance.electorate) || 0;
      if (elect > 0) {
        var turn = (Number(total) / 1e8 / elect * 100).toFixed(2);
        $('govHint').textContent = 'total weight voted: ' + fmt(total) + ' FLM · turnout ' + turn + '% of the ' + elect.toLocaleString('en-US', { maximumFractionDigits: 2 }) + ' FLM snapshot electorate · tallies are chain state — every node counts the same result.';
      } else {
        $('govHint').textContent = 'total weight voted: ' + fmt(total) + ' FLM · tallies are chain state — every node counts the same result.';
      }
      if (FtbState.account) {
        FtbRpc.read(C.ballot, 'voted', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(function (st) {
          if (decodeVoted(st)) $('govHint').textContent = 'this address has voted · total weight voted: ' + fmt(total) + ' FLM.';
        }).catch(function () {});
      }
    }).catch(function (e) { body.appendChild(el('div', 'small', 'tally unavailable: ' + errText(e))); });
  }

  function voteGov(idx, label) {
    if (!votingOpen()) {
      FtbShell.showPlan('voting closed', 'the 7-day window for this ballot has ended. the tally stands as counted on-chain.', [['question', CFG.ballot.question]], null);
      return;
    }
    if (!FtbState.account) { FtbWallet.needConnect(); return; }
    FtbState.proofs().then(function () {
      var en = FtbState.entryFor(FtbState.account);
      if (!en) {
        FtbShell.showPlan('no vote weight', 'this address is not in the published snapshot, so it carries no vote weight. only pre-exploit holders vote.', [['address', FtbState.account]], null);
        return;
      }
      FtbRpc.read(C.ballot, 'voted', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(function (st) {
        if (decodeVoted(st)) {
          FtbShell.showPlan('already voted', 'one vote per snapshot address — the chain has recorded this address.', [['address', FtbState.account]], null);
          return;
        }
        var plan = FtbState.dapi.ballotVote(FtbState.account, en.amount, idx, en.proof);
        FtbShell.showPlan('vote: ' + label,
          'your weight is your snapshot balance in FLM — 1 FLM = 1 vote — proven with the same merkle proof as your claim. one vote per address, counted on-chain.',
          [['question', CFG.ballot.question],
           ['option', '#' + idx + ' ' + label],
           ['weight', fmt(en.amount) + ' FLM'],
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
  var gb = $('govRefresh'); gb.addEventListener('click', function () { flash(gb); refreshGov(); });

  // ---------- the "weight voted" tile (same tallies, summed) ----------
  function refreshVotedTile() {
    if (!FtbRpc.current()) return;
    if (!(C.ballot && C.ballot.indexOf('0x0000') !== 0 && CFG.ballot && CFG.ballot.options && CFG.ballot.options.length)) return;
    Promise.all(CFG.ballot.options.map(function (_, i) {
      return FtbRpc.readInt(C.ballot, 'tally', [{ type: 'Integer', value: String(i + 1) }]);
    })).then(function (vals) {
      var t = vals.reduce(function (s, v) { return s + v; }, 0n);
      $('tVoted').textContent = fmt(t) + ' FLM';
    }).catch(function () {});
  }

  FtbState.on('rpc', function () { refreshGov(); refreshVotedTile(); });
  FtbState.on('account', refreshGov);
  FtbState.on('refresh', function () { refreshGov(); refreshVotedTile(); });
})();
