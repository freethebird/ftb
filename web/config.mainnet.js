/*
 * PROFILE: MAINNET — sealed. Every contract hash is a 0x0000… placeholder:
 * with this profile the whole page renders its named standby states, which is
 * exactly what the QA gate (QA_GATE.md) verifies first. Hashes are filled ONLY
 * after the MainNet contracts are deployed, recorded in DECISIONS.md, and the
 * gate has passed on TestNet with this exact build. Activate with:
 *   cp config.mainnet.js config.js   (config.js stays git-ignored)
 */
window.FTB_CONFIG = {
  network: 'MainNet',

  contracts: {
    ftb:       '0x0000000000000000000000000000000000000000', // fill at MainNet deploy
    migrator:  '0x0000000000000000000000000000000000000000',
    registry:  '0x0000000000000000000000000000000000000000',
    oracle:    '0x0000000000000000000000000000000000000000',
    lend:      '0x0000000000000000000000000000000000000000',
    ballot:    '0x0000000000000000000000000000000000000000', // deployed per proposal
    bondvault: '0x0000000000000000000000000000000000000000',
    treasury:  '0x0000000000000000000000000000000000000000',
    dividends: '0x0000000000000000000000000000000000000000',
    // bNEO (NeoBurger) MainNet hash — ratio-board listing only.
    bneo:     '0x48c40d4666f93408be1bef038b6722404d9a4c2a',
    // router is the external Flamingo swap; set only when swap is wired and verified.
    router:   '0x0000000000000000000000000000000000000000'
  },

  // native NEO and GAS, same on every Neo N3 network (NEO is 0 decimals, whole units)
  assets: {
    neo: '0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5',
    gas: '0xd2a4cff31913016155e38e474a2c06d08be276cf'
  },

  // community MainNet nodes lead; public mainnet endpoints are appended automatically.
  // empty until the first community MainNet node is stood up — the page then reads
  // from the public fallback and says so in #rpcNote.
  rpcs: [
    // 'https://node1.ftb.example'
  ],

  // who else serves this exact front end (beat.json-probed; see MODULES.md module 8).
  mirrors: [
    'https://freethebird.co',
    'https://ipfs.io/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq',
    'https://dweb.link/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq',
    'https://w3s.link/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq'
  ],

  // the published migration proofs — the SAME pre-exploit snapshot binds MainNet:
  // same tree, same root, same electorate; the MainNet migrator is configured with it.
  proofsUrl: './proofs.json',
  merkleRoot: '0x9b4be5c9052570deccd7ce4fb90d1edb439d5f7f31c5c692e41a638904630cc8',

  // set these only while a vote is live
  vote: { ftb: '', wait: '' },

  // committed economics (DECISIONS.md 18 Oct): identical on MainNet — hard cap
  // 100,000,000 FTB, r = 0.189832, electorate 526,782,179.10 FLM-equiv.
  governance: { ratio: 0.189832, electorate: 526782179.10 },

  // no live ballot at MainNet launch
  ballot: { question: '', options: [] },

  // AMM pairs for ratio tracking, once pools exist on MainNet.
  pairs: [
    { label: 'bNEO/FTB', a: '0x48c40d4666f93408be1bef038b6722404d9a4c2a', b: 'FTB' },
    { label: 'GAS/FTB',  a: 'GAS', b: 'FTB' },
    { label: 'bNEO/GAS', a: '0x48c40d4666f93408be1bef038b6722404d9a4c2a', b: 'GAS' }
  ]
};
