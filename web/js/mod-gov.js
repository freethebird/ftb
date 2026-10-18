// mod-gov.js — module 5: governance (MODULES.md).
// The live ballot: snapshot-weighted, counted on-chain. Weight is your snapshot
// balance in FLM — 1 FLM = 1 vote — proven with the same merkle proof as the
// claim. Also owns the "weight voted" stat tile.
//
// Owns (writes only these): #vote card (#govQ, #govHint, #govBody,
// #govRefresh) and the tile #tVoted.
(function () {
  'use strict';
  if (!FtbUtil.$('vote')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, el = FtbUtil.el, fmt = FtbUtil.fmt, errText = FtbUtil.errText;
  var CFG = FtbState.CFG, C = FtbState.C;
  var ballotConfigured = !!(C.ballot && C.ballot.indexOf('0x0000') !== 0 && CFG.ballot && CFG.ballot.question);

  function decodeVoted(st) {
    var it = st && st[0];
    return it && (it.type === 'Boolean' ? it.value : (it.type === 'Integer' ? parseInt(it.value || '0', 10) > 0 : (it.value && it.value !== '')));
  }

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
      body.innerHTML = '';
      vals.forEach(function (v, i) {
        var pct = total > 0n ? (Number(v * 10000n / total) / 100) : 0;
        var opt = el('div', 'gopt');
        var row = el('div', 'grow');
        row.appendChild(el('span', 'gname', '#' + (i + 1) + ' ' + opts[i]));
        row.appendChild(el('span', 'gnum', fmt(v) + ' FLM'));
        var btn = el('button', null, 'vote ->');
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
  $('govRefresh').addEventListener('click', refreshGov);

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
