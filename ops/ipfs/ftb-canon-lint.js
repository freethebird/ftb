#!/usr/bin/env node
'use strict';
// ftb-canon-lint.js — enforces the Canonical Bible (HTML + JS editions) on a web tree.
// Exit 0 = compliant; 1 = a named violation. Called by the pre-commit hook and the deploy
// gate, so non-compliant client code fails to commit AND fails to publish. No deps (Article 2).
//
//   usage: ftb-canon-lint.js <web-dir> [maxKB]
//
// Checks (each a greppable, enumerated rule of the canon):
//   HTML  CSP self-only (no blanket 'unsafe-inline', no third-party script/style origin),
//         no inline <script>/<style>/style=, no on* handler, no javascript:/document.write,
//         no iframe/embed, every target=_blank has rel="noopener noreferrer", referrer meta,
//         no render-blocking <script src> in <head> (defer/async or end-of-body), weight budget.
//   JS    no innerHTML/outerHTML/insertAdjacentHTML/setHTML/document.write (DOM-sink inventory
//         empty), no eval/new Function, no require/bare-import (npm surface), no user-data egress
//         via location.search/hash or history.*State.
const fs = require('fs'), path = require('path');
const ROOT = process.argv[2];
const MAX = (parseInt(process.argv[3], 10) || 300) * 1024;
if (!ROOT) { console.error('usage: ftb-canon-lint.js <web-dir> [maxKB]'); process.exit(2); }

// dev-only / non-client files: not shipped, not client code — skipped so they never false-fail.
const SKIP_DIR = /(^|\/)(archive|backup)(\/|$)/;
const SKIP_FILE = new Set(['serve.mjs', 'make_export.py', 'config.example.js', 'demo.html']);
const fails = [];
const bad = (f, m) => fails.push(f + ': ' + m);

const files = [];
(function walk(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    const p = path.join(d, e.name), rel = path.relative(ROOT, p);
    if (e.isDirectory()) { if (!SKIP_DIR.test('/' + rel)) walk(p); }
    else if (!SKIP_FILE.has(e.name)) files.push(p);
  }
})(ROOT);
const rel = f => path.relative(ROOT, f);
const strip = s => s.replace(/<!--[\s\S]*?-->/g, ''); // drop HTML comments (browser never parses them)

for (const f of files.filter(f => f.endsWith('.html'))) {
  const raw = fs.readFileSync(f, 'utf8'), s = strip(raw), r = rel(f); let m;
  const csp = (raw.match(/http-equiv="Content-Security-Policy"\s+content="([^"]*)"/) || [])[1];
  if (!csp) bad(r, 'no Content-Security-Policy meta');
  else {
    if (!/script-src[^;]*'self'/.test(csp)) bad(r, "CSP script-src lacks 'self'");
    if (/script-src[^;]*'unsafe-inline'/.test(csp)) bad(r, "CSP script-src has blanket 'unsafe-inline'");
    if (/style-src[^;]*'unsafe-inline'/.test(csp)) bad(r, "CSP style-src has blanket 'unsafe-inline'");
    for (const d of (csp.match(/(?:script-src|style-src)[^;]*/g) || []))
      if (/https?:\/\//.test(d)) bad(r, 'CSP ' + d.split(' ')[0] + ' names a third-party origin');
  }
  const sre = /<script\b([^>]*)>([\s\S]*?)<\/script>/gi;
  while ((m = sre.exec(s))) if (!/\bsrc=/.test(m[1]) && m[2].trim()) bad(r, 'inline <script> with content');
  if (/<style\b[^>]*>[\s\S]*?<\/style>/i.test(s)) bad(r, 'inline <style> block');
  if (/\sstyle="/.test(s)) bad(r, 'inline style= attribute');
  if (/\son[a-z]+\s*=\s*["']/i.test(s)) bad(r, 'inline on* event handler');
  if (/(?:href|src|action)\s*=\s*["']\s*javascript:/i.test(s)) bad(r, 'javascript: URL');
  if (/<(?:iframe|embed)\b/i.test(s)) bad(r, 'iframe/embed');
  if (/document\.write/.test(s)) bad(r, 'document.write in markup');
  const are = /<a\b([^>]*)>/gi;
  while ((m = are.exec(s))) {
    const at = m[1];
    if (/target\s*=\s*["']_blank["']/.test(at)) {
      const rl = (at.match(/\brel\s*=\s*["']([^"']*)["']/) || [])[1] || '';
      if (!/noopener/.test(rl) || !/noreferrer/.test(rl)) bad(r, 'target=_blank link missing rel="noopener noreferrer"');
    }
  }
  if (!/name="referrer"/.test(raw)) bad(r, 'no referrer-policy meta');
  const head = (s.match(/<head\b[^>]*>([\s\S]*?)<\/head>/i) || [])[1] || '';
  const hre = /<script\b([^>]*\bsrc=[^>]*)>/gi;
  while ((m = hre.exec(head))) if (!/\bdefer\b/.test(m[1]) && !/\basync\b/.test(m[1])) bad(r, 'render-blocking <script src> in <head> (needs defer/async)');
  // weight budget: page + its local js/css refs
  let total = Buffer.byteLength(raw);
  const seen = new Set();
  for (const ref of (raw.match(/(?:src|href)="([^"]+\.(?:js|css))"/g) || [])) {
    const u = ref.replace(/.*="([^"]+)".*/, '$1');
    if (/^(https?:|\/\/|data:)/.test(u) || seen.has(u)) continue; seen.add(u);
    const p = path.join(path.dirname(f), u.replace(/^\.?\//, ''));
    try { total += fs.statSync(p).size; } catch (e) {}
  }
  if (total > MAX) bad(r, 'over weight budget: ' + (total / 1024).toFixed(0) + ' KB > ' + (MAX / 1024) + ' KB critical-path');
}

for (const f of files.filter(f => f.endsWith('.js') || f.endsWith('.mjs'))) {
  const s = fs.readFileSync(f, 'utf8'), r = rel(f);
  if (/\.(?:innerHTML|outerHTML)\s*=(?!=)/.test(s)) bad(r, 'DOM sink: innerHTML/outerHTML assignment');
  if (/\.insertAdjacentHTML\s*\(/.test(s)) bad(r, 'DOM sink: insertAdjacentHTML');
  if (/\.setHTML\s*\(/.test(s)) bad(r, 'DOM sink: setHTML');
  if (/document\.write\s*\(/.test(s)) bad(r, 'document.write');
  if (/\beval\s*\(/.test(s)) bad(r, 'eval()');
  if (/\bnew\s+Function\s*\(/.test(s)) bad(r, 'new Function()');
  if (/\brequire\s*\(/.test(s)) bad(r, 'require() (npm surface)');
  if (/^\s*import\s+[^'";]*['"][^.\/]/m.test(s)) bad(r, 'bare import (npm surface)');
  if (/\blocation\.(?:search|hash)\s*=(?!=)/.test(s)) bad(r, 'possible egress: writes user state to location.search/hash');
  if (/\bhistory\.(?:pushState|replaceState)\s*\(/.test(s)) bad(r, 'possible egress: history.pushState/replaceState');
  if (/\.Global\b/.test(s) || /\bscopes\s*:\s*128\b/.test(s)) bad(r, 'witness scope Global (128) — use CalledByEntry/CustomContracts, never Global');
}

if (fails.length) { for (const x of fails) console.error('CANON: ' + x); console.error('CANON: FAIL — ' + fails.length + ' violation(s).'); process.exit(1); }
console.log('CANON: PASS (' + files.length + ' files)'); process.exit(0);
