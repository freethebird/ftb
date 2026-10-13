# FTB — Audit against Modus Operandi V8 and the QRN Network Threat Model

**Subject:** the `ftb/` repo — six contracts (`ftb_token`, `ftb_migrator`, `registry`,
`oracle`, `lend`, `ballot`), `pkg/merkle`, the `cmd/` tools, `web/ftb-dapi.js`, manifests,
Makefile, and CI — at 13 October 2026.
**Against:** Modus Operandi V8 (11 October 2026, eleven articles + Annexes B and C) and the
QRN Network Threat Model V1.1, applied through the **Canonical Bible, Hardening Go Edition
V1** (this audit's companion document — the C canon re-expressed for Go, since FTB is not
built in C).
**Audited:** 13 October 2026 — **Verdict: PASS, with one recorded doctrine deviation
(Article 2) and one explicit platform override (Article 11).** All other articles satisfied.
**Document ref:** FTB_AUDIT_MOV8_QRN_13102026

This is the standing-audit pass (Article 7) of the FTB subproject against the current
doctrine. Earlier passes (AUDIT_REPORT, AUDIT_REPORT_PASS2) audited the code against the
project's internal brief; this pass audits the whole project — code, tooling, governance,
and ops — against the full MO V8 article set, the Annex B threat catalogue, and the QRN
network threat classes, under the Go canon.

## Method

Each article was read against the built tree and its gates, not against the documentation
alone — and unlike the earlier passes, **the gates were actually run on this machine** (a
Go 1.27.2 toolchain and the pinned neo-go v0.123.0 compiler are installed here):

| Gate (Go canon matrix) | Result, this pass |
| --- | --- |
| `gofmt -l .` | clean (no output) |
| `go vet ./...` | clean |
| `go build ./...` (darwin/arm64) | pass |
| `go build ./...` cross-compile (linux/amd64, windows/amd64 host pkgs) | pass |
| `go test ./...` — 31 tests across 7 packages | pass |
| `go test -race ./...` | pass |
| `neo-go contract compile` × 6 → NEF + manifest | pass (NEF 1.0–3.3 KB each) |
| `cmd/merkleroot` re-run against the real `snapshot.csv` (r1=1, r2=4) | root `0x639aaa78…a8bba` and total `78326783050305942` — **byte-identical to the compiled `maxSupply` constant and DECISIONS.md** |
| Manifest minimality inspection | confirmed (three contracts with zero permissions; token wildcard scoped to `onNEP17Payment` + `update`; migrator to `mintForMigration`/`closeMigration`; lend to its five calls) |
| `make conformance` (fmt, vet, compile, 37 tests + 3 fuzz targets in 8 packages, `-race`, govulncheck, NEF-hash record) | **pass — added this pass (FTB-23); see the addendum** |
| govulncheck against the pinned module set | clean after the FTB-28 bump (0 reachable vulnerabilities) |
| Fuzz smoke: `FuzzBuildVerify`, `FuzzAggregateRow`, `FuzzHashPairCommutative` | ~4.9M / ~4.9M / ~1.8M executions, no finding (FTB-24) |
| NEF-hash gate removed-guard control | a forged `NEF_RELEASE.sha256` makes `scripts/nef_hash.sh` exit 1 — the check is proven load-bearing, then removed |

Findings are logged below and remediated in place where the fix is documentation or gates;
code findings that need an owner decision are named, not silently deferred.

## Article-by-article verdict (MO V8, under the Go canon)

| # | Article | Verdict | Evidence |
| --- | --- | --- | --- |
| 1 | The Go portability and memory-safety guarantee (Canon Book II) | **Confirmed** | Every gate in the matrix above passes today on this machine, plus the CI axis on linux/amd64. No `unsafe`, no cgo, no assembly anywhere in the tree (inspection gate: grep-clean). Contracts compile under pinned neo-go v0.123.0. The full matrix is now folded into one named guarantee, `make conformance` (FTB-23, fixed this pass). |
| 2 | No dependencies | **Deviation, recorded (FTB-21)** | The graph is not empty and cannot be: contract compilation requires neo-go. The Go canon re-expression is largely met — one named, pinned platform dependency, `go.sum` committed, CI resolves pinned sums, and `govulncheck` is now on the gate path (FTB-23) and green after the FTB-28 bump — but the tree is not yet vendored (`go mod vendor` at release remains the standing recommendation). Everything else is standard library; the web client is dependency-free by design. |
| 3 | No vendors | **Confirmed** | Nothing load-bearing depends on a hosted service, SDK key, or account. RPC access is substitutable (three community nodes + user-pastable endpoints, `ops/DECENTRALIZED_OPS.md`); the front-end is static files pinned on IPFS by five operators. The reused Flamingo swap is an on-chain, audited, immutable NEF called by hash — a protocol peer, not a vendor; no one can revoke it. GitHub CI is a convenience, not a build requirement (`make` runs the same gates locally). |
| 4 | Speed, weight, security, performance | **Confirmed (one open item)** | Weight: six contracts at 1.0–3.3 KB NEF, single-byte storage prefixes, balance keys deleted on zero. Speed: the lend hot path was optimized in pass 2 (oracle resolved once per transaction, storage reads into locals) with the suite pinning behavior. Security: the gates above. Open, in the open: no recorded four-number harness (latency / footprint / audit posture / throughput per release) — the same open item the C reference stack carries; the conformance gate now exists, so the numbers have somewhere to attach. |
| 5 | Leave no stone unturned | **Confirmed** | 37 tests + 3 fuzz targets across 8 packages, including named negative tests for every guarded revert path (the guard tests from passes 1 and 2 are all present and green). Every error return checked; every refusal panics with a reason; missing seam dependencies fail closed (`oracleHash` panics if the registry has no oracle). The fuzz gap named here earlier this pass is closed (FTB-24). Every Annex B class answered below. |
| 6 | Never trade visible for invisible | **Confirmed, exemplary** | The project's strongest article. Placeholders ship with `>>>` markers and a named owner decision (`DECISIONS.md`); the accrual-direction error in AUDIT.md was corrected in the open (FTB-13); "pending audit" banners on every contract; the deploy tool prints the exact multisig wiring instead of hiding it; DECISIONS.md Decision 3 records the TestNet topology tradeoffs explicitly. The Article 2 deviation and the Article 11 override are disclosed in this report rather than smoothed over. |
| 7 | Audit and remediate everything | **Confirmed** | The audit loop is real and closed: pass 1 (14 findings), pass 2 (7 findings + re-verification), each fixed in code with a named test, then this pass. Verified closed today by re-running the artifact: **FTB-01** — the compiled `maxSupply` equals the exact merkleroot total from the real snapshot (78,326,783.05030542 FTB), root matching the published value. Carried open, in the open: FTB-17 (the superseded C# tree still sits in the workspace root — banners added, deletion still recommended before publishing), FTB-18 (client base58 checksum), FTB-20 (liquidator sizing note). No known defect ships unrecorded. |
| 8 | Modularize; add seams (closed-end) | **Confirmed** | The seam rule is realized: each concern is its own contract; modules resolve each other through the registry (timelocked, monotonic-delay, `Apply` callable by anyone — the delay is the protection, not the caller); the token is deliberately referenceless and seals itself. Closed-end: there is **no runtime negotiation anywhere in the stack** — a module hash is bound by governance behind the timelock or not at all; nothing falls back to a weaker variant; a missing binding panics. The token's pre-launch `Update` path is the one open seam, and it is killed by `RenounceOwnership` before value flows — verified unreachable after renounce by the interlock tests. |
| 9 | Recursion where it adds value | **Confirmed** | None, by restraint — the Merkle builder is iterative with explicit layering; no unbounded or accidental recursion exists. Matches the C stack's own verdict on the same article. |
| 10 | Shared toolkit, one canonical implementation | **Confirmed** | `pkg/merkle` is the single source of truth for the leaf format, imported by the CLI and the cross-check tests — consumed, never forked. The on-chain duplication (the claim/vote verification loop appearing in both `ftb_migrator` and `ballot`) is platform-forced: NeoVM contracts cannot share code; the round-trip KAT pins both to the same bytes, so the duplication cannot drift silently. Host-side, one implementation per primitive holds. |
| 11 | The quantum razor | **OVERRIDDEN — platform-bound, recorded in the open (FTB-22)** | Neo N3's witness model is secp256r1 ECDSA only; there is no post-quantum signature path on the platform, so `CheckWitness` — the root of all authority in every FTB contract — rests on a classical assumption alone, and nothing FTB ships on-chain can change that floor. Per the MO scope clause the subproject names the override explicitly: **FTB overrides Article 11 for the platform signature layer.** What FTB owns itself is already on the right side of the razor: the entitlement proofs are sha256 Merkle structures — hash-based, degrading gracefully (Grover-halved, not Shor-broken) — and no FTB-owned path negotiates anything down to anything. The residual exposure is exactly the platform's: witness forgery by a future quantum adversary, and the trusted window's multisig. Mitigations that are real rather than cosmetic: keep the trusted window short (close-then-renounce promptly), keep the multisig keys fresh and offline, and track Neo's PQ roadmap; the migration Merkle anchor itself needs no change. |

## Annex B — threat catalogue, class by class

1. **Forgery via a false witness.** Answered. The mint is forged only through the migrator,
   and the migrator only through a valid proof plus `CheckWitness(account)` — double-claim,
   claim-for-another, and inflated-amount are all test-pinned reverts. The verifier (the
   claim path) is fixed at build time and immutable after renounce; it is never governance-
   mutable.
2. **Challenge / transcript manipulation.** Answered. The migrator root is write-once
   (FTB-02); the ballot's root and option count are write-once; nothing is drawn before it
   binds.
3. **Statement / scheme / version replay.** Answered. The snapshot root binds electorate and
   entitlement; ballot and migrator share one root by construction, so a proof replays only
   into the context it was published for. The stale C# leaf spec that could have caused a
   cross-scheme mismatch is banner-marked (FTB-17, carried).
4. **Downgrade / negotiation.** Answered. No live negotiation exists anywhere; timelock
   cannot shorten (`IncreaseDelay` is monotonic); a zero-delay lock-in is impossible
   (`FinishBootstrap` requires non-zero); module swaps are propose-then-apply in the open.
   The only strength downgrade is the pre-renounce `Update` path, which is renounced before
   value — a one-way door, test-pinned.
5. **Parser / malformed input (LangSec).** Answered on-chain (length checks on every hash
   and proof element; fail-closed panics) and hardened off-chain (CSV aggregation, negative
   rejection, hard-fail on skipped rows, FTB-05/FTB-16). The parser and the Merkle builder
   are now fuzz-gated (FTB-24, fixed this pass: ~10M combined fuzz executions, no finding).
6. **Resource exhaustion / DoS.** Answered. On-chain work is caller-paid gas and bounded by
   small committees; `oracle.Price` iterates the reporter set on every read — bounded by
   policy (small committee), named here so it stays small. Odd-length trees carry up without
   duplication; `merkle.Build` is linear per layer.
7. **Cross-implementation divergence.** Answered, and this is the project's strongest gate:
   the off-chain builder and the on-chain claim are pinned byte-for-byte by a real-VM
   round-trip test, and an independent Python re-implementation re-verified all 4,847
   published proofs against the root. Re-verified again today: the same snapshot reproduces
   the identical root.
8. **Fault / physical.** Out of scope for a chain-hosted artifact (the VM is the fault
   domain); operator-side, the deploy tool hard-fails on any error and validates the root
   length before wiring.
9. **Supply-chain / dependency.** Partially answered — the Article 2 deviation lives here.
   One pinned platform dependency with committed sums, `govulncheck` now on the gate path
   and green (FTB-28, fixed this pass); vendoring at release remains open (FTB-21). The
   reused Flamingo swap is an audited, immutable on-chain artifact, not a live supply line.
10. **Side-channel / scope.** Answered by scope: on-chain state and execution are public,
    constant-time is not load-bearing (recorded mode, Go canon Book II rule 8). Host-side
    secrets live only in the scrypt wallet layer.
11. **Honesty / over-accounting.** Answered. DECISIONS.md is the model: placeholders marked
    `>>>`, unimplemented modes labeled, errors corrected in the open, and this audit's
    deviation and override disclosed rather than smoothed.

## QRN Network Threat Model — class by class

**A. Governance and upgrade authority (the honeypot).** Answered, with the residuals in the
open. Contracts are born with the multisig as owner (FTB-14 fix: deploy decouples owner
from signer; single-key fallback prints a loud warning). Peripheral upgrades sit behind the
registry timelock with monotonic delay and permissionless apply; the token — the value
invariant — seals and renounces, ending in immutability, which is exactly the razor's end
state for this class. Open in the open: the trusted window (FTB-02 residual: `SetMigrator`
re-pointable until renounce, bounded by the cap and by prompt close-then-renounce) and the
oracle's immediate admin (FTB-06: deployment decision — multisig owner, quorum ≥ 3, now in
the pre-launch checklist). The QRN's "hardest unanswered question" is answered for FTB:
retained authority shrinks to zero on the token and to timelocked multisig on peripherals.
**Re-run verification:** deploy config → printed wiring calls; registry one-way-door tests.

**B. Liveness, censorship, escape hatch.** Answered at FTB's layer. Claims are
permissionless (any holder with a proof, no operator in the path); voting is permissionless;
`Apply` is callable by anyone; the oracle's failure mode is a pause that cannot trap
debt-free collateral (FTB-10). Named as out-of-layer: Neo N3 liveness itself (7-committee
dBFT) is the platform's property, not FTB's — recorded, per the QRN's own scope line.
**Re-run verification:** migrator and ballot neotest suites; FTB-10 guard test.

**C. Consensus safety and decentralization economics.** Out of scope for the app layer, and
named as such; the ops plan's community-RPC and validator-vote lines track the layer where
this class lives. What FTB adds on top introduces no new consensus assumption: ballot
tallies are replicated chain state — every full node counts the identical result, so
"counting the vote" is not a service anyone runs.

**D. Proof-system and implementation integrity.** Confirmed — the differential-KAT half and
the reproducibility half both have teeth here. Off-chain builder vs on-chain claim: pinned
by the real-VM round-trip test plus the independent Python re-verification (4,847/4,847),
and now fuzz-gated on both sides of the seam (FTB-24). Reproducibility: pinned toolchain,
pinned neo-go, committed `go.sum`, and the NEF-hash record/verify gate (FTB-23) — the
release step is `cp build/NEF_HASHES.sha256 NEF_RELEASE.sha256`, after which any rebuild
that does not reproduce the recorded hashes fails the gate. **Re-run verification:**
`go test ./contracts/ftb_migrator`; re-run merkleroot (done today — identical root);
`sh scripts/nef_hash.sh`.

**E. Data availability and trustless exit.** Confirmed. The snapshot, the ratios, and
`proofs.json` are published artifacts; the root is reproducible by anyone from them (proven
today); claiming needs no operator. An escape hatch with published, reproducible data is a
real escape hatch.

**F. App-layer and account abstraction.** This is FTB's own layer, and it inherits the MO
plus the pass-1/pass-2 attack-vector catalogs (23 vectors tracked, each blocked or carried
open with a name). The highest-risk member of this class — lending — is gated to Launch 2
behind its own dedicated audit, precisely because this class is where real losses occur.

## Findings this pass

| ID | Sev | Area | Title | Status |
|----|-----|------|-------|--------|
| FTB-21 | Medium | doctrine / supply chain | Article 2 deviation: 46-module graph via neo-go; not vendored | **Open, recorded; narrowed this pass** — `govulncheck` is now gated and green (FTB-23/FTB-28). Remaining recommendation: `go mod vendor` at release (the Go canon's Article 2 realization). The deviation itself is deliberate (one language end to end), so it is paid in the open, not hidden. |
| FTB-22 | Medium | doctrine / platform | Article 11 must be explicitly overridden: Neo N3 witnesses are secp256r1-only | **Named in this report (the override above); recommend** recording the same three lines in `ARCHITECTURE.md` so the override travels with the code, and re-checking at each Neo platform upgrade. FTB-owned crypto (sha256 Merkle) is hash-based and needs no change. |
| FTB-23 | Low | gates | No single `make conformance` target folding the full Go matrix (race, vulncheck, NEF-hash record) into one named guarantee | **Fixed 13 Oct:** `make conformance` runs fmt, vet, contract compile, the full suite, `go test -race ./...`, `govulncheck ./...`, and `scripts/nef_hash.sh` (records `build/NEF_HASHES.sha256`, fails closed against a recorded `NEF_RELEASE.sha256` — the mismatch path was exercised and exits 1). CI runs the same steps. See the addendum. |
| FTB-24 | Low | tests | No fuzz targets for `pkg/merkle` and the snapshot CSV parser | **Fixed 13 Oct:** `FuzzBuildVerify` + `FuzzHashPairCommutative` (pkg/merkle) and `FuzzAggregateRow` (cmd/merkleroot, on the extracted — now testable — `aggregate` function). Smoke runs: ~4.9M, ~1.8M, and ~4.9M executions respectively, no finding. The refactor also gives merkleroot six unit tests (FTB-27, partially addressed). |
| FTB-25 | Info | web | base58 decode skips checksum/version validation (carried: FTB-18) | Open, client-side only, no custody risk. |
| FTB-26 | Info | workspace | Superseded C# tree still present at the workspace root (carried: FTB-17) | Banners in place; deletion still recommended before publishing the repo. |
| FTB-27 | Info | tooling | `cmd/deploy`, `cmd/seed`, `cmd/merkleroot` logic lives in `main()` with no unit tests (carried from pass 1) | **Partially addressed 13 Oct:** `cmd/merkleroot`'s aggregation logic is extracted into a testable `aggregate` function with six unit tests and a fuzz gate. `cmd/deploy` and `cmd/seed` remain `main()`-bound (network-side drivers; the refactor is still worth doing). |
| FTB-28 | Medium | supply chain | govulncheck: six reachable vulnerabilities in `x/net` v0.58.0 and `grpc` v1.82.1 (HTTP/2 paths in the RPC client of the operator tooling) | **Fixed 13 Oct:** found by the new FTB-23 gate on its first run — the gate working as designed. Bumped to `x/net` v0.60.0 / `grpc` v1.83.1 (minimum-fixed); neo-go v0.123.0 and the `go 1.26.0` directive unchanged. All six contract NEFs recompile **byte-identical** across the bump (hashes compared); suite + race green. Recorded as DECISIONS.md Decision 5. Exposure was operator tooling only — contracts never touch these paths. |

## Verified closed this pass

- **FTB-01 (was High): the cap is bound to the snapshot.** Re-running `cmd/merkleroot`
  against the real `snapshot.csv` with r1=1, r2=4 prints total `78326783050305942` base
  units and root `0x639aaa7801f41c75d15c75aaad31aa6d6ede2b8bb81bd30f7ebf9560d50a8bba` —
  exactly the compiled `maxSupply` constant in `ftb_token.go` and the published root in
  DECISIONS.md. What remains is conditional, not open: **if the ratio vote changes r1/r2,
  the constant must be reset to the new printed total before compiling for launch** — the
  merkleroot output prints this instruction itself.
- **Gate reality.** Earlier passes carried the caveat "not compiled in this environment."
  That caveat is retired: every gate in the Go canon matrix ran green on this machine today.

## Open items (recorded, not defects)

- The four-number release harness (Article 4) — same standing open item as the C reference
  stack; the conformance gate (FTB-23) is now the place the numbers attach.
- `minReports` ships at 1 by deliberate design (test harness and TestNet run one reporter);
  the quorum floor belongs to the deployment checklist, where it sits (FTB-06, DECISIONS.md
  Decision 2). Recorded so the default is never mistaken for the launch configuration.
- The trusted window (FTB-02 residual) is inherent to a one-shot migration: the cap bounds
  it, the interlocks order it, and the ops plan minimizes it. It is the documented trust
  period, not a hidden one.

## Conclusion

Under the Canonical Bible, Hardening Go Edition V1, the FTB subproject **satisfies Articles
1 and 3 through 10 of MO V8, answers every Annex B class and every QRN network class at its
layer, records its one dependency deviation (Article 2) and its one platform override
(Article 11) in the open**, and closes its last High finding (FTB-01) by re-computation on
this machine today. The Definition of Done is not yet met — not because of a defect, but
because the doctrine's own external gates still stand: two independent external audits of
the Launch 1 core, a separate audit of oracle plus lend, and a public TestNet with a bug
bounty. Those are the launch gate, named in every document in this lineage, and this audit
does not move them.

## Addendum: gate-hardening pass (13 October 2026)

The two open gate findings from this audit were remediated in place the same day, per
Article 7.

**FTB-23 — the conformance gate now exists.** `make conformance` folds the Go canon's full
verification matrix into one named guarantee: `gofmt`, `go vet`, contract compile under the
pinned neo-go, the full test suite, `go test -race ./...`, `govulncheck ./...`, and
`scripts/nef_hash.sh` (NEF-hash record/verify). CI runs the same steps. The NEF-hash gate
ships with its removed-guard control exercised: a forged `NEF_RELEASE.sha256` makes it exit
1 — proven load-bearing — and the control file was then removed. Release procedure: after a
release build, `cp build/NEF_HASHES.sha256 NEF_RELEASE.sha256` and commit it; from then on
any rebuild that does not reproduce the recorded contract bytes fails the gate.

**FTB-24 — fuzz gates on the LangSec boundary.** Three fuzz targets now run as part of the
suite: `FuzzBuildVerify` and `FuzzHashPairCommutative` in `pkg/merkle` (every emitted proof
must verify; a foreign leaf must never verify; node hashing must be commutative) and
`FuzzAggregateRow` in `cmd/merkleroot` (a fuzzed snapshot row never panics the builder,
never yields a negative total, never both skips and emits). Smoke runs: ~4.9M, ~1.8M, and
~4.9M executions, no finding. To reach the parser, `cmd/merkleroot`'s aggregation loop was
extracted into a testable `aggregate` function — behavior-preserving by construction and
proven so: the refactored CLI reproduces the identical root and total from the real
snapshot — gaining six unit tests in the bargain (FTB-27 partially addressed).

**FTB-28 — the gate caught real vulnerabilities on its first run.** govulncheck reported
six reachable vulnerabilities in two indirect modules of the operator tooling (`x/net`
v0.58.0, `grpc` v1.82.1 — HTTP/2 paths under the RPC client). Fixed by bumping to the
minimum-fixed versions (`x/net` v0.60.0, `grpc` v1.83.1); the pinned neo-go v0.123.0 and
the `go 1.26.0` directive are unchanged. The on-chain artifact is proven untouched: all six
contract NEFs recompile byte-identical across the bump (hashes compared before and after),
and the full suite plus `-race` passes. Recorded as DECISIONS.md Decision 5.

Full matrix after the pass: `make conformance` green end to end — 37 tests + 3 fuzz targets
across 8 packages, race clean, govulncheck clean, NEF hashes recorded.

(c) 2026 — internal audit lineage; reference implementation pending external audit.
