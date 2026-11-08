# Security

Threat model, attack-vector register, key-custody and no-keys attestation, bug-bounty scope,
and vulnerability disclosures for FTB.

| document | what it covers |
|----------|----------------|
| [`KEYLESS_ATTESTATION.md`](KEYLESS_ATTESTATION.md) | the on-chain, replayable proof of which FTB contracts hold keys and which do not (Neo N3 MainNet) |
| [`GOV_RENOUNCE_MILESTONE.md`](GOV_RENOUNCE_MILESTONE.md) | the audit, testnet soak, and irreversible renounce sequence that makes the governance and earn stack keyless |
| [`DEX_SAFETY_SELF_ASSESSMENT.md`](DEX_SAFETY_SELF_ASSESSMENT.md) | FTB scored against the public DEX Safety rubric (Model 2, S1-S10); a self-assessment, not a third-party audit |
| [`WALLET_POLICY.md`](WALLET_POLICY.md) | why FTB connects wallets only through an injected desktop dAPI provider (NeoLine) and will not add WalletConnect or any runtime dependency |

See [`../README.md`](../README.md) for the full documentation map.
