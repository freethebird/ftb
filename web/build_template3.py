#!/usr/bin/env python3
"""Build template3.html — the Flamingo blend.

Same engine as template 2 (font, component CSS, modals, signing script, tiles
script — all shared through build_template2's imports, zero behavior drift),
new skin: flamingo.finance's information architecture — top tab bar, marketing
hero, swap card with tabs, tokens table, FLOCKS-style big-APY dividend card —
poured back into our grayscale typewriter discipline.

Run from web/:  python3 build_template3.py
"""
from build_template2 import font, style_all, modals, main_script, TILES_SCRIPT

T3_CSS = """
/* ============================================================
   TEMPLATE 3 — the Flamingo blend. flamingo.finance's familiar
   shell (top tabs, hero, swap card, tokens table, FLOCKS-style
   APY card) re-expressed in strict grayscale VT323. Same ids,
   same flows, same invariants as templates 1 and 2.
   ============================================================ */
body{font-size:15px}

/* ---- top tab bar (Flamingo app chrome, flattened) ---- */
.topbar{position:sticky; top:0; z-index:50; background:var(--paper);
  border-bottom:1px solid var(--ink); display:flex; align-items:center;
  gap:18px; padding:10px 20px; flex-wrap:wrap}
.topbar .bird3{line-height:0; color:var(--ink); border:0}
.topbar .bird3:hover{background:transparent}
.topbar .bird3 svg{height:34px; width:auto; display:block}
.topbar .wordmark{font-family:var(--disp); font-size:26px; letter-spacing:.08em;
  border:0}
.topbar .wordmark:hover{background:transparent; color:var(--ink)}
.tabs{display:flex; gap:2px; flex-wrap:wrap; margin-left:8px}
.tabs a{border:1px solid transparent; padding:5px 11px; font-size:12px;
  letter-spacing:.14em; text-transform:uppercase; color:var(--ink-2)}
.tabs a:hover{border-color:var(--ink); background:transparent; color:var(--ink)}
.topright{margin-left:auto; display:flex; align-items:center; gap:10px;
  font-size:12px; flex-wrap:wrap}
.topright .idstate{font-size:11.5px}

/* ---- hero (Flamingo landing: one sentence + one button) ---- */
.hero3{max-width:1060px; margin:0 auto; padding:64px 22px 40px; text-align:center}
.hero3 h1{font-family:var(--disp); font-weight:400;
  font-size:clamp(34px,6.4vw,64px); line-height:1.03; letter-spacing:.02em; margin:0 0 14px}
.hero3 .sub{font-size:13.5px; color:var(--ink-2); max-width:62ch; margin:0 auto 22px;
  line-height:1.7}
.cta-row{display:flex; gap:10px; justify-content:center; flex-wrap:wrap}
.cta{display:inline-block; border:1px solid var(--ink); padding:10px 22px;
  font-size:12.5px; letter-spacing:.16em; text-transform:uppercase}
.cta.solid{background:var(--ink); color:var(--paper)}
.cta:not(.solid):hover{background:var(--ink); color:var(--paper)}
.cta.solid:hover{opacity:.85}

/* ---- content frame ---- */
.page{max-width:1060px; margin:0 auto; padding:0 22px 90px}

/* ---- stat tiles (Flamingo analytics band) ---- */
.tiles{display:grid; grid-template-columns:repeat(4,1fr); gap:10px; margin-top:8px}
.tile{border:1px solid var(--ink); padding:10px 12px}
.tile .tl{font-size:10.5px; letter-spacing:.14em; text-transform:uppercase; color:var(--faint)}
.tile .tn{font-family:var(--disp); font-size:28px; line-height:1.15; word-break:break-all}

/* ---- cards (the Flamingo panel: titled card, content inside) ---- */
.card{border:1px solid var(--ink); margin-top:26px; background:var(--paper)}
.card-h{display:flex; align-items:baseline; gap:14px; padding:12px 16px;
  border-bottom:1px solid var(--ink); flex-wrap:wrap}
.card-h .no{font-family:var(--disp); font-size:20px; letter-spacing:.12em;
  text-transform:uppercase}
.card-h .lede{font-size:11px; letter-spacing:.14em; text-transform:uppercase;
  color:var(--faint)}
.card-h .spacer{margin-left:auto}
.card-b{padding:16px}
/* card tabs, Flamingo swap|pool style */
.ctabs{display:flex; gap:0}
.ctabs a, .ctabs span.cur{padding:5px 14px; font-size:11.5px; letter-spacing:.14em;
  text-transform:uppercase; border:1px solid var(--ink); margin-left:-1px}
.ctabs span.cur{background:var(--ink); color:var(--paper)}
.ctabs a:hover{background:var(--ink); color:var(--paper)}

/* ---- trade grid: swap card + tokens table ---- */
.tradegrid{display:grid; grid-template-columns:5fr 7fr; gap:26px; align-items:start}

/* tokens table: restyle the ratio cards as rows, Flamingo "Tokens on Flamingo" */
.toktab .rgrid{display:block; counter-reset:row; border:0}
.toktab .rcard{border:0; border-bottom:1px solid var(--faint); counter-increment:row;
  display:grid; grid-template-columns:4ch 1.2fr 1fr 1fr auto; gap:10px;
  align-items:baseline; padding:9px 2px}
.toktab .rcard::before{content:counter(row,decimal-leading-zero); color:var(--faint);
  font-size:11px}
.toktab .rcard .rp{font-size:12px; letter-spacing:.08em; color:var(--ink)}
.toktab .rcard .rnum{font-size:21px; margin:0}
.toktab .rcard .rinv{font-size:11.5px}
.tokhead{display:grid; grid-template-columns:4ch 1.2fr 1fr 1fr auto; gap:10px;
  padding:0 2px 6px; font-size:10.5px; letter-spacing:.14em; text-transform:uppercase;
  color:var(--faint); border-bottom:3px double var(--ink)}

/* ---- FLOCKS-style earn card: the APY is the headline ---- */
.earngrid{display:grid; grid-template-columns:7fr 5fr; gap:26px; align-items:start}
.bign{font-family:var(--disp); font-size:clamp(44px,7vw,72px); line-height:1;
  margin:6px 0 2px}
.biglabel{font-size:11px; letter-spacing:.16em; text-transform:uppercase; color:var(--faint)}
.flockstats{display:grid; grid-template-columns:1fr 1fr 1fr; gap:10px; margin-top:14px}
.flockstats .fs{border-top:1px solid var(--ink); padding-top:8px}
.flockstats .fs .l{font-size:10px; letter-spacing:.14em; text-transform:uppercase;
  color:var(--faint)}
.flockstats .fs .v{font-family:var(--disp); font-size:19px; word-break:break-all}

/* ---- governance duo ---- */
.govgrid{display:grid; grid-template-columns:1fr 1fr; gap:26px; align-items:start}
.hero3mini{border:1px solid var(--ink); padding:16px}
.hero3mini h2{font-family:var(--disp); font-weight:400; font-size:30px; margin:.2em 0 .3em}

/* ---- claim + verify ---- */
.act3{margin-top:26px}

/* ---- footer ---- */
.foot3{margin-top:44px; padding-top:16px; border-top:3px double var(--ink);
  font-size:12px; color:var(--ink-2); line-height:1.9; max-width:70ch}

/* full-width primary button, slightly rounded — the Flamingo CTA shape */
.go{border-radius:3px; width:100%; text-align:center}

/* ---- clickable = phosphor key (MS-DOS terminal green, one size everywhere) ---- */
:root{--keybg:#101010; --keyfg:#33ff33}
a, button, .linkish, button.linkish, .tabs a, .cta, .cta.solid, .ctabs a{
  background:var(--keybg); color:var(--keyfg);
  border:1px solid var(--keyfg); border-radius:2px;
  padding:.45em .8em; font-size:12px; letter-spacing:.14em;
  text-transform:uppercase; text-decoration:none; line-height:1.2;
}
a:hover, button:hover, .linkish:hover, button.linkish:hover,
.tabs a:hover, .cta:hover, .cta.solid:hover, .cta:not(.solid):hover, .ctabs a:hover{
  background:var(--keyfg); color:var(--keybg);
}
/* the big CTAs are the same key, just full-width */
.ticket .go, .go{font-size:12px; padding:.7em .8em}
/* brand marks stay bare, phosphor on hover */
.topbar .bird3, .topbar .wordmark{background:transparent; color:var(--ink);
  border:0; padding:0; font-size:inherit; letter-spacing:.08em; text-transform:none}
.topbar .bird3:hover, .topbar .wordmark:hover{background:transparent; color:var(--keyfg)}
/* selected pill/tab = key fully engaged: solid phosphor, dark legend */
.tsel button.on, .ctabs span.cur{background:var(--keyfg); color:var(--keybg);
  border:1px solid var(--keyfg)}
/* tab + card-tab rows: keys sit flush like a keyboard */
.tabs a, .ctabs a, .ctabs span.cur{margin-left:-1px}
.tabs a:first-child, .ctabs a:first-child, .ctabs span.cur:first-child{margin-left:0}

@media (max-width:860px){
  .tradegrid, .earngrid, .govgrid{grid-template-columns:1fr}
  .tiles{grid-template-columns:1fr 1fr}
  .topbar{gap:10px; padding:8px 12px}
  .topright{margin-left:0}
}
"""

MARKUP = """<header class="topbar">
  <a class="bird3" href="#top" aria-label="Free the Bird">
    <svg viewBox="0 0 220 130" fill="none" stroke="currentColor" stroke-width="9"
         stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
      <path d="M14 84 C 46 30, 82 26, 110 62 C 138 26, 174 30, 206 84"/>
      <circle cx="110" cy="66" r="7"/>
      <line x1="110" y1="73" x2="110" y2="112"/>
    </svg>
  </a>
  <a class="wordmark" href="#top">FTB</a>
  <nav class="tabs" aria-label="sections">
    <a href="#trade">trade</a>
    <a href="#earn">earn</a>
    <a href="#lend">lend</a>
    <a href="#vote">vote</a>
    <a href="#board">board</a>
    <a href="#claim">claim</a>
    <a href="#verify">verify</a>
  </nav>
  <div class="topright">
    <span class="muted" id="rpcNote">rpc: connecting...</span>
    <span class="idstate" id="idstate">[ no wallet ]</span>
    <button id="connectBtn">connect -&gt;</button>
    <button class="linkish" id="themeBtn">theme</button>
    <button class="linkish" id="ownRpcBtn">own node -&gt;</button>
  </div>
</header>

<div id="top"></div>
<span id="dateline" hidden>// CONNECTING //</span>

<section class="hero3">
  <h1>the flamingo, set free.</h1>
  <p class="sub">trade · earn · borrow · govern — ratios only, no fiat, no oracle, no admin.</p>
  <div class="cta-row">
    <a class="cta solid" href="#trade">start trading -&gt;</a>
    <a class="cta" href="#earn">earn dividends -&gt;</a>
    <a class="cta" href="#vote">govern -&gt;</a>
  </div>
</section>

<div class="page">

  <!-- ============ STAT BAND ============ -->
  <div class="tiles">
    <div class="tile"><div class="tl">ftb supply</div><div class="tn" id="tSupply">—</div></div>
    <div class="tile"><div class="tl">hard cap</div><div class="tn" id="tCap">—</div></div>
    <div class="tile"><div class="tl">weight voted</div><div class="tn" id="tVoted">—</div></div>
    <div class="tile"><div class="tl">block</div><div class="tn" id="tBlock">—</div></div>
  </div>

  <div class="ticker" id="ticker" style="margin-top:12px">// market: waiting for the pools</div>

  <!-- ============ TRADE: swap card + tokens table ============ -->
  <div class="tradegrid">
    <div class="card" id="trade">
      <div class="card-h">
        <div class="ctabs"><span class="cur">swap</span><a href="#market">pools</a></div>
      </div>
      <div class="card-b">
        <div class="bal" id="balStrip" hidden>
          <span>wallet:</span>
          <span>FTB <b id="balFtb">—</b></span>
          <span>NEO <b id="balNeo">—</b></span>
          <span>GAS <b id="balGas">—</b></span>
          <span>claimable <b id="balClaim">—</b></span>
        </div>
        <div id="swap">
          <div class="ticket" style="border:0; padding:0">
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

    <div class="card toktab" id="market">
      <div class="card-h">
        <span class="no">tokens on ftb</span>
        <span class="spacer"></span>
        <span class="lede">ratios, not fiat — no oracle</span>
        <button class="linkish" id="ratioBtn">refresh -&gt;</button>
      </div>
      <div class="card-b">
        <div class="tokhead"><span>#</span><span>pair</span><span>ratio</span><span>inverse</span><span>drift</span></div>
        <div class="rgrid" id="ratioBody">
          <div class="rcard">
            <div class="rp">standby</div>
            <div class="rnum">—</div>
            <div class="rinv" id="ratioHint">loads when the router and pairs are configured for this network</div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- ============ EARN + LEND ============ -->
  <div class="earngrid">
    <div class="card" id="earn">
      <div class="card-h">
        <span class="no">ftb dividends</span>
        <span class="spacer"></span>
        <span class="lede">the holders' 2/9 of every fee</span>
        <button class="read linkish" id="earnRefresh">refresh -&gt;</button>
      </div>
      <div class="card-b">
        <div class="biglabel">dividend apy</div>
        <div class="bign" id="dApy">—</div>
        <div class="flockstats">
          <div class="fs"><div class="l">total staked</div><div class="v" id="dTotal">—</div></div>
          <div class="fs"><div class="l">growth / ftb</div><div class="v" id="dAcc">—</div></div>
          <div class="fs"><div class="l">you can claim</div><div class="v" id="dPending">—</div></div>
        </div>
        <div id="earnPos" hidden style="margin-top:14px">
          <div class="meta" style="display:flex; gap:8px; align-items:center; flex-wrap:wrap">
            <span class="tag">staked</span><span id="dStaked">—</span>
            <span class="tag">stake</span><input id="dvStakeAmt" inputmode="decimal" autocomplete="off" placeholder="0.0" style="width:9ch">
            <button class="linkish" id="dvStakeGo">stake -&gt;</button>
            <span class="tag">unstake</span><input id="dvUnstakeAmt" inputmode="decimal" autocomplete="off" placeholder="0.0" style="width:9ch">
            <button class="linkish" id="dvUnstakeGo">unstake -&gt;</button>
            <button class="linkish" id="dvClaimGo">claim -&gt;</button>
          </div>
          <p class="small muted" id="dvHint">review-and-sign in your wallet — the page never holds keys</p>
        </div>
        <p class="small muted" id="earnOff">standby — goes live when the dividends module hash is configured for this network</p>
      </div>
    </div>

    <div class="card" id="lend">
      <div class="card-h">
        <span class="no">lend</span>
        <span class="spacer"></span>
        <span class="lede">NEO+GAS collateral · FTB loans</span>
        <button class="read linkish" id="lendRefresh">refresh -&gt;</button>
      </div>
      <div class="card-b">
        <div class="flockstats">
          <div class="fs"><div class="l">utilization</div><div class="v" id="lUtil">—</div></div>
          <div class="fs"><div class="l">borrow apy</div><div class="v" id="lBApy">—</div></div>
          <div class="fs"><div class="l">supply apy</div><div class="v" id="lSApy">—</div></div>
        </div>
        <div class="flockstats" style="grid-template-columns:1fr 1fr">
          <div class="fs"><div class="l">borrows</div><div class="v" id="lBorrows">—</div></div>
          <div class="fs"><div class="l">holder reserve</div><div class="v" id="lReserve">—</div></div>
        </div>
        <div id="lendPos" hidden style="margin-top:14px">
          <div class="meta" style="display:flex; gap:8px; align-items:center; flex-wrap:wrap">
            <span class="tag pub" id="pHealth">—</span>
            <span class="tag">NEO</span><span id="pNeo">—</span>
            <span class="tag">GAS</span><span id="pGas">—</span>
            <span class="tag">debt</span><span id="pDebt">—</span>
            <span class="tag">max</span><span id="pMax">—</span>
            <span class="tag">shares</span><span id="pShares">—</span>
          </div>
        </div>
        <p class="small muted" id="lendOff">standby — goes live when the lend contract hash is configured for this network</p>
      </div>
    </div>
  </div>

  <!-- ============ GOVERNANCE ============ -->
  <div class="govgrid">
    <div class="card" id="vote">
      <div class="card-h">
        <span class="no">governance</span>
        <span class="spacer"></span>
        <span class="lede">snapshot-weighted, counted on-chain</span>
        <button class="linkish" id="govRefresh">tally -&gt;</button>
      </div>
      <div class="card-b">
        <h2 class="disp" style="font-size:28px; margin:.1em 0 .3em" id="govQ">no live ballot</h2>
        <p class="small muted" id="govHint">weight = your snapshot balance.</p>
        <div id="govBody"></div>
      </div>
    </div>

    <div class="card" id="board">
      <div class="card-h">
        <span class="no">proposals</span>
        <span class="spacer"></span>
        <span class="lede">bonded — spam pays</span>
        <button class="linkish" id="boardRefresh">refresh -&gt;</button>
      </div>
      <div class="card-b">
        <div id="boardBody">
          <div class="entry">
            <span class="idx">//</span>
            <div><span class="ttl">standby — the board loads when the registry is configured for this network</span></div>
            <div class="meta"><span class="tag">standby</span></div>
          </div>
        </div>
        <div class="ticket" style="margin-top:12px">
          <div class="trow">
            <label for="pMeta">proposal</label>
            <input id="pMeta" maxlength="1024" autocomplete="off" placeholder="what should the community decide?">
            <div class="tsel" id="pKindSel" role="group" aria-label="proposal kind"></div>
          </div>
          <div class="trow">
            <label for="pRef">reference</label>
            <input id="pRef" autocomplete="off" spellcheck="false" placeholder="0x… ballot contract hash, or the token hash for a listing">
          </div>
          <div class="tmeta" id="pMetaHint">bond: — FTB · exact, escrowed — refunded on a legitimate close, kept against spam</div>
          <button class="go" id="pGo">review &amp; sign -&gt;</button>
        </div>
      </div>
    </div>
  </div>

  <!-- ============ CLAIM ============ -->
  <div class="card act3" id="claim">
    <div class="card-h">
      <span class="no">claim</span>
      <span class="spacer"></span>
      <span class="lede">migration — claim your FTB</span>
    </div>
    <div class="card-b">
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
        <div class="meta"><span id="claimHint">connect a wallet to check</span></div>
      </div>
      <div class="entry">
        <span class="idx">03</span>
        <div><span class="ttl">The attacker is not in the snapshot.</span></div>
        <button class="read linkish" id="proofsBtn">proofs -&gt;</button>
        <div class="meta"><span class="tag">merkle</span><span id="rootLine">root —</span><span>4,847 holders</span></div>
      </div>
    </div>
  </div>

  <!-- ============ VERIFY ============ -->
  <div class="card act3" id="verify">
    <div class="card-h">
      <span class="no">verify</span>
      <span class="spacer"></span>
      <span class="lede">verify everything — trust nothing</span>
    </div>
    <div class="card-b">
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
        Check every hash against the repo before you sign. After close + renounceOwnership
        the token has no admin — readable on-chain, forever.
      </p>
    </div>
  </div>

  <footer class="foot3">
    <span class="disp" style="font-size:20px">free the bird</span><br>
    <span class="muted" id="shiftLine">// day shift</span><br>
    Reference implementation pending external audit. Not financial advice.
    MIT code, public proofs — a gift to the Flamingo community.
  </footer>
</div>

"""


def main():
    out = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>free the bird — template 3 (flamingo blend)</title>
<style>
""" + font + "\n" + style_all + "\n" + T3_CSS + """
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

    open('template3.html', 'w').write(out)
    print('wrote template3.html', len(out), 'bytes')


if __name__ == '__main__':
    main()
