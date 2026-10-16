#!/usr/bin/env python3
"""Build the single-file FTB export: template3.html with config.js, ftb-dapi.js,
the snapshot proofs, and a beat.json stamp inlined, plus a fetch shim so the file
works from file:// with no siblings. Output: ../../ftb-template3-export.html
(workspace root). Regenerate after any config.js or template change.
"""
import json
import pathlib
import subprocess
import datetime

WEB = pathlib.Path(__file__).resolve().parent
OUT = WEB.parent.parent / "ftb-template3-export.html"

tpl = (WEB / "template3.html").read_text()
config = (WEB / "config.js").read_text()
dapi = (WEB / "ftb-dapi.js").read_text()
proofs = json.loads((WEB / "proofs.json").read_text())

commit = subprocess.run(["git", "rev-parse", "--short", "HEAD"], cwd=WEB.parent,
                        capture_output=True, text=True).stdout.strip() or "unknown"
beat = {"project": "ftb", "commit": commit,
        "published": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "ipns": "k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq"}

shim = f"""<script>
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
</script>
<script>
/* inlined from config.js */
{config}
</script>
<script>
/* inlined from ftb-dapi.js */
{dapi}
</script>"""

old = '<script src="config.js"></script>\n<script src="ftb-dapi.js"></script>'
assert old in tpl, "script tags not found in template3.html"
out = tpl.replace(old, shim)
OUT.write_text(out)
print(f"wrote {OUT} ({OUT.stat().st_size/1e6:.1f} MB), beat commit {commit}, proofs {proofs['count']}")
