/*
 * PROFILE: TESTNET - the live TestNet deployment (committed 18 Oct 2026).
 * Activate with:  cp config.testnet.js config.js   (config.js stays git-ignored)
 * This is the only profile the QA gate (QA_GATE.md) allows to publish today.
 */
window.FTB_CONFIG = {
  network: 'TestNet',

  contracts: {
    // R4 reset (20 Oct 2026): token v2 - the claimed ledger lives INSIDE the token, so
    // no migrator swap can ever reopen a claim (the R2+R3 double-claim class is dead).
    ftb:      '0x1ff1f51e63179867367ae8982af583e01e7cbdbd', // FTB token v2 (cap 100M, per-account claim ledger)
    migrator: '0x4af54b861eafcff198892b38b8b60fd06f95dd2d', // migration claim R4 - stateless proof-checker, v2 tree (5,194 wallets), mints floor(FLM-equiv × 0.189832)
    registry: '0x0d59f3bb8018f5668ac4448249f21927fe961614', // seam board (re-pointed via timelock 20 Oct)
    oracle:   '0x1c92a93d438b9468b45706cbc4b1ddcc345df875', // Launch 2
    lend:     '0x3b319667f54a4888d0d366d4923d93a833baf8c5', // R4 reset: fresh state (FtbLendR2), same audited code
    // ballot is deployed per proposal; the hash IS the proposal id
    // R4 19 Oct: fresh tally (vote reset), electorate = v2 claim tree (5,194 wallets
    // incl. dev + salvage); voting window runs to 31 Oct - DECISIONS.md, Decision 14.
    ballot:   '0x2f76d354c1345946b0dc22d189724b71c7539690',
    // bond vault holds listing bonds (50/50 liquidity/rug-pull split)
    bondvault: '0x20663c31352c224da37cd136dab587f2da55d390', // R4 reset: fresh state (FtbBondVaultR2)
    // treasury escrows proposal budgets; dividends pays the holders' 2/9 to stakers
    treasury:  '0xa0bcfb4de8898d12a5535151fd80050c46391033', // R4 reset: fresh state (FtbTreasuryR2)
    dividends: '0xa1f8fcbdfee1446cd47d46d773d8b6a7f2542c63', // R4 reset: fresh state (FtbDividendsR2)
    // Discord bond system (deployed 22 Oct 2026; separate from the listing bondvault above):
    dbondvault: '0x62f053d7f360d286e6ee0551155d7724564fea2b', // FtbDiscordBond -- Discord entry-bond vault
    accusation: '0xfada35e4dd4fd8ce22c7badf2d6449b449aaeda9', // FtbBondAccusation -- the bond's own dispute ballot
    // bNEO (NeoBurger) - ratio-board listing only; lend collateral is native NEO + GAS.
    bneo:     '0x48c40d4666f93408be1bef038b6722404d9a4c2a',
    // router is the community AMM (reused audited Flamingo); pairs recompiled vs token v2
    router:   '0x7e3fa9356e3294acf709c51062ca051f1df6cb5c'
  },

  // native NEO and GAS, same on every Neo N3 network (NEO is 0 decimals, whole units)
  assets: {
    neo: '0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5',
    gas: '0xd2a4cff31913016155e38e474a2c06d08be276cf'
  },

  // index.html reads over raw JSON-RPC with failover (wallets sign separately).
  // community nodes first; public endpoints are appended automatically.
  // #1: ftb-node (1984.is Reykjavík) - neo-go T5 node behind Caddy TLS; syncs via
  // NeoFS block fetcher. HTTPS so the deployed site can call it (no mixed content).
  rpcs: [
    'https://freethebird.co/rpc'
    // 'https://node2.ftb.example',
    // 'https://node3.ftb.example',
    // 'https://node4.ftb.example',
    // 'https://node5.ftb.example'
  ],

  // who else serves this exact front end. the mirrors tile probes each base URL's
  // beat.json (written per deploy by ops/ipfs/ftb-push-ipfs.sh): reachable AND same
  // commit = live. add each community node's web URL as it comes online.
  mirrors: [
    'https://freethebird.co',
    'https://ipfs.io/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq',
    'https://dweb.link/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq',
    'https://w3s.link/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq'
  ],

  // live node health heartbeat: the node's /status.json, served by Caddy OUTSIDE the IPFS
  // bundle (so it is live between deploys). Absolute, so mirrors read the node too.
  statusUrl: 'https://freethebird.co/status.json',

  // the published migration proofs (also on IPFS; keep the CID in the repo)
  proofsUrl: './proofs.json',
  merkleRoot: '0x9a36b6a0faf6adddf727788e5a35ba9db48db14958043637fad67fb468243003',

  // set these only while a vote is live (the two published addresses)
  vote: { ftb: '', wait: '' },

  // committed economics (DECISIONS.md 18 Oct): hard cap 100,000,000 FTB.
  // r = 100,000,000 ÷ 526,782,179.10 = 0.189832 FTB per FLM/FLOCKS - the migrator
  // mints floor(entitlement × r), verifiable on-chain via migrator.quote().
  // the genesis vote weighs 1 FLM = 1 vote over the v2 claim tree (R4 ballot, 19 Oct -
  // 5,194 wallets, 526,782,179.10 FLM-equiv electorate, incl. dev + salvage wallets);
  // future votes weigh 1 FTB = 1 vote.
  governance: { ratio: 0.189832, electorate: 526782179.10 },

  // the live ballots (genesis votes - DECISIONS.md, Decisions 13-15): one contract per
  // proposal, the hash IS the proposal id. All three share the v2 electorate
  // (5,194 wallets) and the 31 Oct deadline. UI enforces the window; the contracts
  // themselves have no expiry - late votes still land on-chain.
  ballots: [
    { hash: '0x2f76d354c1345946b0dc22d189724b71c7539690', // FtbBallotR4 - ratio vote (reset)
      question: '1 FLM/FLOCKS = 0.189832 FTB - adopt?', options: ['adopt', 'wait'],
      endsAt: '2026-10-31T23:59:59Z' },
    { hash: '0xd9b19731ca159f956ab71e6d42dec7fef28a72e9', // FtbBallotLaunchDate
      question: 'MainNet launch date?', options: ['1 November', '7 November'],
      endsAt: '2026-10-31T23:59:59Z' },
    { hash: '0xc62269897f6747d52c90ca8fe6b9c9eb02d7f9ad', // FtbBallotClaimWindow
      question: 'FTB claim window - how long can holders claim?', options: ['6 months', '1 year'],
      endsAt: '2026-10-31T23:59:59Z' },
  ],

  // AMM pairs for ratio tracking (Section II): label + token hashes for router.getReserves.
  pairs: [
    { label: 'GAS/FTB',  a: 'GAS', b: 'FTB' },
    { label: 'NEO/FTB',  a: 'NEO', b: 'FTB' }
  ]
};
