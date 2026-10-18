/*
 * Copy to config.js and fill in per deployment. These are PLACEHOLDERS, not live hashes.
 * config.js is git-ignored. demo.html reads this to prefill its fields; the adapter also
 * accepts the same shape: new FtbDapi(window.FTB_CONFIG).
 */
window.FTB_CONFIG = {
  network: 'TestNet', // 'TestNet' first, 'MainNet' only after audit

  contracts: {
    ftb:      '0x0000000000000000000000000000000000000000', // FTB token
    migrator: '0x0000000000000000000000000000000000000000', // migration claim
    registry: '0x0000000000000000000000000000000000000000', // seam board
    oracle:   '0x0000000000000000000000000000000000000000', // Launch 2
    lend:     '0x0000000000000000000000000000000000000000', // Launch 2
    // ballot is deployed per proposal; the hash IS the proposal id
    ballot:   '0x0000000000000000000000000000000000000000',
    // bond vault holds listing bonds (50/50 liquidity/rug-pull split)
    bondvault: '0x0000000000000000000000000000000000000000',
    // treasury escrows proposal budgets; dividends pays the holders' 2/9 to stakers
    treasury:  '0x0000000000000000000000000000000000000000',
    dividends: '0x0000000000000000000000000000000000000000',
    // bNEO (NeoBurger) — ratio-board listing only; lend collateral is native NEO + GAS.
    // This is the MainNet hash; use the correct one for your network.
    bneo:     '0x48c40d4666f93408be1bef038b6722404d9a4c2a',
    // router is the external Flamingo swap; set only if you wire swap in your UI.
    router:   '0x0000000000000000000000000000000000000000'
  },

  // native NEO and GAS, same on every Neo N3 network (NEO is 0 decimals, whole units)
  assets: {
    neo: '0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5',
    gas: '0xd2a4cff31913016155e38e474a2c06d08be276cf'
  },

  // index.html reads over raw JSON-RPC with failover (wallets sign separately).
  // list your five community nodes first; public endpoints are appended automatically.
  rpcs: [
    // 'https://node1.ftb.example',
    // 'https://node2.ftb.example',
    // 'https://node3.ftb.example',
    // 'https://node4.ftb.example',
    // 'https://node5.ftb.example'
  ],

  // who else serves this exact front end. the mirrors tile probes each base URL's
  // beat.json (written per deploy by ops/ipfs/ftb-push-ipfs.sh): reachable AND same
  // commit = live, reachable but older = stale, no answer = down. our own node(s),
  // then the public IPFS gateways resolving the IPNS name.
  mirrors: [
    // 'https://freethebird.co',
    // 'https://ipfs.io/ipns/<IPNS_NAME>',
    // 'https://dweb.link/ipns/<IPNS_NAME>',
    // 'https://w3s.link/ipns/<IPNS_NAME>'
  ],

  // the published migration proofs (also on IPFS; keep the CID in the repo)
  proofsUrl: './proofs.json',
  merkleRoot: '0x9b4be5c9052570deccd7ce4fb90d1edb439d5f7f31c5c692e41a638904630cc8',

  // set these only while a vote is live (the two published addresses)
  vote: { ftb: '', wait: '' },

  // committed economics (DECISIONS.md 18 Oct): hard cap 100,000,000 FTB.
  // r = 100,000,000 ÷ 526,782,179.10 = 0.189832 FTB per FLM/FLOCKS — the migrator
  // mints floor(entitlement × r), verifiable on-chain via migrator.quote().
  // the genesis vote weighs 1 FLM = 1 vote over the snapshot electorate (FLM-equiv);
  // future votes weigh 1 FTB = 1 vote. electorate drives the turnout readout.
  governance: { ratio: 0.189832, electorate: 0 },

  // the live ballot: human-readable question + option labels, matching the on-chain order
  // (option 0 is the first label). Leave question empty to hide the governance section.
  ballot: { question: '', options: [] },

  // AMM pairs for ratio tracking: label + the two token hashes passed to router.getReserves.
  // These are the intended MainNet tokens; on TestNet set your deployed stand-ins.
  pairs: [
    { label: 'bNEO/FTB', a: '0x48c40d4666f93408be1bef038b6722404d9a4c2a', b: 'FTB' },
    { label: 'GAS/FTB',  a: 'GAS', b: 'FTB' },
    { label: 'bNEO/GAS', a: '0x48c40d4666f93408be1bef038b6722404d9a4c2a', b: 'GAS' }
  ]
};
