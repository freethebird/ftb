# FTB documentation map

Everything in the repo that is prose rather than code lives here under `docs/`, grouped by
purpose, so any document has an obvious home and the root stays clean. The canonical
decision log stays at the repo root as `DECISIONS.md` (it is referenced by name from dozens
of code comments).

| folder | what goes here |
|--------|----------------|
| `overview/` | the one-page "what is this", the architecture and seams, use-cases, and the stack map |
| `governance/` | the fee / economics constitution, the proposal -> vote -> code process, and records of specific proposals and ballots |
| `audits/` | internal audit lineage and any external audit reports; IAR (independent audit review) briefs |
| `security/` | threat model, attack-vector register, key-custody and multisig procedure, bug-bounty scope, vulnerability disclosures |
| `migration/` | the 2026 Flamingo -> FTB migration: snapshot findings, wallet forensics, reconciliation, unflag process, burn / salvage mechanics |
| `community/` | community manual, FAQ, comms, and community-governance features (e.g. the Discord bond) |
| `releases/` | per-version release notes and changelog history (compiled NEF hashes live in `/NEF_RELEASE.sha256`) |

Published datasets the docs cite (CSV / JSON) live under `/data/` - see `../data/README.md`.
`STACK_MAP.md` / `.png` sit at the `docs/` root because several files link to `docs/STACK_MAP.md` directly.

## Conventions
- One topic per file; link between files with relative paths.
- A new venture or initiative files its docs under the folder that matches its purpose: a
  new governance proposal -> `governance/`, a new community feature -> `community/`, a
  security review -> `security/` or `audits/`. Add a new top-level folder here only when a
  document genuinely fits none of the above, and record it in the table above.
- Operational runbooks (how to run a node, deploy, sign) live in the repo's top-level
  `ops/` directory, not here. Front-end module docs live in `web/`.
