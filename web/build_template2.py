#!/usr/bin/env python3
"""Build template2.html from scratch: new layout, reusing template 1's inlined font,
component CSS, modals, and signing script (same element ids, same flows).

Run from web/:  python3 build_template2.py
"""
import re

base = open('template1.html').read()

# --- reusable parts from template 1 ---
font = re.search(r'@font-face\{.*?\n\}', base, re.S).group(0)
style_all = re.search(r'<style>(.*?)</style>', base, re.S).group(1)
modals = base[base.index('<!-- ================= modal: claim'):base.index('<script src="config.js">')].rstrip()
main_script = re.search(r'<script>\n(\(function\(\)\{[\s\S]*?\)\(\);)\s*</script>', base).group(1)

T2_CSS = """
/* ============================================================
   TEMPLATE 2 — station dashboard, designed from scratch against
   UI_RATIONALE.md. Left rail navigation (NEAR: plain words), a
   governance hero panel (Terra: the vote is the front page),
   stat tiles and an always-on ticker (Hyperliquid), and a
   ticket-first swap column (THORSwap). Strict grayscale, VT323,
   zero dependencies.
   ============================================================ */
body{font-size:15px}

/* ---- left rail ---- */
.rail{position:fixed; left:0; top:0; bottom:0; width:200px; z-index:50;
  background:var(--paper); border-right:1px solid var(--ink);
  display:flex; flex-direction:column; padding:18px 16px; gap:0}
.rail .bird2{border:0; align-self:flex-start; line-height:0; color:var(--ink)}
.rail .bird2:hover{background:transparent}
.rail .bird2 svg{height:72px; width:auto; display:block}
.rail .brand2{font-family:var(--disp); font-size:36px; letter-spacing:.06em;
  border:0; align-self:flex-start; margin-top:4px}
.rail .brand2:hover{background:transparent; color:var(--ink)}
.rail .date2{font-size:11px; color:var(--faint); letter-spacing:.1em; margin-top:2px}
.rail nav{display:flex; flex-direction:column; gap:2px; margin-top:30px}
.rail nav a{border:0; padding:7px 9px; font-size:12.5px; letter-spacing:.16em;
  text-transform:uppercase; color:var(--ink-2)}
.rail nav a:hover{background:var(--ink); color:var(--paper)}
.railfoot{margin-top:auto; display:flex; flex-direction:column; gap:9px;
  font-size:12px; align-items:flex-start}
.railfoot .idstate{font-size:11.5px}

/* ---- content frame ---- */
.wrap{margin-left:200px; padding:22px 26px 90px; max-width:1080px}

/* ---- governance hero (the vote IS the front page) ---- */
.hero2{background:var(--ink); color:var(--paper); padding:20px 24px 18px}
.hero2-h{display:flex; justify-content:space-between; align-items:baseline; gap:1rem;
  font-size:11px; letter-spacing:.16em; text-transform:uppercase; opacity:.75; flex-wrap:wrap}
.hero2 h1{font-family:var(--disp); font-weight:400; font-size:clamp(30px,5.4vw,52px);
  line-height:1.02; margin:.35em 0 .3em; letter-spacing:.02em}
.hero2-hint{font-size:12.5px; opacity:.8; max-width:64ch; margin:0 0 4px}
.hero2 .gopt{border-color:var(--paper); background:transparent}
.hero2 .gbar{border-color:var(--paper)}
.hero2 .gbar i{background:var(--paper)}
.hero2 .gopt .gmeta{color:var(--paper); opacity:.7}
.hero2 .gopt button{color:var(--paper); border-color:var(--paper)}
.hero2 .gopt button:hover{background:var(--paper); color:var(--ink)}
.hero2 .linkish{color:var(--paper); border-bottom-color:var(--paper)}
.hero2 .linkish:hover{background:var(--paper); color:var(--ink)}

/* ---- stat tiles ---- */
.tiles{display:grid; grid-template-columns:repeat(4,1fr); gap:10px; margin-top:14px}
.tile{border:1px solid var(--ink); padding:10px 12px}
.tile .tl{font-size:10.5px; letter-spacing:.14em; text-transform:uppercase; color:var(--faint)}
.tile .tn{font-family:var(--disp); font-size:30px; line-height:1.15; word-break:break-all}

/* ---- two working columns: market left, swap + wallet right ---- */
.cols{display:grid; grid-template-columns:1fr 1fr; gap:30px; margin-top:26px; align-items:start}
.col-h{border-top:3px double var(--ink); border-bottom:3px double var(--ink);
  padding:3px 0; margin-bottom:4px; display:flex; justify-content:space-between;
  align-items:baseline; gap:.6rem; flex-wrap:wrap}
.col-h .no{font-family:var(--disp); font-size:22px; letter-spacing:.1em}
.col-h .lede{font-size:11px; letter-spacing:.16em; text-transform:uppercase; color:var(--faint)}

/* ---- claim + verify ---- */
.act2{margin-top:34px}

/* ---- footer ---- */
.foot2{margin-top:44px; padding-top:16px; border-top:3px double var(--ink);
  font-size:12px; color:var(--ink-2); line-height:1.9; max-width:70ch}

@media (max-width:760px){
  .rail{position:static; width:auto; border-right:0; border-bottom:1px solid var(--ink);
    flex-direction:row; flex-wrap:wrap; align-items:center; gap:10px; padding:10px 14px}
  .rail .bird2 svg{height:36px}
  .rail nav{flex-direction:row; flex-wrap:wrap; margin-top:0; gap:0}
  .railfoot{margin-top:0; flex-direction:row; flex-wrap:wrap; align-items:center}
  .wrap{margin-left:0; padding:16px 16px 70px}
  .tiles{grid-template-columns:1fr 1fr}
  .cols{grid-template-columns:1fr}
}
"""

MARKUP = """<aside class="rail">
  <a class="bird2" href="#top" aria-label="Free the Bird">
    <svg viewBox="0 0 220 130" fill="none" stroke="currentColor" stroke-width="9"
         stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
      <path d="M14 84 C 46 30, 82 26, 110 62 C 138 26, 174 30, 206 84"/>
      <circle cx="110" cy="66" r="7"/>
      <line x1="110" y1="73" x2="110" y2="112"/>
    </svg>
  </a>
  <a class="brand2" href="#top">FTB</a>
  <div class="date2" id="dateline">// CONNECTING //</div>
  <nav aria-label="sections">
    <a href="#vote">vote</a>
    <a href="#market">market</a>
    <a href="#swap">swap</a>
    <a href="#claim">claim</a>
    <a href="#verify">verify</a>
  </nav>
  <div class="railfoot">
    <span class="idstate" id="idstate">[ no wallet ]</span>
    <button id="connectBtn">connect -&gt;</button>
    <button class="linkish" id="themeBtn">theme</button>
    <span class="muted" id="rpcNote">rpc: connecting...</span>
    <button class="linkish" id="ownRpcBtn">own node -&gt;</button>
  </div>
</aside>

<div class="wrap" id="top">

  <!-- ============ GOVERNANCE HERO ============ -->
  <section class="hero2" id="vote">
    <div class="hero2-h">
      <span>// governance — snapshot-weighted, counted on-chain</span>
      <button class="linkish" id="govRefresh">tally -&gt;</button>
    </div>
    <h1 id="govQ">no live ballot</h1>
    <p class="hero2-hint" id="govHint">one vote per snapshot address — weight is your snapshot
      balance, your proof is the same one you claim with. every node counts the same chain state.</p>
    <div id="govBody"></div>
  </section>

  <!-- ============ STAT TILES ============ -->
  <div class="tiles">
    <div class="tile"><div class="tl">ftb supply</div><div class="tn" id="tSupply">—</div></div>
    <div class="tile"><div class="tl">hard cap</div><div class="tn" id="tCap">—</div></div>
    <div class="tile"><div class="tl">weight voted</div><div class="tn" id="tVoted">—</div></div>
    <div class="tile"><div class="tl">block</div><div class="tn" id="tBlock">—</div></div>
  </div>

  <div class="ticker" id="ticker" style="margin-top:14px">// market: waiting for the pools</div>

  <!-- ============ MARKET / SWAP COLUMNS ============ -->
  <div class="cols">
    <div>
      <div class="mkt" id="market">
        <div class="col-h"><span class="no">MARKET</span>
          <span class="lede">asset ratios — no fiat, no oracle</span>
          <button class="linkish" id="ratioBtn">refresh -&gt;</button></div>
        <div class="rgrid" id="ratioBody" style="grid-template-columns:1fr">
          <div class="rcard">
            <div class="rp">standby</div>
            <div class="rnum">—</div>
            <div class="rinv" id="ratioHint">loads when the router and pairs are configured for this network</div>
          </div>
        </div>
      </div>
    </div>
    <div>
      <div class="bal" id="balStrip" hidden>
        <span>wallet:</span>
        <span>FTB <b id="balFtb">—</b></span>
        <span>bNEO <b id="balBneo">—</b></span>
        <span>GAS <b id="balGas">—</b></span>
        <span>claimable <b id="balClaim">—</b></span>
      </div>
      <div id="swap">
        <div class="col-h"><span class="no">SWAP</span>
          <span class="lede">reused audited flamingo router</span></div>
        <div class="ticket">
          <div class="trow">
            <label for="swAmt">you pay</label>
            <input id="swAmt" inputmode="decimal" autocomplete="off" placeholder="0.0">
            <div class="tsel" id="swFromSel" role="group" aria-label="pay asset"></div>
          </div>
          <div class="trow">
            <label for="swOut">you receive</label>
            <input id="swOut" readonly placeholder="—" tabindex="-1">
            <div class="tsel" id="swToSel" role="group" aria-label="receive asset"></div>
          </div>
          <div class="tmeta" id="swMeta">quote from live pool reserves · 0.3% pool fee · min received at 0.5% slippage</div>
          <button class="go" id="swGo">review &amp; sign -&gt;</button>
        </div>
        <p class="small muted" id="swapHint">loads when the router hash is configured for this network.</p>
      </div>
    </div>
  </div>

  <!-- ============ CLAIM ============ -->
  <section class="act2" id="claim">
    <div class="col-h"><span class="no">CLAIM</span>
      <span class="lede">migration — claim your FTB</span></div>
    <div class="entry">
      <span class="idx">01</span>
      <div><span class="ttl">Migration status</span></div>
      <button class="read linkish" id="refreshBtn">refresh -&gt;</button>
      <div class="meta">
        <span class="tag pub" id="migState">reading</span>
        <span id="supplyLine">supply —</span>
        <span id="capLine">cap —</span>
      </div>
    </div>
    <div class="entry">
      <span class="idx">02</span>
      <div><span class="ttl">Check your claim against the pre-exploit snapshot</span></div>
      <button class="read linkish" id="checkBtn">check -&gt;</button>
      <div class="meta"><span id="claimHint">connect a wallet, we look up your address in the published proofs</span></div>
    </div>
    <div class="entry">
      <span class="idx">03</span>
      <div><span class="ttl">The snapshot is the fairness. The attacker is not in it.</span></div>
      <button class="read linkish" id="proofsBtn">proofs -&gt;</button>
      <div class="meta"><span class="tag">merkle</span><span id="rootLine">root —</span><span>4,847 holders</span></div>
    </div>
  </section>

  <!-- ============ VERIFY ============ -->
  <section class="act2" id="verify">
    <div class="col-h"><span class="no">VERIFY</span>
      <span class="lede">verify everything — trust nothing</span></div>
    <dl class="prov">
      <dt>token</dt><dd id="vToken">—</dd>
      <dt>migrator</dt><dd id="vMigrator">—</dd>
      <dt>registry</dt><dd id="vRegistry">—</dd>
      <dt>ballot</dt><dd id="vBallot">—</dd>
      <dt>merkle root</dt><dd id="vRoot">—</dd>
      <dt>max supply</dt><dd id="vCap">—</dd>
      <dt>mint sealed</dt><dd id="vSealed">—</dd>
      <dt>snapshot</dt><dd>block 12,782,704 — reproducible with snapshot_state.py against any archive node</dd>
    </dl>
    <p class="sealnote">
      These hashes are the whole ballgame. A hostile mirror of this page would change them —
      so check them against the published repo and the deploy output before you sign anything.
      After close + renounceOwnership, the token has no admin at all; that fact is readable
      on-chain, forever.
    </p>
  </section>

  <footer class="foot2">
    <span class="disp" style="font-size:20px">free the bird</span><br>
    Reference implementation pending external audit. Nothing here is financial, legal, or
    investment advice. A gift to the Flamingo community: the code is MIT, the proofs are public,
    the mint is sealed by mathematics. Verify, then sign.
  </footer>
</div>

"""

TILES_SCRIPT = """
// ---------- stat tiles (template 2; independent small reader, same RPC list) ----------
(function(){
  var CFG=window.FTB_CONFIG||{}, C=CFG.contracts||{};
  var NET=CFG.network||'TestNet';
  var RPCS=(CFG.rpcs&&CFG.rpcs.length?CFG.rpcs.slice():[]).concat(NET==='MainNet'
    ? ['https://mainnet1.neo.coz.io:443','https://mainnet2.neo.coz.io:443']
    : ['https://testnet1.neo.coz.io:443','https://testnet2.neo.coz.io:443']);
  try{ var own=localStorage.getItem('ftb_rpc'); if(own) RPCS.unshift(own); }catch(e){}
  function rpc(url,m,p){ return fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({jsonrpc:'2.0',id:1,method:m,params:p||[]})}).then(function(r){return r.json()})
    .then(function(j){ if(j.error) throw new Error(j.error.message||'rpc'); return j.result; }); }
  function pick(i){ i=i||0; if(i>=RPCS.length) return Promise.reject(new Error('no rpc'));
    return rpc(RPCS[i],'getblockcount').then(function(h){ return h>0?RPCS[i]:pick(i+1); }).catch(function(){ return pick(i+1); }); }
  function readInt(url,h,op,args){ return rpc(url,'invokefunction',[h,op,args||[]]).then(function(r){
    if(r.state!=='HALT') throw new Error('fault'); var it=r.stack&&r.stack[0];
    return (it&&it.value!=null)?BigInt(it.value):0n; }); }
  function fmt(b){ return FtbDapi.util.fromBaseUnits(b.toString()); }
  function set(id,v){ var e=document.getElementById(id); if(e) e.textContent=v; }
  pick().then(function(url){
    rpc(url,'getblockcount').then(function(h){ set('tBlock', Number(h).toLocaleString('en-US')); }).catch(function(){});
    if (C.ftb && C.ftb.indexOf('0x0000')!==0){
      readInt(url,C.ftb,'totalSupply').then(function(v){ set('tSupply', fmt(v)); }).catch(function(){});
      readInt(url,C.ftb,'maxSupply').then(function(v){ set('tCap', fmt(v)); }).catch(function(){});
    }
    if (C.ballot && C.ballot.indexOf('0x0000')!==0 && CFG.ballot && CFG.ballot.options && CFG.ballot.options.length){
      Promise.all(CFG.ballot.options.map(function(_,i){
        return readInt(url,C.ballot,'tally',[{type:'Integer',value:String(i)}]);
      })).then(function(vals){
        var t=vals.reduce(function(s,v){return s+v;},0n);
        set('tVoted', fmt(t)+' FTB');
      }).catch(function(){});
    }
  }).catch(function(){});
})();
"""

out = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>free the bird — template 2 (station dashboard)</title>
<style>
""" + font + "\n" + style_all + "\n" + T2_CSS + """
</style>
</head>
<body>
""" + MARKUP + modals + """

<script src="config.js"></script>
<script src="ftb-dapi.js"></script>
<script>
""" + main_script + """
</script>
<script>""" + TILES_SCRIPT + """
</script>
</body>
</html>
"""

open('template2.html', 'w').write(out)
print('wrote template2.html', len(out), 'bytes')
