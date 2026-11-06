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
    ftb:      '0xce37caa3f6c0ac6d4cbd82b14ccb8ec8740f3e43', // FTB token (v4 stack, cap 100M, per-account claim ledger)
    migrator: '0x08c02e1887b652f9b1cb6a17c668fc9af28047ac', // migration claim R6 - stateless proof-checker, v4 tree (5,233 wallets), mints floor(FLM-equiv × 0.173173)
    registry: '0x81eadc5714730359a24c80b155a7138cfeeee4ef', // seam board (v4 stack)
    // keyless/de-keyed stack - fill after the de-keyed full stack is deployed to TestNet (cmd/deploy testnet launch2):
    gov:          '0x22c2f6c04a0dede8eeb294a5fa87d1125f462fda', // FtbGov (v4 stack)
    loanbook:     '0x231441f837c674d198177d122c23d4d65c438107', // FtbLoanBook (oracleless P2P lend, v4 stack)
    commandments: '0xe473a0ce2d2710e57c04edbcb9fc45a4be8cee89', // FtbCommandments
    // ballot is deployed per proposal; the hash IS the proposal id
    // R4 19 Oct: fresh tally (vote reset), electorate = v2 claim tree (5,194 wallets
    // incl. dev + salvage); voting window runs to 31 Oct - DECISIONS.md, Decision 14.
    ballot:   '0x35f592f715edf14279610cfe08b7bdde8e22ceab',
    // bond vault holds listing bonds (50/50 liquidity/rug-pull split)
    bondvault: '0x83377a037c6b05b63a15592717252c05b0cfbe36', // v4 stack (FtbBondVault)
    // treasury escrows proposal budgets; dividends pays the holders' 2/9 to stakers
    treasury:  '0x21f70b15f2f0e1f41cc689a0f304849ef23ec5e4', // v4 stack (FtbTreasury)
    dividends: '0xd4f64a7f28a34bbfc7fd42916c218cc4b4fbd597', // v4 stack (FtbDividends)
    // Discord bond system (deployed 22 Oct 2026; separate from the listing bondvault above):
    dbondvault: '0x6089a9daf016b23c9432675811f4a7c5a238679d', // FtbDiscordBond -- Discord entry-bond vault
    accusation: '0xaf8edfdddb34179074cac2f4aee6879dfd6f7300', // FtbBondAccusation -- the bond's own dispute ballot
    // router is the community AMM (reused audited Flamingo); pairs recompiled vs token v4 (admin NMK83, frozen post-seed)
    router:   '0x6b98c58d952c804880b03acaff4dd767810641f3'
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
  proofsUrl: './proofs_v4.json',
  merkleRoot: '0x235e7118608baf1880ef0fd50ed291900cbff26f6447725765403f85da1e75b4',

  // set these only while a vote is live (the two published addresses)
  vote: { ftb: '', wait: '' },

  // committed economics (DECISIONS.md 18 Oct): hard cap 100,000,000 FTB.
  // r = 100,000,000 ÷ 526,782,179.10 = 0.189832 FTB per FLM/FLOCKS - the migrator
  // mints floor(entitlement × r), verifiable on-chain via migrator.quote().
  // the genesis vote weighs 1 FLM = 1 vote over the v2 claim tree (R4 ballot, 19 Oct -
  // 5,194 wallets, 526,782,179.10 FLM-equiv electorate, incl. dev + salvage wallets);
  // future votes weigh 1 FTB = 1 vote.
  governance: { ratio: 0.173173, electorate: 577458329.20 },

  // the live ballots (genesis votes - DECISIONS.md, Decisions 13-15): one contract per
  // proposal, the hash IS the proposal id. All three share the v2 electorate
  // (5,194 wallets) and the 31 Oct deadline. UI enforces the window; the contracts
  // themselves have no expiry - late votes still land on-chain.
  ballots: [
    { hash: '0x35f592f715edf14279610cfe08b7bdde8e22ceab', // FtbBallotR4 - ratio vote (reset)
      question: '1 FLM/FLOCKS = 0.173173 FTB - adopt?', options: ['adopt', 'wait'],
      endsAt: '2026-10-31T23:59:59Z' },
    { hash: '0x173c86b8a830a963d4d1562095ecb8c783a8ab1f', // FtbBallotLaunchDate
      question: 'MainNet launch date?', options: ['1 November', '7 November'],
      endsAt: '2026-10-31T23:59:59Z' },
    { hash: '0x22d76763a29724c75153c4112fc8ca8a655f6c24', // FtbBallotClaimWindow
      question: 'FTB claim window - how long can holders claim?', options: ['6 months', '1 year'],
      endsAt: '2026-10-31T23:59:59Z' },
  ],

  // AMM pairs for ratio tracking (Section II): label + token hashes for router.getReserves.
  pairs: [
    { label: 'GAS/FTB',  a: 'GAS', b: 'FTB' },
    { label: 'NEO/FTB',  a: 'NEO', b: 'FTB' }
  ]
};
