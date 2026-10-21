# Data

Published datasets that the documentation and the on-chain migration cite - kept in the repo
so anyone can reproduce the claim tree and verify the numbers.

| folder | what goes here |
|--------|----------------|
| `migration/` | the claim lists (`ftb_claim_list*.csv`), the merkle `proofs_v2.json`, and the flagged-wallet list |
| `salvage/` | LP, vault, orderbook, and broker salvage snapshots |

The merkle root published in `web/config.testnet.js` is reproducible from
`migration/proofs_v2.json` (built by `cmd/merkleroot`). See `../snapshot/README.md` for the
end-to-end reproduction from raw chain state.
