# ftb-discord

Two Discord bots for the Freethebird (FTB) **Discord bond** module, built in Go
(`discordgo` + `neo-go`), read-only against Neo N3, **keyless**, and
**interface-first** (the on-chain contracts are not deployed yet — this code
defines the interface they must conform to).

| Bot | Binary | Job | Discord privilege |
|-----|--------|-----|-------------------|
| **Verify** | `ftb-verifybot` | `/verify` → modal (tx id / wallet / bond id) → check the deposit on-chain → grant the **Bonded** role | Guilds; **Manage Roles** |
| **Sentinel** | `ftb-sentinelbot` | watch messages → detect spam → if the spammer is **bonded**, post a keyless accusation to the liquidation-ballot channel | Guilds, Guild Messages, **Message Content** (privileged) |

Neither bot holds a wallet or a key. The verify bot only *reads* the chain and
grants a role. The sentinel only *reads* the chain and *posts* a message with a
link; a bonded member signs the actual on-chain `fileCase` / `vote` transaction
with **their own** wallet on the member-operated signing page. No admin key can
move, file against, or liquidate a bond — only the vote does.

---

## Design choices (locked for this build)

- **Runtime:** Go — one language with the neo-go contracts, native Neo N3 RPC,
  two sanctioned deps (`discordgo`, `neo-go`), one static binary per bot.
- **Accusation path:** *keyless flag-and-hand-off.* The sentinel never signs.
- **Contracts:** *interface-first.* The vault/ballot aren't deployed; the method
  surface and stack encodings below are the spec the contracts must meet. Until
  a hash is configured the bot runs in **standby** and says so (earned-or-standby,
  the backend analog of the web modules' R5/R8).

### Canon mapping (Modus Operandi, backend analog)

- **No keys** anywhere — tokens come from the environment, never from config or
  code, and are never logged. On-chain writes are always member-signed.
- **Earned-or-standby** — a feature whose contract hash is unset reports standby
  instead of faking a result.
- **Validate all external input** — every user-supplied tx/wallet/bond string
  and every message is validated before it reaches an RPC call (the backend
  equivalent of the web "no innerHTML, build node-by-node" rule).
- **One responsibility per package;** least privilege per bot (the sentinel
  cannot manage roles; the verify bot cannot read messages).

---

## Layout

```
cmd/verifybot      Bot 1 — verification (slash command + modal + role grant)
cmd/sentinelbot    Bot 2 — spam detection + keyless accusation
internal/config    env secrets + JSON ops config; standby flags (std-lib only)
internal/neo       read-only Neo N3 seam: vault, ballot, NEP-17 corroboration
internal/store     verify bot's local Discord↔bond cache (atomic JSON)
internal/handoff   keyless sign-to-file / sign-to-vote deep-link builder
internal/spam      configurable spam-detection engine (pure logic, unit-tested)
config.example.json
```

The two bots share the `internal/*` packages but are **not coupled through a
file**: the sentinel resolves a flagged member to their bond on demand from the
chain (`getBondByDiscord`), not from the verify bot's cache.

---

## Build & run

```sh
go build ./...                       # compile everything
go test ./...                        # unit tests
go build -o bin/ftb-verifybot   ./cmd/verifybot
go build -o bin/ftb-sentinelbot ./cmd/sentinelbot
```

Configuration is a JSON file (operational, no secrets) plus two environment
variables (secrets):

```sh
cp config.example.json config.json   # then edit the IDs / endpoint
export FTB_VERIFY_BOT_TOKEN=…         # verify bot's token
export FTB_SENTINEL_BOT_TOKEN=…       # sentinel bot's token
export FTB_CONFIG=./config.json       # optional; defaults to ./config.json

./bin/ftb-verifybot      # run bot 1
./bin/ftb-sentinelbot    # run bot 2 (separate process / token)
```

Leave `contracts.vault_hash` / `contracts.ballot_hash` empty to run in standby
(useful before the contracts exist, and in preview). Set them (and
`neo.ftb_token_hash`, `handoff.sign_base_url`) to go live — no code change.

### Discord setup

Create **two** bot applications (one token each) so privileges stay separate.

- **Verify bot:** scope `applications.commands` + `bot`; permission **Manage
  Roles**. The **Bonded** role must sit *below* the verify bot's own top role in
  the role list, or Discord refuses the grant. No Message Content intent.
- **Sentinel bot:** scope `bot`; enable the **Message Content** privileged
  intent in the Developer Portal (Bot → Privileged Gateway Intents). It needs
  read access to the channels it should watch and send access to the ballot /
  mod-alert channels. It needs **Moderate Members** *only* if you turn on
  `spam.auto_moderate` (off by default).

Put mod and bot roles in `discord.ignore_role_ids` so the sentinel never flags
them.

---

## Contract interface (interface-first — the spec for the contract/git chat)

The bots call these **read-only** methods. The contracts must emit exactly these
stack shapes. Method names are overridable in `config.contracts.methods` so the
bots can match the final manifest without a rebuild.

### Bond-vault contract

| Method | Params | Returns |
|--------|--------|---------|
| `getBond` | `bondId: Integer` | Bond struct (below), or `Null` if none |
| `getBondByDiscord` | `discordUserId: ByteString` | `Integer` bondId (`0` if none) |
| `bondAmount` | — | `Integer` fixed bond in base units (expect `100e8`) |

**Bond struct** (Array/Struct, in order):

```
[0] owner      ByteString  20-byte script hash (big-endian) of the depositor wallet
[1] amount     Integer     bond amount in FTB base units (8 decimals)
[2] discordId  ByteString  depositor's immutable Discord user ID (snowflake, UTF-8)
[3] status     Integer     0 none | 1 active | 2 refunded | 3 liquidated
[4] depositTx  ByteString  32-byte deposit tx hash (big-endian)
[5] createdAt  Integer     unix-ms
```

> **Tag refinement:** the handoff doc said "tagged with the Discord **handle**."
> This build pins the tag to the Discord **user ID** (snowflake) instead —
> handles are mutable, IDs are not — so the bond↔member link never breaks on a
> rename. The contract should store and index the user ID.

### Accusation-ballot contract (separate from protocol governance)

| Method | Params | Returns |
|--------|--------|---------|
| `getCase` | `caseId: Integer` | Case struct (below), or `Null` |
| `listOpenCases` | — | `Array<Integer>` of open case ids |
| `isBondedVoter` | `wallet: Hash160` | `Bool` (holds an active bond) |
| `fileCase` *(member-signed)* | `accusedBondId: Integer, reason: Integer, evidence: ByteString(32)` | `Integer` caseId |
| `vote` *(member-signed)* | `caseId: Integer, choice: Integer` | `Bool` |

**Case struct** (Array/Struct, in order):

```
[0] accusedBondId  Integer
[1] reasonCode     Integer     (see handoff reason codes below)
[2] evidenceHash   ByteString  32-byte sha-256 of the off-chain evidence bundle
[3] forVotes       Integer     votes to LIQUIDATE
[4] againstVotes   Integer     votes to DISMISS
[5] status         Integer     0 open | 1 liquidated | 2 dismissed
[6] deadline       Integer     unix-ms voting deadline
[7] threshold      Integer     votes required to resolve (1 bonded member = 1 vote)
```

`fileCase` and `vote` are **never** called by a bot — they are built and signed
by a bonded member on the signing page. The contract must gate both on
`isBondedVoter(caller)` and enforce one-member-one-vote and burn-on-liquidation
with no admin override.

---

## Keyless hand-off URL contract (signing-page dependency)

When the sentinel posts an accusation it attaches a **Link button** to the
member-operated signing page. Parameters ride in the URL **fragment** (after
`#`), so they never reach the IPFS gateway or server logs.

```
file a case:  {sign_base_url}#/file-case?accused={bondId}&reason={code}&evidence={sha256hex}&nonce={hex}
vote:         {sign_base_url}#/vote?case={caseId}&choice={liquidate|dismiss}
```

The signing page (web/git track — **not built here**) must: parse
`location.hash`, build the `fileCase` / `vote` invocation, have the member sign
with NeoLine / WalletConnect, broadcast, and show the resulting txid. The bot
then reads the ballot to confirm the case/vote landed.

**Reason codes** (`internal/handoff`): `0` unspecified, `1` message-rate,
`2` duplicate-content, `3` mass-mention, `4` link-flood, `5` invite-spam,
`6` post-join burst.

---

## Spam detection (tunable, no fixed policy)

`internal/spam` scores each message over a per-user sliding window; rules that
trip add their weight, and a total ≥ `score_threshold` flags the message. All
knobs live in `config.spam`:

- **rate** — ≥ `rate_messages` in `window_seconds`
- **duplicate** — ≥ `duplicate_count` near-duplicates (token-set Jaccard ≥
  `duplicate_similarity`)
- **mass-mention** — ≥ `max_mentions` (an `@everyone` trips this on its own)
- **link-flood** — ≥ `max_links` URLs; **invite-spam** — any `discord.gg/…`
- **post-join burst** — rate rule within `join_grace_minutes` of joining

Defaults need two independent signals to flag (threshold `2.0`), which keeps a
single borderline message from tripping. A flagged user is reported at most once
per 10 minutes, and the bot skips a new accusation if the ballot already has an
open case for that bond.

`auto_moderate` (default **off**) optionally lets the sentinel timeout/delete
itself; left off, it only detects and reports, and humans act — least privilege.

---

## Standby behaviour (what runs before the contracts exist)

| Feature | Standby (no hash) | Live (hash set) |
|---------|-------------------|-----------------|
| `/verify` input validation | ✅ runs | ✅ runs |
| On-chain bond check + role grant | tells user vault isn't live | ✅ verifies + grants |
| Spam detection | ✅ runs | ✅ runs |
| Bonded-member resolution | falls back to mod alert | ✅ resolves from chain |
| Accusation `file` button | shows "signing page standby" | ✅ keyless link |

---

## Security notes

- **Public data only.** The verify bot handles the tx id, wallet address and
  bond id — all public on-chain data. It stores nothing else.
- **No keys.** Grep the tree: there is no wallet, no private key, no signer.
- **Authoritative check.** Verification gates on the vault's own `getBond`
  record; the NEP-17 Transfer scan is a secondary corroboration (logged).
- **WIRE-LIVE TODO (endianness).** NEP-17 Transfer `from`/`to` byte order is
  matched endianness-tolerantly (`internal/neo/nep17.go`) so a real deposit is
  never rejected on a guess. Pin the exact order against a real FTB testnet
  Transfer when the vault goes live, then tighten the matcher.

## Not built here / next steps

1. **Contracts** — the bond-vault + accusation-ballot (neo-go), to the interface
   above. Contract/git chat.
2. **Signing page** — the member-wallet `file-case` / `vote` page honouring the
   URL contract above. Web/git chat.
3. **Wire live** — set `vault_hash`, `ballot_hash`, `ftb_token_hash`,
   `sign_base_url`; confirm the NEP-17 endianness TODO.

Commits to `github.com/freethebird/ftb` happen in the git chat under the
`freethebird` identity — this is a build/dev deliverable.
