#!/usr/bin/env bash
# ftb-deploy-gate.sh — publish-time floor. Runs on the STAGED web/ tree about to go to
# IPFS. Exit 0 = may publish; nonzero = caller MUST NOT publish (live site stays put).
# Checks: JS parses, config profile correct, proofs.json intact, HTML asset refs resolve,
# no secret in the public bundle. MO Article 6: every failure named, never a silent pass.
set -uo pipefail
STAGE="${1:?usage: ftb-deploy-gate.sh <staged-web-dir> <profile>}"
PROFILE="${2:?usage: ftb-deploy-gate.sh <staged-web-dir> <profile>}"
fail=0; note(){ printf 'GATE: %s\n' "$*" >&2; fail=1; }

# 1) JS syntax — a parse error white-screens the dApp
if command -v node >/dev/null 2>&1; then
  while IFS= read -r -d '' f; do
    node --check "$f" >/dev/null 2>&1 || note "JS syntax error: ${f#"$STAGE"/}"
  done < <(find "$STAGE" -type f \( -name '*.js' -o -name '*.mjs' \) -print0)
else
  note "node missing — cannot verify JS syntax"
fi

# 2) config profile — shipped config.js must equal the intended profile + declare its network
if [ ! -f "$STAGE/config.js" ]; then note "config.js missing"
elif [ ! -f "$STAGE/config.$PROFILE.js" ]; then note "config.$PROFILE.js missing"
else
  cmp -s "$STAGE/config.js" "$STAGE/config.$PROFILE.js" || note "config.js != config.$PROFILE.js"
  case "$PROFILE" in
    testnet) grep -q "network:[[:space:]]*'TestNet'" "$STAGE/config.js" || note "config.js not TestNet";;
    mainnet) grep -q "network:[[:space:]]*'MainNet'" "$STAGE/config.js" || note "config.js not MainNet";;
  esac
  grep -q "ftb:[[:space:]]*'0x0000" "$STAGE/config.js" && note "config.js ships a placeholder ftb contract"
fi

# 3) proofs.json — valid JSON, non-empty entries[]
if [ ! -f "$STAGE/proofs.json" ]; then note "proofs.json missing"
else node -e 'try{const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));process.exit(Array.isArray(j.entries)&&j.entries.length?0:1)}catch(e){process.exit(1)}' "$STAGE/proofs.json" 2>/dev/null || note "proofs.json invalid/empty"
fi

# 4) every local <script src>/<link href> an HTML page names must exist in the bundle
missing=""
for html in "$STAGE"/*.html; do
  [ -f "$html" ] || continue
  for ref in $(grep -oE '(src|href)="[^"]+\.(js|css|mjs)"' "$html" 2>/dev/null | sed -E 's/.*="([^"]+)"/\1/'); do
    case "$ref" in http*|//*|data:*) continue;; esac
    rp="${ref#./}"; rp="${rp#/}"; [ -f "$STAGE/$rp" ] || missing="$missing ${html##*/}->$ref"
  done
done
[ -n "$missing" ] && note "broken asset ref(s):$missing"

# 5) no private key / token in the public, permanent bundle
grep -rEIl -e '-----BEGIN [A-Z ]*PRIVATE KEY-----' -e 'AKIA[0-9A-Z]{16}' \
  -e 'ghp_[0-9A-Za-z]{36}' -e 'xox[baprs]-[0-9A-Za-z-]{10,}' "$STAGE" >/dev/null 2>&1 \
  && note "possible secret in the staged bundle"

[ "$fail" -ne 0 ] && { echo "GATE: FAIL — publish blocked." >&2; exit 1; }
echo "GATE: PASS"; exit 0
