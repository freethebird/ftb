/*
 * PROFILE: TESTNET - the live TestNet deployment (committed 18 Oct 2026).
 * Activate with:  cp config.testnet.js config.js   (config.js stays git-ignored)
 * This is the only profile the QA gate (QA_GATE.md) allows to publish today.
 */
window.FTB_CONFIG = {
  network: 'TestNet',

  contracts: {
    ftb:      '0xdc82bb0c89085dfeb81a40cebce786cb1b73044c', // FTB token (TestNet, cap 100M committed 18 Oct via pre-launch update)
    migrator: '0xe4f91c4957670ae295d55efc0e8ac6e3c5dcfc99', // migration claim R3 - v2 tree (5,194 wallets incl. salvage + dev allocation), mints floor(FLM-equiv × 0.189832)
    registry: '0x0d59f3bb8018f5668ac4448249f21927fe961614', // seam board
    oracle:   '0x1c92a93d438b9468b45706cbc4b1ddcc345df875', // Launch 2
    lend:     '0xae55c7b4950f9da4486baafdfd6ba2ae5ebdf8f4', // Launch 2 - v2 18 Oct: divUp rounding fix (dust debts repayable), adds owner-guarded update
    // ballot is deployed per proposal; the hash IS the proposal id
    // R4 19 Oct: fresh tally (vote reset), electorate = v2 claim tree (5,194 wallets
    // incl. dev + salvage); voting window runs to 31 Oct - DECISIONS.md, Decision 14.
    ballot:   '0x2f76d354c1345946b0dc22d189724b71c7539690',
    // bond vault holds listing bonds (50/50 liquidity/rug-pull split)
    bondvault: '0x029785b1a2b395a295ebf96f7d68a3cee9afb967',
    // treasury escrows proposal budgets; dividends pays the holders' 2/9 to stakers
    treasury:  '0xcac71ad3478491401ec72c35831521437d06aea6',
    dividends: '0xb98cd5d04b20b84fcef0fc9d305dd5092c5a0df9', // v2 18 Oct: monthly revenue ledger + update method,
    // bNEO (NeoBurger) - ratio-board listing only; lend collateral is native NEO + GAS.
    bneo:     '0x48c40d4666f93408be1bef038b6722404d9a4c2a',
    // router is the external Flamingo swap; no AMM on TestNet (neo#3285) - placeholder.
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
