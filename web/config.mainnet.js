/*
 * PROFILE: MAINNET - sealed. Every contract hash is a 0x0000… placeholder:
 * with this profile the whole page renders its named standby states, which is
 * exactly what the QA gate (QA_GATE.md) verifies first. Hashes are filled ONLY
 * after the MainNet contracts are deployed, recorded in DECISIONS.md, and the
 * gate has passed on TestNet with this exact build. Activate with:
 *   cp config.mainnet.js config.js   (config.js stays git-ignored)
 */
window.FTB_CONFIG = {
  network: 'MainNet',

  contracts: {
    ftb:       '0xf85f93d672cbf79f7749fb50a2d46a48421d7e71', // FreeTheBird (v4, cap 100M, per-account claim ledger)
    migrator:  '0x51a82390734d9d38cc89fed6bab26e72419c7ad2', // FtbMigratorR6 (root 235e7118, mints floor(FLM x 0.173173))
    registry:  '0xb796337672703c7d111364f3acf8dea54f7a714e', // FtbRegistry (finished, 2-day timelock)
    gov:          '0x0000000000000000000000000000000000000000', // FtbGov deployed 0x277095a7bc8567b4040ecf7b6253767d1451192f; dark until the governance milestone (audit + soak + renounce)
    loanbook:     '0x19414f2c1b04bd2fd705d065887ec7cbdc244aaf', // FtbLoanBook - oracleless, keyless P2P term loans (no owner key)
    commandments: '0x5d89ea321f6c0db71bcc91287c4f9fed8db59f6c', // FtbCommandments - on-chain charter (inked + renounced 6 Nov, immutable)
    ballot:    '0x0000000000000000000000000000000000000000', // deployed per proposal
    bondvault: '0x0000000000000000000000000000000000000000', // FtbBondVault deployed 0xc77d27432a2f82b4aeb30b81bbc5e012a04ddf1c; dark until the governance milestone
    treasury:  '0x0000000000000000000000000000000000000000', // FtbTreasury deployed 0x64f8fe20340211954ace5a99f63a05663c9a280c; dark until the governance milestone
    dividends: '0x0000000000000000000000000000000000000000', // FtbDividends deployed 0x5066582708c4d273b3a7b7eadf8dc94855e4c1a8; dark until the governance milestone
    // router is FTB's own C# AMM (self-contained, no Flamingo runtime dependency); set to the
    // deployed MainNet router after the AMM is deployed, seeded, and frozen.
    router:   '0x41ee590749816ef96a2adde02c375b8f64cb26af'
  },

  // deployed but SEALED: shown on the LO key panel (real hash, verified on-chain) and held
  // dark in `contracts` above until the governance milestone (audit + soak + renounce).
  sealed: {
    gov:       '0x277095a7bc8567b4040ecf7b6253767d1451192f',
    treasury:  '0x64f8fe20340211954ace5a99f63a05663c9a280c',
    bondvault: '0xc77d27432a2f82b4aeb30b81bbc5e012a04ddf1c',
    dividends: '0x5066582708c4d273b3a7b7eadf8dc94855e4c1a8'
  },

  // native NEO and GAS, same on every Neo N3 network (NEO is 0 decimals, whole units)
  assets: {
    neo: '0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5',
    gas: '0xd2a4cff31913016155e38e474a2c06d08be276cf'
  },

  // the FTB MainNet node (1984, Reykjavik) leads and is the menu default; the public
  // mainnet endpoints below are appended as dropdown fallbacks. served over Caddy TLS
  // at /rpc-mainnet so the deployed site reads it with no mixed content.
  rpcs: [
    'https://freethebird.co/rpc-mainnet'
  ],

  // known-good public Neo N3 MainNet RPCs - offered in the node dropdown so users can switch
  // when one is flaky; auto-pick + the read-failover already rotate among them.
  publicRpcs: [
    'https://mainnet1.neo.coz.io:443',
    'https://mainnet2.neo.coz.io:443',
    'https://n3seed1.ngd.network:443',
    'https://n3seed2.ngd.network:443',
    'https://rpc1.n3.nspcc.ru:20332',
    'https://neo3-mainnet.neoline.io'
  ],

  // who else serves this exact front end (beat.json-probed; see MODULES.md module 8).
  mirrors: [
    'https://freethebird.co',
    'https://ipfs.io/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq',
    'https://dweb.link/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq',
    'https://w3s.link/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq'
  ],

  // live node health heartbeat: the node's /status.json, served by Caddy OUTSIDE the IPFS
  // bundle (so it is live between deploys). Absolute, so mirrors read the node too.
  statusUrl: 'https://freethebird.co/status.json',

  // the published migration proofs - the SAME pre-exploit snapshot binds MainNet:
  // same tree, same root, same electorate; the MainNet migrator is configured with it.
  proofsUrl: './proofs_v4.json',
  merkleRoot: '0x235e7118608baf1880ef0fd50ed291900cbff26f6447725765403f85da1e75b4',

  // set these only while a vote is live
  vote: { ftb: '', wait: '' },

  // committed economics (DECISIONS.md 18 Oct): identical on MainNet - hard cap
  // 100,000,000 FTB, r = 0.173173, electorate 577,458,329.20 FLM-equiv (R6 tree, root 235e7118).
  governance: { ratio: 0.173173, electorate: 577458329.20, unsealAt: '2027-01-01T00:00:00Z' }, // unsealAt: published ISO date the sealed gov contracts renounce (TestNet soak as the safety net; no external audit); '' => "sealed"

  // no live ballot at MainNet launch
  ballot: { question: '', options: [] },

  // AMM pairs for ratio tracking, once pools exist on MainNet.
  pairs: [
    { label: 'GAS/FTB',  a: 'GAS', b: 'FTB' }
  ]
};
