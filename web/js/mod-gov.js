// mod-gov.js — module 5: governance (MODULES.md).
// The live ballots: snapshot-weighted, counted on-chain, one contract per proposal —
// the hash IS the proposal id. Weight is your snapshot balance in FLM (1 FLM = 1 vote),
// proven with the same merkle proof as the claim. Also owns the "weight voted" stat tile.
//
// Owns (writes only these): #vote card (#govQ, #govHint, #govBody, #govDonut,
// #govClock, #govLegend, #govRefresh) and the tile #tVoted.
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

  // the voting window (DECISIONS.md, Decision 14: genesis ballots close 31 Oct), from
  // config — UI-enforced only; the contracts have no expiry, so late votes still land
  // on-chain but the page won't offer them.
  function endsMsOf(b) { return b.endsAt ? Date.parse(b.endsAt) : NaN; }
  function ballotOpen(b) { var ms = endsMsOf(b); return !(ms > 0) || Date.now() < ms; }
  var clockEndsMs = ballotConfigured ? Math.max.apply(null, BL.map(function (b) { return endsMsOf(b) || 0; })) : NaN;

  function decodeVoted(st) {
    var it = st && st[0];
    return it && (it.type === 'Boolean' ? it.value : (it.type === 'Integer' ? parseInt(it.value || '0', 10) > 0 : (it.value && it.value !== '')));
  }

  function electBase() {
    var elect = (CFG.governance && CFG.governance.electorate) || 0;
    return elect > 0 ? elect * 1e8 : 0; // tallies are base units (8 decimals)
  }

  // ---------- one donut per ballot: option weights vs the snapshot supply ----------
  // Segments are the options; the ring background is the unvoted remainder — every
  // slice denominated in the 526,782,179.10 FLM snapshot supply.
  var SEG_COLS = ['var(--ink)', 'var(--ink-2)'];
  function donutSvg(vals, label) {
    var eb = electBase();
    var total = vals.reduce(function (s, v) { return s + v; }, 0n);
    var segs = [], off = 0;
    vals.forEach(function (v, i) {
      var pct = eb ? Number(v) / eb * 100 : 0;
      if (pct > 0) {
        segs.push('<circle cx="21" cy="21" r="15.9155" fill="none" stroke="' + SEG_COLS[i % SEG_COLS.length] +
          '" stroke-width="7" stroke-dasharray="' + pct.toFixed(3) + ' ' + (100 - pct).toFixed(3) +
          '" stroke-dashoffset="' + (-off).toFixed(3) + '" transform="rotate(-90 21 21)"/>');
      }
      off += pct;
    });
    var turn = eb ? Number(total) / eb * 100 : 0;
    return '<div style="margin-bottom:6px">' +
      '<svg viewBox="0 0 42 42" role="img" aria-label="vote split — ' + label + '" style="max-width:120px">' +
      '<circle cx="21" cy="21" r="15.9155" fill="none" stroke="var(--hair)" stroke-width="7"/>' +
      segs.join('') +
      '<text x="21" y="20.5" text-anchor="middle" fill="var(--ink)" style="font-family:var(--disp); font-size:5px">' + turn.toFixed(1) + '%</text>' +
      '<text x="21" y="26" text-anchor="middle" fill="var(--faint)" style="font-size:2.2px; letter-spacing:.02em">of supply</text>' +
      '</svg>' +
      '<div class="small muted" style="font-size:11px; letter-spacing:.08em; text-transform:uppercase">' + label + '</div>' +
      '</div>';
  }

  function renderDonuts(allVals) {
    var host = $('govDonut'); if (!host) return;
    if (!electBase()) { host.innerHTML = ''; return; }
    host.innerHTML = BL.map(function (b, i) {
      return donutSvg(allVals[i] || [], '#' + (i + 1));
    }).join('');
    var leg = $('govLegend'); if (!leg) return;
    var opts = (BL[0] && BL[0].options) || [];
    leg.innerHTML = '';
    opts.forEach(function (o, i) {
      var row = el('div', 'govleg-row');
      var left = el('span');
      var sw = el('span', 'sw'); sw.style.background = SEG_COLS[i % SEG_COLS.length];
      left.appendChild(sw);
      left.appendChild(document.createTextNode('option ' + (i + 1) + ' (dark share of supply)'));
      row.appendChild(left);
      leg.appendChild(row);
    });
    leg.appendChild(el('div', null, 'denominator: ' + ((CFG.governance && CFG.governance.electorate) || 0).toLocaleString('en-US', { maximumFractionDigits: 2 }) + ' FLM snapshot supply · same electorate for every ballot'));
  }

  // ---------- countdown to the end of the window (shared deadline across ballots) ----------
  function pad2(n) { return (n < 10 ? '0' : '') + n; }
  function tickClock() {
    var t = $('govClock'); if (!t) return;
    if (!ballotConfigured || !(clockEndsMs > 0)) { t.textContent = '—'; return; }
    var ms = clockEndsMs - Date.now();
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
    $('govQ').textContent = BL.length === 1 ? BL[0].question : BL.length + ' live ballots';
    // tally reads: every option of every ballot (on-chain options are 1-based —
    // tally keys are byte-wide; 0 faults, FTB-31)
    Promise.all(BL.map(function (b) {
      return Promise.all((b.options || []).map(function (_, i) {
        return FtbRpc.readInt(b.hash, 'tally', [{ type: 'Integer', value: String(i + 1) }]);
      }));
    })).then(function (allVals) {
      renderDonuts(allVals);
      body.innerHTML = '';
      BL.forEach(function (b, bi) {
        var vals = allVals[bi];
        var total = vals.reduce(function (s, v) { return s + v; }, 0n);
        var open = ballotOpen(b);
        var sec = el('div');
        sec.style.cssText = bi > 0 ? 'margin-top:18px; border-top:1px solid var(--hair); padding-top:12px' : '';
        sec.appendChild(el('h3', 'disp', '#' + (bi + 1) + ' ' + b.question)).style.cssText = 'font-size:19px; margin:.1em 0 .1em';
        var hint = el('p', 'small muted');
        var eb = electBase();
        var turn = eb ? (Number(total) / eb * 100).toFixed(2) : '0.00';
        hint.textContent = 'weight voted: ' + fmt(total) + ' FLM · turnout ' + turn + '%' + (open ? '' : ' · window closed');
        sec.appendChild(hint);
        vals.forEach(function (v, i) {
          var pct = total > 0n ? (Number(v * 10000n / total) / 100) : 0;
          var opt = el('div', 'gopt');
          var row = el('div', 'grow');
          row.appendChild(el('span', 'gname', '#' + (i + 1) + ' ' + b.options[i]));
          row.appendChild(el('span', 'gnum', fmt(v) + ' FLM'));
          var btn = el('button', 'govote', 'vote ->');
          btn.disabled = !open;
          btn.addEventListener('click', function () { voteGov(b, i + 1, b.options[i]); });
          row.appendChild(btn);
          opt.appendChild(row);
          var bar = el('div', 'gbar'); var fill = el('i'); fill.style.width = pct + '%'; bar.appendChild(fill);
          opt.appendChild(bar);
          var meta = el('div', 'gmeta');
          meta.appendChild(el('span', null, pct.toFixed(2) + '% of weight voted'));
          meta.appendChild(el('span', null, 'option ' + (i + 1) + ' on-chain'));
          opt.appendChild(meta);
          sec.appendChild(opt);
        });
        body.appendChild(sec);
      });
      // turnout is measured against the snapshot electorate (FLM-equiv, from config),
      // NOT the FTB cap: vote weight is denominated in FLM.
      var elect = (CFG.governance && CFG.governance.electorate) || 0;
      $('govHint').textContent = BL.length + ' ballots · electorate ' + elect.toLocaleString('en-US', { maximumFractionDigits: 2 }) + ' FLM snapshot weight · tallies are chain state — every node counts the same result.';
      if (FtbState.account) {
        Promise.all(BL.map(function (b) {
          return FtbRpc.read(b.hash, 'voted', [{ type: 'Hash160', value: FtbState.acctHash160() }]).then(function (st) { return decodeVoted(st); }).catch(function () { return false; });
        })).then(function (votedFlags) {
          var n = votedFlags.filter(Boolean).length;
          if (n > 0) $('govHint').textContent = 'this address has voted in ' + n + ' of ' + BL.length + ' ballots · ' + $('govHint').textContent;
        });
      }
    }).catch(function (e) { body.appendChild(el('div', 'small', 'tally unavailable: ' + errText(e))); });
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
          FtbShell.showPlan('already voted', 'one vote per snapshot address per ballot — the chain has recorded this address.', [['address', FtbState.account], ['question', b.question]], null);
          return;
        }
        var plan = FtbState.dapi.ballotVote(FtbState.account, en.amount, idx, en.proof, b.hash);
        FtbShell.showPlan('vote: ' + label,
          'your weight is your snapshot balance in FLM — 1 FLM = 1 vote — proven with the same merkle proof as your claim. one vote per address per ballot, counted on-chain.',
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
  var gb = $('govRefresh'); gb.addEventListener('click', function () { flash(gb); refreshGov(); });

  // ---------- the "weight voted" tile (ballot #1's tallies, summed) ----------
  function refreshVotedTile() {
    if (!FtbRpc.current()) return;
    if (!ballotConfigured || !(BL[0].options && BL[0].options.length)) return;
    Promise.all(BL[0].options.map(function (_, i) {
      return FtbRpc.readInt(BL[0].hash, 'tally', [{ type: 'Integer', value: String(i + 1) }]);
    })).then(function (vals) {
      var t = vals.reduce(function (s, v) { return s + v; }, 0n);
      FtbUtil.tileNum($('tVoted'), t, ' FLM');
    }).catch(function () {});
  }

  FtbState.on('rpc', function () { refreshGov(); refreshVotedTile(); });
  FtbState.on('account', refreshGov);
  FtbState.on('refresh', function () {
    refreshGov(); refreshVotedTile();
    // a confirmed write clears the proposal form — fresh state, nothing lingering
    var pm = $('pMeta'), pr = $('pRef');
    if (pm) pm.value = ''; if (pr) pr.value = '';
  });
})();
