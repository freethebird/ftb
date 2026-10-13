# T0 onboarding: the five operators

One page per role. Send each member only their section plus the common parts. The full
context lives in `ops/DECENTRALIZED_OPS.md`; this is the "what do I actually do this week"
version. Everything here is TestNet-first; nothing touches MainNet value.

## Everyone (all five)

1. Provision a VM: 2 vCPU, 4 GB RAM, 80 GB NVMe, Ubuntu 24.04 or Debian 12. Different
   provider and jurisdiction per person — that spread is the point.
2. Harden it per `ops/1984_SETUP.md` section 2 (non-root user, key-only SSH, ufw, automatic
   security updates).
3. Generate your multisig keypair on the VM (never on a shared machine):

   ```bash
   # neo-cli console (download from github.com/neo-project/neo-node/releases)
   create address
   list key        # shows your compressed public key (33 bytes, starts 02 or 03)
   ```

   (Command names per the pinned neo-cli release; `help` in the console lists them.)
4. Send the coordinator **only your public key**. Your private key never leaves your VM.
   For MainNet later, this key moves to a hardware wallet.
5. When the coordinator publishes the five pubkeys, independently derive the multisig
   address: `import multisigaddress 3 <pk1> <pk2> <pk3> <pk4> <pk5>`. If your result differs
   from the published address by even one character, stop and say so — that check is how we
   catch a typo or a substitution before it matters.
6. Install an IPFS node (kubo); when the front-end CID is published, `ipfs pin add <CID>`.

**Send back to the coordinator:** your pubkey, your VM's role confirmation, your RPC endpoint
URL (A/B/C only, once live).

## Member A, B, C — RPC node operators

1. Run a Neo N3 full node (guide §12.2, docker one-liner) with the **RpcServer** and
   **TokensTracker** plugins. For T0 this is TestNet: ports 20332/20333, magic 894710606.
2. Wait for full sync (`show state`, Height matches the network).
3. Put Caddy or nginx in front for TLS; open only 443 and 20333 in ufw.
4. Hand your `https://` endpoint URL to the coordinator for the front-end failover list.
5. Launch 2 (later): your VM also runs an oracle reporter — keep some TestNet GAS aside.

## Member D — archive node

Same node setup, plus the **StateService** plugin, 8 GB RAM, 250 GB+ disk, and **no pruning —
ever**. Your node is how anyone re-derives the pre-exploit snapshot and the vote tally without
trusting a third-party indexer. During T2 you will re-run `snapshot_state.py` against your own
node and compare the result with the coordinator's independent run.

## Member E — build box and release coordinator

1. 4 GB RAM is comfortable for `go test`; follow `ops/1984_SETUP.md` end to end
   (it installs Go 1.26+, the neo-go compiler, compiles all contracts, runs all 28 tests).
2. Your box holds the deploy tooling (`cmd/deploy`, `cmd/merkleroot`) and the seed config.
3. Wallet password comes from the environment, never the config file:
   `export FTB_DEPLOY_PASSWORD=...` before any `go run ./cmd/...`.
4. You run the deploy with `owner_multisig` filled with the five pubkeys, then circulate the
   printed wiring calls for multisig signature (T1).

## The coordinator's T0 checklist

- [ ] Five pubkeys collected; all five members confirm the same 3-of-5 address
- [ ] Three TestNet RPC endpoints live behind TLS (A, B, C)
- [ ] Archive node syncing (D)
- [ ] Build box green: `make` passes, contracts in `build/` (E)
- [ ] TestNet GAS in the deploy wallet (from the TestNet faucet) (E)
- [ ] `cmd/deploy/config.json` filled: TestNet RPC, `owner_multisig` pubkeys,
      `merkle_root_hex` = 0x639aaa7801f41c75d15c75aaad31aa6d6ede2b8bb81bd30f7ebf9560d50a8bba,
      `timelock_blocks` small for TestNet (e.g. 20)
- [ ] Then T1: deploy + multisig wiring, exactly as printed by the tool

---

*Operational guide, not financial or legal advice. Pin exact CLI command names after the first
TestNet rehearsal; they shift between neo-cli releases.*
