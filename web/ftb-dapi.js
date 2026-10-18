/*
 * ftb-dapi.js : a dependency-free adapter between a web page and the FTB contracts on Neo N3,
 * driven through the NeoLine dApi (and other dApi wallets: O3, OneGate use the same interface).
 *
 * Design, matching the project MO:
 *   - No dependencies, one file, inlinable. Nothing is fetched.
 *   - Read-only by nature. Builders are PURE: they return a "plan" object describing exactly
 *     one invocation. Nothing is signed until you pass a plan to execute(). The plan carries a
 *     human summary so a UI can show what will be signed BEFORE signing.
 *   - Contract hashes come from config, never hard-coded. Point it at TestNet first.
 *   - Least privilege: writes default to the CalledByEntry witness scope (the wallet signs for
 *     the connected account and nothing broader). Swap, which hops through the router, can take
 *     an explicit CustomContracts signer.
 *
 * Wallet generations: this drives the widely supported N3 dApi object (window.NEOLineN3). Newer
 * NeoLine also exposes a NEP-21 provider (window event Neo.DapiProvider.ready); detectNep21()
 * reports it so a page can tell the user, but invokes in this version go through the N3 dApi.
 *
 * Reference code. Internal audit lineage only - no external audit was performed
 * (DECISIONS.md, Decision 7). Test these flows on TestNet before wiring to MainNet value.
 */
(function (global) {
  'use strict';

  var MAGIC = { MainNet: 860833102, TestNet: 894710606 };
  var WITNESS = { None: 0, CalledByEntry: 1, CustomContracts: 16, CustomGroups: 32, Global: 128 };
  var GAS = '0xd2a4cff31913016155e38e474a2c06d08be276cf'; // native GAS, 8 decimals
  var NEO = '0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5'; // native NEO, 0 decimals (whole units)
  var DEC = 8; // FTB and GAS are 8 decimals; NEO is whole-unit, never scale it

  // ---------------- byte / number helpers (no deps) ----------------
  function hexToBytes(hex) {
    hex = String(hex).replace(/^0x/, '');
    if (hex.length % 2) hex = '0' + hex;
    var a = new Uint8Array(hex.length / 2);
    for (var i = 0; i < a.length; i++) a[i] = parseInt(hex.substr(i * 2, 2), 16);
    return a;
  }
  function bytesToHex(b) {
    var s = '';
    for (var i = 0; i < b.length; i++) s += b[i].toString(16).padStart(2, '0');
    return s;
  }
  function bytesToB64(b) {
    var s = '';
    for (var i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
    return global.btoa(s);
  }
  function b64ToBytes(s) {
    var bin = global.atob(s), a = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) a[i] = bin.charCodeAt(i);
    return a;
  }
  function hexToB64(hex) { return bytesToB64(hexToBytes(hex)); }
  function b64ToHex(s) { return bytesToHex(b64ToBytes(s)); }
  function reverseHex(hex) { return bytesToHex(hexToBytes(hex).reverse()); }

  // decimal string <-> base units (BigInt-safe, no floats)
  function toBaseUnits(dec, decimals) {
    if (decimals == null) decimals = DEC;
    dec = String(dec).trim();
    var neg = dec[0] === '-'; if (neg) dec = dec.slice(1);
    var parts = dec.split('.'), i = parts[0] || '0', f = parts[1] || '';
    f = (f + '0'.repeat(decimals)).slice(0, decimals);
    var v = BigInt((i + f).replace(/^0+(?=\d)/, '') || '0');
    return (neg ? -v : v).toString();
  }
  function fromBaseUnits(base, decimals) {
    if (decimals == null) decimals = DEC;
    var v = BigInt(base), neg = v < 0n; if (neg) v = -v;
    var s = v.toString().padStart(decimals + 1, '0');
    var i = s.slice(0, s.length - decimals), f = s.slice(s.length - decimals).replace(/0+$/, '');
    return (neg ? '-' : '') + i + (f ? '.' + f : '');
  }

  // base58check decode of a Neo N3 address -> 20-byte script hash hex (no 0x). By default this
  // returns the address-embedded (big-endian) order; pass {le:true} for little-endian. NeoLine's
  // signer.account wants little-endian: verified that NaUjKgf5vMuFt7Ffgfffcpc41uH3adx1jq ->
  // 2cab903ff032ac693f8514581665be534beac39f is the {le:true} output, so signerFor() uses LE.
  var B58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
  function base58Decode(str) {
    var bytes = [0];
    for (var i = 0; i < str.length; i++) {
      var c = B58.indexOf(str[i]);
      if (c < 0) throw new Error('bad base58 char: ' + str[i]);
      var carry = c;
      for (var j = 0; j < bytes.length; j++) { carry += bytes[j] * 58; bytes[j] = carry & 0xff; carry >>= 8; }
      while (carry) { bytes.push(carry & 0xff); carry >>= 8; }
    }
    for (var k = 0; k < str.length && str[k] === '1'; k++) bytes.push(0);
    return new Uint8Array(bytes.reverse());
  }

  // SHA-256 over a Uint8Array, pure JS, no deps (Article 2): the address checksum below must
  // not depend on crypto.subtle being present or synchronous. Standard FIPS 180-4.
  var SHA256_K = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2];
  function sha256(data) {
    var h = [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19];
    var bitLen = data.length * 8;
    var padded = Math.ceil((data.length + 9) / 64) * 64;
    var m = new Uint8Array(padded);
    m.set(data);
    m[data.length] = 0x80;
    var dv = new DataView(m.buffer);
    dv.setUint32(padded - 4, bitLen >>> 0, false);
    dv.setUint32(padded - 8, Math.floor(bitLen / 0x100000000), false);
    var w = new Int32Array(64);
    for (var off = 0; off < padded; off += 64) {
      for (var t = 0; t < 16; t++) w[t] = dv.getInt32(off + t * 4, false);
      for (t = 16; t < 64; t++) {
        var x = w[t - 15], y = w[t - 2];
        var s0 = ((x >>> 7) | (x << 25)) ^ ((x >>> 18) | (x << 14)) ^ (x >>> 3);
        var s1 = ((y >>> 17) | (y << 15)) ^ ((y >>> 19) | (y << 13)) ^ (y >>> 10);
        w[t] = (w[t - 16] + s0 + w[t - 7] + s1) | 0;
      }
      var a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
      for (t = 0; t < 64; t++) {
        var S1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7));
        var ch = (e & f) ^ (~e & g);
        var t1 = (hh + S1 + ch + SHA256_K[t] + w[t]) | 0;
        var S0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10));
        var maj = (a & b) ^ (a & c) ^ (b & c);
        var t2 = (S0 + maj) | 0;
        hh = g; g = f; f = e; e = (d + t1) | 0; d = c; c = b; b = a; a = (t1 + t2) | 0;
      }
      h[0] = (h[0] + a) | 0; h[1] = (h[1] + b) | 0; h[2] = (h[2] + c) | 0; h[3] = (h[3] + d) | 0;
      h[4] = (h[4] + e) | 0; h[5] = (h[5] + f) | 0; h[6] = (h[6] + g) | 0; h[7] = (h[7] + hh) | 0;
    }
    var out = new Uint8Array(32);
    var odv = new DataView(out.buffer);
    for (var i2 = 0; i2 < 8; i2++) odv.setUint32(i2 * 4, h[i2] >>> 0, false);
    return out;
  }

  // Full base58check verification (FTB-18/FTB-25, closed): version byte must be 0x35 (Neo N3)
  // and the trailing 4 bytes must match the double-SHA256 checksum, so a mistyped or corrupted
  // address is rejected here instead of silently deriving a wrong script hash.
  var N3_ADDRESS_VERSION = 0x35;
  function addressToScriptHashHex(address, opts) {
    var raw = base58Decode(address);
    if (raw.length !== 25) throw new Error('bad address length');
    if (raw[0] !== N3_ADDRESS_VERSION) throw new Error('bad address version byte (not Neo N3)');
    var body = raw.slice(0, 21);
    var sum = sha256(sha256(body));
    for (var i = 0; i < 4; i++) {
      if (raw[21 + i] !== sum[i]) throw new Error('bad address checksum');
    }
    var sh = raw.slice(1, 21); // 20-byte script hash, big-endian as the address stores it
    var hex = bytesToHex(sh);
    return (opts && opts.le) ? reverseHex(hex) : hex;
  }

  // base58 encode — the inverse of base58Decode above. Used only to RENDER addresses
  // from chain data (storage keys); decode remains the one path that ingests text.
  function base58Encode(bytes) {
    var zeros = 0; while (zeros < bytes.length && bytes[zeros] === 0) zeros++;
    var digits = [0];
    for (var i = zeros; i < bytes.length; i++) {
      var carry = bytes[i];
      for (var j = 0; j < digits.length; j++) { carry += digits[j] << 8; digits[j] = carry % 58; carry = (carry / 58) | 0; }
      while (carry) { digits.push(carry % 58); carry = (carry / 58) | 0; }
    }
    var s = '';
    for (var z = 0; z < zeros; z++) s += '1';
    for (var q = digits.length - 1; q >= 0; q--) s += B58[digits[q]];
    return s;
  }

  // script hash -> N-address. Input contract: the 20 bytes in the order contract
  // STORAGE KEYS hold them (e.g. findstorage results after the prefix byte) — the
  // same order the address itself embeds. If you hold the canonical RPC "0x…" form,
  // reverseHex it first. Uses the pure-JS sha256 above, NOT crypto.subtle: rendering
  // must work on plain-http mirrors (own node, local IPFS gateway) where subtle
  // is absent.
  function scriptHashToAddress(storageHex) {
    var payload = new Uint8Array(21);
    payload[0] = N3_ADDRESS_VERSION;
    payload.set(hexToBytes(String(storageHex).replace(/^0x/, '')), 1);
    var sum = sha256(sha256(payload));
    var full = new Uint8Array(25);
    full.set(payload); full.set(sum.slice(0, 4), 21);
    return base58Encode(full);
  }

  // ---------------- typed dApi arguments ----------------
  var A = {
    h160: function (v) { // accepts an N-address or a 0x scripthash; ALWAYS emits canonical
      // Hash160 (little-endian StringLE hex). NeoLine's invoke validation rejects the
      // 'Address' arg type ("Please check your input"), and raw RPC interprets Hash160
      // as StringLE — the address-stored byte order reads a phantom account (FTB: found
      // 18 Oct via balanceOf returning 0 for a funded wallet).
      if (typeof v === 'string' && v[0] === 'N') return { type: 'Hash160', value: '0x' + addressToScriptHashHex(v, { le: true }) };
      return { type: 'Hash160', value: String(v).indexOf('0x') === 0 ? v : '0x' + v };
    },
    int: function (v) { return { type: 'Integer', value: String(v) }; },
    str: function (v) { return { type: 'String', value: String(v) }; },
    bool: function (v) { return { type: 'Boolean', value: !!v }; },
    bytesHex: function (hex) { return { type: 'ByteArray', value: hexToB64(hex) }; }, // dApi ByteArray = base64
    arr: function (items) { return { type: 'Array', value: items }; },
    any: function (v) { return { type: 'Any', value: v === undefined ? null : v }; }
  };

  // ---------------- read-result decoding ----------------
  function first(r) { return (r && r.stack && r.stack.length) ? r.stack[0] : null; }
  function decodeInt(r) { var it = first(r); if (!it || it.value == null) return 0n; return BigInt(it.value); }
  function decodeBool(r) {
    var it = first(r); if (!it) return false;
    if (it.type === 'Boolean') return !!it.value;
    if (it.value == null || it.value === '') return false;
    try { return BigInt(it.value) !== 0n; } catch (e) { return !!it.value; }
  }
  function decodeHash160(r) { // VM returns a 20-byte LE bytestring; present as 0x big-endian
    var it = first(r); if (!it || it.value == null || it.value === '') return null;
    var hex = (it.type === 'ByteString' || it.type === 'ByteArray') ? b64ToHex(it.value) : String(it.value).replace(/^0x/, '');
    return '0x' + reverseHex(hex);
  }

  // ---------------- detection ----------------
  function detectLegacy(timeoutMs) {
    return new Promise(function (resolve, reject) {
      function make() { try { return new global.NEOLineN3.Init(); } catch (e) { return null; } }
      if (global.NEOLineN3) { var n = make(); if (n) return resolve(n); }
      var settled = false;
      function on() {
        if (settled) return; settled = true;
        var n = make();
        n ? resolve(n) : reject(new Error('NeoLine N3 dApi failed to initialize'));
      }
      global.addEventListener('NEOLine.N3.EVENT.READY', on, { once: true });
      setTimeout(function () {
        if (settled) return; settled = true;
        global.removeEventListener('NEOLine.N3.EVENT.READY', on);
        var n = global.NEOLineN3 ? make() : null;
        n ? resolve(n) : reject(new Error('No Neo N3 dApi wallet detected. Install NeoLine (or another dApi wallet) and reload.'));
      }, timeoutMs || 3000);
    });
  }
  function detectNep21(timeoutMs) { // informational only in this version
    return new Promise(function (resolve) {
      var found = null;
      function on(e) { if (e && e.detail && e.detail.provider && e.detail.provider.name === 'NeoLine') { found = e.detail.provider; } }
      global.addEventListener('Neo.DapiProvider.ready', on);
      try { global.dispatchEvent(new Event('Neo.DapiProvider.request')); } catch (e) {}
      setTimeout(function () { global.removeEventListener('Neo.DapiProvider.ready', on); resolve(found); }, timeoutMs || 1500);
    });
  }

  // ---------------- the adapter ----------------
  function FtbDapi(cfg) {
    cfg = cfg || {};
    this.cfg = {
      network: cfg.network || 'TestNet',
      timeoutMs: cfg.timeoutMs || 3000,
      contracts: cfg.contracts || {}, // ftb, migrator, registry, oracle, lend, ballot, bneo, router
      assets: Object.assign({ gas: GAS }, cfg.assets || {}),
      vote: cfg.vote || {} // { ftb: '<address>', wait: '<address>' }
    };
    this.n3 = null;
    this.account = null;
  }

  FtbDapi.prototype.init = function () {
    var self = this;
    return detectLegacy(this.cfg.timeoutMs).then(function (n3) { self.n3 = n3; return self; });
  };
  FtbDapi.prototype.hasNep21 = function () { return detectNep21(this.cfg.timeoutMs); };
  FtbDapi.prototype._need = function (name) {
    var h = this.cfg.contracts[name];
    if (!h) throw new Error('config.contracts.' + name + ' is not set');
    return h;
  };
  FtbDapi.prototype.connect = function () {
    var self = this;
    return this.n3.getAccount().then(function (a) { self.account = a.address; return a; });
  };
  FtbDapi.prototype.networks = function () { return this.n3.getNetworks(); };

  // low-level read (no signer: FTB reads never check a witness)
  FtbDapi.prototype.read = function (scriptHash, operation, args) {
    return this.n3.invokeRead({ scriptHash: scriptHash, operation: operation, args: args || [], signers: [] });
  };
  // low-level write: takes a plan, signs it. Optional plan.signers / plan.fee.
  // NeoLine's invoke REQUIRES signers to be present (dapi-layer check: missing or
  // non-array -> MALFORMED_INPUT "Please check your input" before any popup).
  // Default: the connected account with CalledByEntry — the least privilege that
  // covers every FTB write (vote, claim, supply, repay, withdraw, stake).
  FtbDapi.prototype.defaultSigner = function () {
    if (!this.account) throw new Error('connect first: no account to sign with');
    return { account: addressToScriptHashHex(this.account, { le: true }), scopes: 1 }; // 1 = CalledByEntry
  };
  FtbDapi.prototype.execute = function (plan) {
    if (!plan || plan.kind !== 'write') throw new Error('execute expects a write plan');
    var req = { scriptHash: plan.scriptHash, operation: plan.operation, args: plan.args };
    req.signers = plan.signers || [this.defaultSigner()];
    if (plan.fee) req.fee = plan.fee;
    return this.n3.invoke(req);
  };
  FtbDapi.prototype._w = function (scriptHash, operation, args, summary, extra) {
    return Object.assign({ kind: 'write', scriptHash: scriptHash, operation: operation, args: args, summary: summary }, extra || {});
  };

  // ---- reads ----
  FtbDapi.prototype.balanceOf = function (acct) { return this.read(this._need('ftb'), 'balanceOf', [A.h160(acct)]).then(decodeInt); };
  FtbDapi.prototype.totalSupply = function () { return this.read(this._need('ftb'), 'totalSupply', []).then(decodeInt); };
  FtbDapi.prototype.maxSupply = function () { return this.read(this._need('ftb'), 'maxSupply', []).then(decodeInt); };
  FtbDapi.prototype.migrationOpen = function () { return this.read(this._need('ftb'), 'migrationOpen', []).then(decodeBool); };
  FtbDapi.prototype.claimed = function (acct) { return this.read(this._need('migrator'), 'claimed', [A.h160(acct)]).then(decodeBool); };
  FtbDapi.prototype.registryGet = function (name) { return this.read(this._need('registry'), 'get', [A.str(name)]).then(decodeHash160); };
  FtbDapi.prototype.price = function (asset) { return this.read(this._need('oracle'), 'price', [A.h160(asset)]).then(decodeInt); };
  FtbDapi.prototype.debtOf = function (user) { return this.read(this._need('lend'), 'debtOf', [A.h160(user)]).then(decodeInt); };
  FtbDapi.prototype.collateralValue = function (user) { return this.read(this._need('lend'), 'collateralValue', [A.h160(user)]).then(decodeInt); };
  FtbDapi.prototype.maxBorrow = function (user) { return this.read(this._need('lend'), 'maxBorrow', [A.h160(user)]).then(decodeInt); };
  FtbDapi.prototype.isUnhealthy = function (user) { return this.read(this._need('lend'), 'isUnhealthy', [A.h160(user)]).then(decodeBool); };
  FtbDapi.prototype.sharesOf = function (user) { return this.read(this._need('lend'), 'sharesOf', [A.h160(user)]).then(decodeInt); };
  // ballot reads: tallies and one-vote markers are public chain state, same for every node
  FtbDapi.prototype.ballotRoot = function () { return this.read(this._need('ballot'), 'root', []); };
  FtbDapi.prototype.ballotNumOptions = function () { return this.read(this._need('ballot'), 'numOptions', []).then(decodeInt); };
  FtbDapi.prototype.ballotTally = function (option) { return this.read(this._need('ballot'), 'tally', [A.int(option)]).then(decodeInt); };
  FtbDapi.prototype.ballotVoted = function (acct) { return this.read(this._need('ballot'), 'voted', [A.h160(acct)]).then(decodeBool); };
  // proposals board reads (registry v2): the board is replicated chain state — no one hosts it
  FtbDapi.prototype.proposalCount = function () { return this.read(this._need('registry'), 'proposalCount', []).then(decodeInt); };
  FtbDapi.prototype.proposalBond = function () { return this.read(this._need('registry'), 'proposalBond', []).then(decodeInt); };
  FtbDapi.prototype.boardEscrow = function () { return this.read(this._need('registry'), 'escrow', []).then(decodeInt); };
  FtbDapi.prototype.proposal = function (id) { return this.read(this._need('registry'), 'proposal', [A.int(id)]); };

  // ---- write plan builders (PURE: build, inspect .summary, then execute()) ----
  FtbDapi.prototype.claim = function (account, amountBase, proofHexArray) {
    var proof = A.arr((proofHexArray || []).map(function (h) { return A.bytesHex(h); }));
    return this._w(this._need('migrator'), 'claim', [A.h160(account), A.int(amountBase), proof],
      'Claim ' + fromBaseUnits(amountBase) + ' FLM-equiv as ' + fromBaseUnits(ftbOf(amountBase)) + ' FTB (r = 0.189832) for ' + account);
  };
  FtbDapi.prototype.supply = function (user, amountBase) {
    return this._w(this._need('ftb'), 'transfer',
      [A.h160(user), A.h160(this._need('lend')), A.int(amountBase), A.arr([A.str('supply')])],
      'Supply ' + fromBaseUnits(amountBase) + ' FTB to the lend pool');
  };
  FtbDapi.prototype.repay = function (user, amountBase) {
    return this._w(this._need('ftb'), 'transfer',
      [A.h160(user), A.h160(this._need('lend')), A.int(amountBase), A.arr([A.str('repay')])],
      'Repay ' + fromBaseUnits(amountBase) + ' FTB of debt');
  };
  // NEO collateral is whole units: 0 decimals, no base-unit scaling.
  FtbDapi.prototype.addCollateralNeo = function (user, wholeNeo) {
    return this._w(NEO, 'transfer',
      [A.h160(user), A.h160(this._need('lend')), A.int(wholeNeo), A.any(null)],
      'Add ' + wholeNeo + ' NEO as collateral');
  };
  FtbDapi.prototype.addCollateralGas = function (user, amountBase) {
    return this._w(this.cfg.assets.gas, 'transfer',
      [A.h160(user), A.h160(this._need('lend')), A.int(amountBase), A.any(null)],
      'Add ' + fromBaseUnits(amountBase) + ' GAS as collateral');
  };
  FtbDapi.prototype.borrow = function (user, amountBase) {
    return this._w(this._need('lend'), 'borrow', [A.h160(user), A.int(amountBase)],
      'Borrow ' + fromBaseUnits(amountBase) + ' FTB');
  };
  FtbDapi.prototype.withdrawSupply = function (user, shares) {
    return this._w(this._need('lend'), 'withdrawSupply', [A.h160(user), A.int(shares)],
      'Withdraw ' + shares + ' supply shares');
  };
  FtbDapi.prototype.withdrawCollateral = function (user, neoWhole, gasBase) {
    return this._w(this._need('lend'), 'withdrawCollateral', [A.h160(user), A.int(neoWhole), A.int(gasBase)],
      'Withdraw collateral: ' + neoWhole + ' NEO, ' + fromBaseUnits(gasBase) + ' GAS');
  };
  FtbDapi.prototype.liquidate = function (liquidator, repayBase, borrower) {
    return this._w(this._need('ftb'), 'transfer',
      [A.h160(liquidator), A.h160(this._need('lend')), A.int(repayBase), A.arr([A.str('liq'), A.h160(borrower)])],
      'Liquidate ' + borrower + ', repaying up to ' + fromBaseUnits(repayBase) + ' FTB');
  };
  // dividends module: stake FTB to earn the holders' 2/9; claim and unstake any time
  FtbDapi.prototype.stakeFtb = function (user, amountBase) {
    return this._w(this._need('ftb'), 'transfer',
      [A.h160(user), A.h160(this._need('dividends')), A.int(amountBase), A.arr([A.str('stake')])],
      'Stake ' + fromBaseUnits(amountBase) + ' FTB for dividends');
  };
  FtbDapi.prototype.claimDividends = function (user) {
    return this._w(this._need('dividends'), 'claim', [A.h160(user)],
      'Claim earned dividends');
  };
  FtbDapi.prototype.unstakeFtb = function (user, amountBase) {
    return this._w(this._need('dividends'), 'unstake', [A.h160(user), A.int(amountBase)],
      'Unstake ' + fromBaseUnits(amountBase) + ' FTB (earned dividends stay claimable)');
  };
  FtbDapi.prototype.vote = function (voter, choice) {
    var to = this.cfg.vote[choice];
    if (!to) throw new Error('vote address for "' + choice + '" is not set in config.vote');
    return this._w(this.cfg.assets.gas, 'transfer', [A.h160(voter), A.h160(to), A.int('1'), A.any(null)],
      'Vote ' + String(choice).toUpperCase() + ': dust 0.00000001 GAS to ' + to);
  };
  // On-chain weighted vote (preferred over the dust-signal vote above): proves snapshot weight
  // with the SAME migration Merkle proof, one vote per snapshot address. proofHexArray is the
  // array of hex node hashes from proofs.json for `account`; amountBase is its snapshot balance
  // in FLM-equivalent base units — the genesis vote weighs exactly 1 FLM = 1 vote (DECISIONS.md
  // 18 Oct). The ballot contract is deployed per proposal; its hash IS the proposal id.
  FtbDapi.prototype.ballotVote = function (account, amountBase, option, proofHexArray) {
    var proof = A.arr((proofHexArray || []).map(function (h) { return A.bytesHex(h); }));
    return this._w(this._need('ballot'), 'vote',
      [A.h160(account), A.int(amountBase), A.int(option), proof],
      'Vote option #' + option + ' with weight ' + fromBaseUnits(amountBase) + ' FLM (1 FLM = 1 vote) for ' + account);
  };
  // Submit a proposal to the board (registry v2): the exact bond IS the transfer amount and
  // the proposal rides as the payment data [kind, ref, meta]. The registry escrows the bond;
  // a legitimate close refunds it, spam forfeits it. kind: "ballot" (ref = ballot contract
  // hash) or "listing" (ref = the token to list, meta carries the bond terms).
  FtbDapi.prototype.submitProposal = function (account, kind, refHash, meta, bondBase) {
    if (kind !== 'ballot' && kind !== 'listing') throw new Error('kind must be ballot or listing');
    return this._w(this._need('ftb'), 'transfer',
      [A.h160(account), A.h160(this._need('registry')), A.int(bondBase),
       A.arr([A.str(kind), A.h160(refHash), A.str(meta)])],
      'Submit ' + kind + ' proposal (bond ' + fromBaseUnits(bondBase) + ' FTB): ' + String(meta).slice(0, 60));
  };
  // Swap uses the EXTERNAL Flamingo router. Confirm the method name, arg order, and signer
  // scope against the deployed manifest; multi-hop usually needs a CustomContracts signer
  // covering the router, the tokens in the path, and the pair contracts.
  FtbDapi.prototype.swap = function (sender, amountInBase, minOutBase, pathHashes, deadlineMs, signer) {
    var plan = this._w(this._need('router'), 'swapTokenInForTokenOut',
      [A.h160(sender), A.int(amountInBase), A.int(minOutBase),
       A.arr((pathHashes || []).map(function (h) { return A.h160(h); })), A.int(deadlineMs)],
      'Swap ' + fromBaseUnits(amountInBase) + ' in, min ' + fromBaseUnits(minOutBase) + ' out, path ' + (pathHashes || []).join(' -> '));
    if (signer) plan.signers = [signer];
    plan.note = 'External Flamingo router: verify method, arg order, and signer scope against the deployed manifest before use.';
    return plan;
  };

  // build an explicit CustomContracts signer for swap, from the connected address
  FtbDapi.prototype.signerFor = function (address, allowedContracts, opts) {
    // NeoLine expects the signer account as the little-endian script hash hex (verified). Pass
    // {be:true} only if a particular wallet rejects it and wants big-endian.
    var le = !(opts && opts.be);
    return {
      account: addressToScriptHashHex(address, { le: le }),
      scopes: WITNESS.CustomContracts,
      allowedContracts: allowedContracts || []
    };
  };

  // ---- events (unsubscribe with the returned function) ----
  FtbDapi.prototype.on = function (event, handler) {
    var map = {
      accountChanged: 'NEOLine.N3.EVENT.ACCOUNT_CHANGED',
      networkChanged: 'NEOLine.N3.EVENT.NETWORK_CHANGED',
      connected: 'NEOLine.N3.EVENT.CONNECTED',
      disconnected: 'NEOLine.N3.EVENT.DISCONNECTED',
      blockHeight: 'NEOLine.N3.EVENT.BLOCK_HEIGHT_CHANGED',
      txConfirmed: 'NEOLine.N3.EVENT.TRANSACTION_CONFIRMED'
    };
    var name = map[event] || event;
    var wrapped = function (e) { handler(e && e.detail); };
    global.addEventListener(name, wrapped);
    return function () { global.removeEventListener(name, wrapped); };
  };

  // exact mirror of migrator.quote() on-chain (DECISIONS.md 18 Oct): the committed
  // conversion r = 100,000,000 FTB ÷ 526,782,179.10 FLM-equiv = 0.189832, computed in
  // integers with floor semantics — floor(amount × 10^16 / 52,678,217,910,000,000).
  var RATIO_NUM = 10000000000000000n, RATIO_DEN = 52678217910000000n;
  function ftbOf(flmBase) { return (BigInt(flmBase) * RATIO_NUM / RATIO_DEN).toString(); }

  // expose helpers for pages and tests
  FtbDapi.MAGIC = MAGIC;
  FtbDapi.WITNESS = WITNESS;
  FtbDapi.GAS = GAS;
  FtbDapi.args = A;
  FtbDapi.util = {
    toBaseUnits: toBaseUnits, fromBaseUnits: fromBaseUnits,
    hexToB64: hexToB64, b64ToHex: b64ToHex, reverseHex: reverseHex,
    addressToScriptHashHex: addressToScriptHashHex,
    scriptHashToAddress: scriptHashToAddress,
    decodeInt: decodeInt, decodeBool: decodeBool, decodeHash160: decodeHash160,
    ftbOf: ftbOf
  };

  global.FtbDapi = FtbDapi;
  if (typeof module !== 'undefined' && module.exports) module.exports = FtbDapi;
})(typeof window !== 'undefined' ? window : this);
