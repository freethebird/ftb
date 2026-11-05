#!/usr/bin/env python3
"""Build the single-file FTB export: the modular index.html with config.js,
ftb-dapi.js, every js/ module, the page CSS (index.css), the snapshot proofs,
and a beat.json stamp inlined, plus a fetch shim so the file works from file://
with no siblings. The export's inline <script>/<style> are carried in the CSP by
SHA-256 hash (Canonical Bible, HTML edition: never a blanket 'unsafe-inline'),
so the single-file form runs under the same self-only policy as the modular one.
Output: ../../ftb-export.html (workspace root). Regenerate after any change to
index.html, index.css, js/, config.js, or proofs.json — the QA gate (QA_GATE.md)
requires a fresh export per publish.
"""
import json
import pathlib
import re
import subprocess
import datetime
import hashlib
import base64

WEB = pathlib.Path(__file__).resolve().parent
OUT = WEB.parent.parent / "ftb-export.html"

tpl = (WEB / "index.html").read_text()
proofs = json.loads((WEB / "proofs.json").read_text())

# the ordered script tags the module split loads; inlined verbatim, in order
SRCS = (["config.js", "ftb-dapi.js"]
        + [f"js/{n}.js" for n in (
            "ftb-rpc", "ftb-state", "shell",
            "mod-token", "mod-swap", "mod-wallet")])

commit = subprocess.run(["git", "rev-parse", "--short", "HEAD"], cwd=WEB.parent,
                        capture_output=True, text=True).stdout.strip() or "unknown"
config_text = (WEB / "config.js").read_text()
m = re.search(r"network:\s*'(\w+)'", config_text)
network = m.group(1) if m else "unknown"
beat = {"project": "ftb", "commit": commit, "network": network,
        "published": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "ipns": "k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq"}

def csp_hash(inner_text):
    # CSP source-expression hash over the EXACT inline element text content
    d = hashlib.sha256(inner_text.encode("utf-8")).digest()
    return "'sha256-" + base64.b64encode(d).decode("ascii") + "'"

# --- inline <script> inner texts (hashed and wrapped identically) ---
shim_inner = f"""
/* inlined snapshot proofs ({proofs['count']:,} entries) + fetch shim: requests for
   proofs.json and beat.json are answered from memory — the export needs no sibling files */
window.FTB_PROOFS_JSON = {json.dumps(proofs)};
window.FTB_BEAT_JSON = {json.dumps(beat)};
(function(){{ var orig = window.fetch;
  window.fetch = function(u, opts){{
    if (typeof u === "string" && /proofs\\.json(\\?|$)/.test(u)){{
      return Promise.resolve(new Response(JSON.stringify(window.FTB_PROOFS_JSON),
        {{ status: 200, headers: {{ "Content-Type": "application/json" }} }}));
    }}
    if (typeof u === "string" && /(^|\\/)beat\\.json(\\?|$)/.test(u) && !/https?:/.test(u)){{
      return Promise.resolve(new Response(JSON.stringify(window.FTB_BEAT_JSON),
        {{ status: 200, headers: {{ "Content-Type": "application/json" }} }}));
    }}
    return orig(u, opts);
  }};
}})();
"""
script_inners = [shim_inner]
for src in SRCS:
    script_inners.append(f"\n/* inlined from {src} */\n{(WEB / src).read_text()}\n")

# --- inline <style> inner text (page CSS, inlined so the export is self-contained) ---
style_inner = "\n" + (WEB / "index.css").read_text() + "\n"

# --- hashes for the export's CSP ---
script_hashes = " ".join(csp_hash(x) for x in script_inners)
style_hash = csp_hash(style_inner)

# --- assemble the export ---
shim = "\n".join("<script>" + x + "</script>" for x in script_inners)
old = "\n".join(f'<script defer src="{s}"></script>' for s in SRCS)
assert old in tpl, "script tag block not found in index.html — has the load list changed?"
out = tpl.replace(old, shim)

link = '<link rel="stylesheet" href="index.css">'
assert link in out, "index.css <link> not found in index.html — has the stylesheet ref changed?"
out = out.replace(link, "<style>" + style_inner + "</style>")

# extend the self-only CSP with the exact inline hashes (no blanket 'unsafe-inline')
assert "script-src 'self'" in out and "style-src 'self'" in out, "expected a self-only CSP to extend"
out = out.replace("script-src 'self'", "script-src 'self' " + script_hashes, 1)
out = out.replace("style-src 'self'", "style-src 'self' " + style_hash, 1)

OUT.write_text(out)
print(f"wrote {OUT} ({OUT.stat().st_size/1e6:.1f} MB), network {network}, "
      f"beat commit {commit}, proofs {proofs['count']}, "
      f"{len(script_inners)} script-hashes + 1 style-hash in CSP")
