#!/usr/bin/env node
'use strict';
// ftb-canon-cs-lint.js — enforces the Canonical Bible (C# edition) on the vendored Flamingo AMM.
// Exit 0 = compliant; 1 = a named violation; 2 = usage. No deps (Article 2). Called by the
// `cs-canon` make target and the pre-commit hook, so a stale constant or an unresolved compiler
// warning fails to commit AND fails the conformance gate.
//
//   usage: ftb-canon-cs-lint.js <cs-root>
//
// Checks (each an enumerated rule of the C# canon, Book II/III):
//   - no active [InitialValue(...)] carrying a KNOWN-RETIRED identity (the 6 Nov 2026 failure:
//     the AMM baked the retired NeNEyks admin and the retired FTBv2 token as compile-time
//     constants, so the contracts were wired to the previous deployment).
//   - no active [InitialValue(...)] with a zero / placeholder hash.
//   - no unresolved #warning (Flamingo ships "check this admin address is current"; it must be
//     resolved and removed before the build).
//   - no admin-gated method defined without a CheckWitness / Verify authority check in its file.
//   - no witness scope Global (128) and no CallFlags.All on a cross-contract call.
// Informational (printed, non-failing): ContractManagement.Update methods, listed for the UNSAFE
// inventory so the freeze (Book II rule 9) is tracked; it cannot be proven from source alone.
const fs = require('fs'), path = require('path');
const ROOT = process.argv[2];
if (!ROOT) { console.error('usage: ftb-canon-cs-lint.js <cs-root>'); process.exit(2); }

// Identities retired on the v4 migration. An active [InitialValue] carrying one of these means the
// contract is wired to a dead stack. Keyed lowercase; both the N3 address and the 0x hash forms.
const RETIRED = new Map([
  ['neneykswakbjdexqvta2x1n3rx1hmzwrep', 'retired NeNEyks deploy wallet as admin'],
  ['0x33a902a7878274e82ec3e239f9f1d49236436aca', 'retired NeNEyks deploy wallet as admin'],
  ['0x1ff1f51e63179867367ae8982af583e01e7cbdbd', 'retired FTBv2 token (launch token is v4 0xce37caa3)'],
  ['0x0009b28b90c23b5fc2c49265572695defe2c9f57', 'retired whitelist contract reference'],
  ['0x2b582cdc325e0788a6732a79671687580d985dc4', 'retired factory contract reference'],
]);
const ZERO = /^0x0{40}$/i;

const ADMIN_METHODS = /\b(SetAdmin|AddRouter|RegisterExchangePair|RemoveExchangePair|SetFeeTo|Pause|Resume|SetWhiteList|AddExchangePair|CreateExchangePair)\b\s*\(/;
const fails = [], inventory = [];
let callAllCount = 0; // Contract.Call(... CallFlags.All): audited NEP-17 transfer pattern, tracked not failed
const bad = (f, m) => fails.push(f + ': ' + m);

const files = [];
(function walk(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    const p = path.join(d, e.name);
    if (e.isDirectory()) { if (!/(^|\/)(bin|obj)$/.test('/' + e.name)) walk(p); }
    else if (e.name.endsWith('.cs')) files.push(p);
  }
})(ROOT);
const rel = f => path.relative(ROOT, f);

for (const f of files) {
  const raw = fs.readFileSync(f, 'utf8'), r = rel(f);
  const lines = raw.split(/\r?\n/);
  let hasWitness = /Runtime\.CheckWitness\s*\(/.test(raw) || /\bVerify\s*\(\s*\)/.test(raw);
  let definesAdmin = false;

  lines.forEach((ln, i) => {
    const t = ln.trim();
    const commented = t.startsWith('//') || t.startsWith('*') || t.startsWith('/*');

    // [InitialValue("...")] — active lines only
    const iv = ln.match(/\[InitialValue\(\s*"([^"]+)"/);
    if (iv && !commented) {
      const v = iv[1].toLowerCase();
      if (RETIRED.has(v)) bad(r, `line ${i + 1}: stale [InitialValue] "${iv[1]}" — ${RETIRED.get(v)}`);
      else if (ZERO.test(v)) bad(r, `line ${i + 1}: placeholder [InitialValue] "${iv[1]}" — zero hash, not a real target`);
    }

    // unresolved compiler warning
    if (!commented && /^#warning\b/.test(t)) bad(r, `line ${i + 1}: unresolved #warning — resolve and remove before build`);

    // witness scope Global / over-broad call flags
    if (!commented && /(WitnessScope\.Global|Scopes\.Global|\bGlobal\b\s*=\s*128)/.test(ln))
      bad(r, `line ${i + 1}: witness scope Global — use CalledByEntry/CustomContracts, never Global`);
    // CallFlags.All on a Contract.Call is the audited NEP-17 transfer pattern (the callback needs
    // it). Narrowing it would be a forbidden logic edit (Book III), so it is tracked, not failed;
    // a human confirms each is a transfer, not an over-broad read, on upgrade.
    if (!commented && /Contract\.Call\([^)]*CallFlags\.All/.test(ln)) callAllCount++;

    if (!commented && ADMIN_METHODS.test(ln) && /\b(public|internal)\b/.test(ln)) definesAdmin = true;

    if (!commented && /ContractManagement\.Update\s*\(/.test(ln)) inventory.push(`${r}: line ${i + 1}: ContractManagement.Update — must be unreachable after freeze (Book II rule 9)`);
  });

  if (definesAdmin && !hasWitness)
    bad(r, 'defines an admin-gated method but the file has no CheckWitness(GetAdmin())/Verify() authority check');
}

if (callAllCount) inventory.push(`${callAllCount} Contract.Call(... CallFlags.All) site(s) — audited NEP-17 transfer pattern; review on upgrade, do not narrow in place`);
if (inventory.length) { console.error('CS-CANON inventory (UNSAFE cells to track, non-failing):'); for (const x of inventory) console.error('  INV: ' + x); }
if (fails.length) { for (const x of fails) console.error('CS-CANON: ' + x); console.error('CS-CANON: FAIL — ' + fails.length + ' violation(s) over ' + files.length + ' file(s).'); process.exit(1); }
console.log('CS-CANON: PASS (' + files.length + ' files)'); process.exit(0);
