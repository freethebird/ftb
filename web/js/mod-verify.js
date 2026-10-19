// mod-verify.js — module 8: verify (MODULES.md).
// The verify grid (every hash from config, in the open), the cap/sealed chain
// reads, and the IPFS mirror counter. Verify everything — trust nothing.
//
// Owns (writes only these): #verify grid — #vToken, #vMigrator, #vRegistry,
// #vBallot, #vRoot, #vCap, #vSealed — and the tile #tMirrors.
(function () {
  'use strict';
  if (!FtbUtil.$('verify')) return; // R2: inert where the markup is absent

  var $ = FtbUtil.$, fmt = FtbUtil.fmt;
  var CFG = FtbState.CFG, C = FtbState.C;

  // ---------- the grid: config, verbatim ----------
  $('vToken').textContent = C.ftb || '—';
  $('vMigrator').textContent = C.migrator || '—';
  $('vRegistry').textContent = C.registry || '—';
  // one ballot contract per proposal — list them all, newest config shape first
  var BL = (CFG.ballots && CFG.ballots.length)
    ? CFG.ballots.map(function (b) { return b.hash; })
    : (C.ballot ? [C.ballot] : []);
  $('vBallot').textContent = BL.length ? BL.join('  ·  ') : '—';
  $('vRoot').textContent = CFG.merkleRoot || '—';

  // ---------- chain reads: hard cap and the seal ----------
  function refresh() {
    if (!FtbRpc.current() || !FtbState.configured) return;
    FtbRpc.readBool(C.ftb, 'migrationOpen').then(function (open) {
      $('vSealed').textContent = open ? 'not yet — open for claims' : 'yes — no admin, no mint, forever';
    }).catch(function () {});
    FtbRpc.readInt(C.ftb, 'maxSupply').then(function (v) {
      $('vCap').textContent = fmt(v) + ' FTB (hard cap, compiled in)';
    }).catch(function () {});
  }

  // ---------- ipfs mirror counter ----------
  // every mirror is a base URL that should serve THIS exact build. the deploy script
  // stamps each build with beat.json {project, commit, published}; we compare our own
  // beat.json against every mirror's: reachable AND same commit = live, reachable but
  // older = stale, no answer = down. hover the tile for the per-mirror breakdown.
  (function () {
    var tile = $('tMirrors'); if (!tile) return;
    var M = (CFG.mirrors && CFG.mirrors.length ? CFG.mirrors.slice() : []);
    if (!M.length) { tile.textContent = '—'; return; }
    function get(u, ms) {
      return Promise.race([
        fetch(u + (u.indexOf('?') >= 0 ? '&' : '?') + 'v=' + Date.now(), { cache: 'no-store' })
          .then(function (r) { if (!r.ok) throw new Error('http ' + r.status); return r.json(); }),
        new Promise(function (_, rej) { setTimeout(function () { rej(new Error('timeout')); }, ms); })]);
    }
    function probe() {
      get('beat.json', 4000).then(function (mine) {
        var myCommit = mine && mine.commit;
        Promise.all(M.map(function (base) {
          return get(base.replace(/[/]+$/, '') + '/beat.json', 8000).then(function (b) {
            return { base: base, ok: true, same: !!(b && b.project === 'ftb' && myCommit && b.commit === myCommit) };
          }).catch(function () { return { base: base, ok: false, same: false }; });
        })).then(function (rs) {
          var live = rs.filter(function (r) { return r.same; }).length;
          tile.textContent = live + '/' + M.length;
          tile.title = rs.map(function (r) {
            return (r.same ? '● live' : (r.ok ? '◐ stale' : '○ down')) + '  ' + r.base;
          }).join('\n');
        });
      }).catch(function () {
        tile.textContent = '?/' + M.length;
        tile.title = 'no beat.json on this origin — deploys are version-stamped by ftb-push-ipfs.sh';
      });
    }
    probe(); setInterval(probe, 300000);
  })();

  FtbState.on('rpc', refresh);
  FtbState.on('refresh', refresh);
})();
