# FTB decentralized operations plan

How five community members with five VMs run the whole FTB stack with no single point of
control. This is the operational companion to `docs/overview/ARCHITECTURE.md` (the seams) and
`../docs/STACK_MAP.md` (the signing boundary), and it is the script for the TestNet rehearsal
before MainNet.

## 0. The principle

The DEX mechanism - swap math, the sealed mint, Merkle claims, pool reserves - is contract
code and contract state. Once deployed, every full node on Neo N3 verifies and replicates it.
You do not run the mechanism; the chain does. Decentralization work is everything *around* it:

| Layer | Centralization risk | The plan |
|---|---|---|
| Admin keys | One hot key in the trusted window | 3-of-5 multisig as owner from the first block (FTB-14 fix) |
| RPC access | One endpoint can censor or die | 3 community RPC nodes + public endpoints + user-pastable URL |
| Reads / indexer | NeoFura is a third party | Community archive node; snapshot and tally reproducible by anyone |
| Front-end | One web host can be taken down | Static files on IPFS, pinned by all five; server is a mirror |
| Oracle (Launch 2) | Reporters are trusted | 3 independent reporters, quorum >= 3, safe pause on failure |
| Consensus (L1) | 7 validators, not yours | Community NEO voting drive; Neo X question goes to the vote |

## 1. Roles for the five members

Everyone holds a multisig key and pins the front-end on IPFS. The specialties:

| Member | Runs | Why |
|---|---|---|
| A | RPC node #1 + oracle reporter (Launch 2) | access layer |
| B | RPC node #2 + oracle reporter (Launch 2) | access layer |
| C | RPC node #3 + oracle reporter (Launch 2) | access layer |
| D | Archive full node (snapshot + vote tally reproduction) | verifiable history |
| E | Build box (compile + test + deploy tooling) + front-end mirror coordinator | release engineering |

Spread the VMs across providers and jurisdictions. Do not put two roles on the same physical
host, and do not let one member hold two multisig keys.

## 2. The VMs

Per VM: 2 vCPU, 4–8 GB RAM, 80 GB NVMe (member D's archive node: 8 GB RAM, 250 GB+, it keeps
historic state forever). Ubuntu 24.04 or Debian 12. Harden each box with the runbook in
`ops/1984_SETUP.md` (non-root user, key-only SSH, ufw, unattended upgrades) - that doc was
written for the build box but the hardening applies to all five.

## 3. The three RPC nodes (A, B, C)

Docker, the simplest path (details in the community guide, section 12.2–12.3):

```bash
docker run -d --name neofull \
  -p 10332:10332 -p 10333:10333 \
  -v $PWD/neo-data:/neo-cli/Data_LevelDB \
  ghcr.io/neo-project/neo-cli:latest
```

Required plugins: **RpcServer** (the endpoint), **TokensTracker** (`getnep17balances`, which the
vote tally uses). Bind RpcServer, then put Caddy or nginx in front for TLS so wallets get an
`https://` URL. Open only 10333 (p2p) and 443 in ufw; keep 10332 behind the proxy.

A neo-go node works too (the repo's toolchain already installs it) - one language end to end.
Either way, wait for full sync (`show state` until Height matches the network) before listing
the endpoint.

For the TestNet rehearsal, the same setup with TestNet ports (20332/20333) and magic
894710606.

## 4. The archive node (D)

Same node, plus **StateService** and no pruning. Its job is historic state: re-running
`snapshot_state.py` and `tally_votes.py` against a node the community controls, so the
snapshot and the vote never depend on trusting NeoFura or any single indexer.

The fairness rule: **two members independently reproduce the snapshot and compare roots.**
If D's archive node and E's run (against a public historic RPC) produce the same Merkle root,
the snapshot is verified by construction, not by trust.

## 5. The multisig (all five)

3-of-5, created before anything deploys. Flow:

1. Each member generates a keypair on their own machine (neo-cli `create address`, then
   `list key` to see the pubkey - verify exact console commands against your pinned neo-cli
   release) and publishes only the **compressed public key**. Private keys never leave each
   member's machine; for MainNet, use hardware-backed keys.
2. Everyone independently computes the multisig address from the five pubkeys
   (neo-cli `import multisigaddress 3 <pk1> <pk2> <pk3> <pk4> <pk5>`). All five must get the
   SAME address. This is the check that no one mistyped and no one is being fooled.
3. Publish the five pubkeys, the threshold, and the address in the repo.

You never need to paste a hash into the deploy config by hand: `cmd/deploy` now takes the
pubkeys directly and computes the owner hash itself (`owner_multisig` in the config), so what
the contracts are born with is provably the same address everyone derived.

## 6. Deploying with the multisig as owner (the FTB-14 flow)

On E's build box, after `make` is green:

```bash
cp cmd/deploy/config.example.json cmd/deploy/config.json
# fill rpc, wallet, contracts_dir, merkle_root_hex, timelock_blocks,
# and owner_multisig {m: 3, pubkeys: [...the five pubkeys...]}
export FTB_DEPLOY_PASSWORD='...'   # password stays out of the file (FTB-12)
go run ./cmd/deploy -config cmd/deploy/config.json
```

What happens: the tool deploys token, migrator, and registry with the **multisig hash as
owner from the first block** (deploy needs no owner signature; `_deploy` just stores it). Then,
because the deployer wallet cannot sign for the multisig, the tool prints the exact wiring
calls with the concrete hashes filled in:

```
1. <token>    setMigrator <migrator>
2. <migrator> configure <token> <root>
3. <registry> setInitialDelay <blocks>
4. <registry> bootstrap "token" <token>
5. <registry> bootstrap "migrator" <migrator>
6. <registry> finishBootstrap
```

The multisig members execute those six, in order, each transaction collecting 3 of 5
signatures (rehearse the coordination on TestNet - see §9; the exact offline-signing commands
depend on your pinned neo-cli/neo-go release, so pin them in this doc after the first
rehearsal). Publish the tool's full output next to `proofs.json` so anyone can verify the
wiring matches the published root and cap.

The one-way doors at the end of the migration window, also multisig-signed, in this order
(enforced in code, FTB-03):

```
7. <migrator> close              # seals the mint forever
8. <token>    renounceOwnership  # token becomes fully immutable
```

After step 8, verify on-chain: `totalSupply == maxSupply` (or less, if unclaimed), owner is
empty, `migrationOpen == false`. From that block on, the token has no admin at all.

## 7. The front-end

Keep it pure static files - the dApi adapter (`web/ftb-dapi.js`) holds no keys and every write
is a plan the wallet shows the user before signing, so a compromised host can lie about what
is displayed but cannot move funds (the signing boundary in the stack map). Still:

1. Build to static files. `ipfs add -r ./dist` on E's box, then `ipfs pin add <CID>` on all
   five VMs.
2. Serve via public gateways and the members' own gateways; attach a name with Neo Name
   Service or a DNS dnslink.
3. The server-hosted copy is a mirror, not the source of truth. Publish the CID and the
   config (all contract hashes) in the repo; the contract hashes in the config are the one
   thing a hostile mirror would change, so they must be checkable against the published list.
4. Ship the RPC failover list (A, B, C, plus public endpoints) with first-healthy-wins
   selection and a user-pastable endpoint (guide §12.5 has the snippet). A user running their
   own node cannot be censored at all.

## 8. Oracle committee (Launch 2 only)

Reporters on A's, B's, and C's VMs - three independent operators, each aggregating several
venues off-chain. Deploy oracle and lend with `owner` = the same multisig, call `setParams`
with `minReports` 3 (the shipped default of 1 is a placeholder, FTB-06), and enroll the three
reporters. The failure mode is safe by design: too few fresh reports makes `Price` revert,
which pauses borrowing and liquidations rather than trading on a bad number. Rehearse that
pause on TestNet.

## 9. TestNet rehearsal - the whole ceremony, end to end

The point of TestNet is to rehearse the *distributed operations* with the actual five people,
not just to run the contracts. Do every step for real:

- [ ] **T0 - stand up.** Five VMs hardened; three TestNet RPC nodes synced; archive node
      syncing; multisig created and all five members derive the same address.
- [ ] **T1 - deploy + wire.** `cmd/deploy` with `owner_multisig`; the six wiring calls
      executed by the multisig; output published. Everyone verifies `getMigrator`, the root,
      and the registry entries on-chain from their own node.
- [ ] **T2 - snapshot reproduction.** Two members independently build the snapshot and the
      Merkle root; roots compared byte-for-byte; `maxSupply` set to the exact printed total
      (FTB-01) and `proofs.json` published on IPFS.
- [ ] **T3 - migration window.** Community testers claim via NeoLine through different RPC
      endpoints, including at least one user on their own node. Bug bounty runs here.
      `cmd/seed` seeds the pools (empty-pool guard on, FTB-04).
- [ ] **T4 - failure drills.** (a) Kill RPC #1 - front-end fails over, nothing else notices.
      (b) Take down the hosted front-end - users switch to the IPFS mirror. (c) Stop one
      oracle reporter - price still quorums; stop two - `Price` reverts and borrowing pauses
      cleanly. (d) Run a full registry `propose` → wait timelock → `apply` cycle and watch the
      events in the open.
- [ ] **T5 - seal.** Multisig signs `close`, then `renounceOwnership`; verify owner empty and
      mint sealed on-chain.
- [ ] **T6 - go/no-go.** Every box above ticked, bounty findings resolved (no external
      audit - withdrawn per DECISIONS.md Decision 7). Only then does MainNet repeat this script.

## 10. Key hygiene

- Wallet passwords come from the environment (`password_env` in deploy config, FTB-12), never
  committed; `.gitignore` already excludes wallets and filled configs - keep it that way.
- MainNet multisig keys on hardware wallets; TestNet can be software keys.
- Each member's VM holds only their own key. There is no shared machine with all five.

## 11. What this achieves

After T5 on MainNet: the token is immutable with a sealed mint; the migrator is inert; the
swap is Flamingo's audited code with liquidity from many LPs; the registry/oracle/lend can
only change through a 3-of-5 multisig plus a public timelock; access rides on three
independent RPC operators plus public endpoints plus the user's own node; the front-end lives
on IPFS pinned by five machines; and the snapshot and vote are reproducible by anyone with an
archive node. No single member, VM, provider, or company can move funds, change the rules
quietly, censor a user, or take the app down.

---

*Operational plan, not financial or legal advice. Command names for multisig creation and
offline signing vary between neo-cli/neo-go releases - pin the exact commands here after the
first TestNet rehearsal. Nothing reaches MainNet until the pre-launch gate in
`docs/audits/AUDIT_REPORT.md` passes.*
