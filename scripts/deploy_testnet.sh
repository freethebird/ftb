#!/bin/sh
# Deploys the FTB core (token, migrator, registry) plus the first governance ballot to
# Neo N3 TestNet, then fills web/config.js with the deployed hashes.
#
# Prereq: cmd/deploy/testnet.wallet.json must hold enough TestNet GAS (~100 GAS is ample;
# faucet: https://faucet.ngd.network/?n3=1). Wallet password lives in cmd/deploy/.pass
# (git-ignored, chmod 600).
#
#   sh scripts/deploy_testnet.sh
#
set -eu
cd "$(dirname "$0")/.."

export FTB_DEPLOY_PASS=$(cat cmd/deploy/.pass)

echo "== compiling contracts =="
sh scripts/compile.sh

echo "== deploying to TestNet =="
go run ./cmd/deploy -config cmd/deploy/config.testnet.json | tee /tmp/ftb-testnet-deploy.log

LINE=$(grep '^FTB_HASHES ' /tmp/ftb-testnet-deploy.log | tail -1 | sed 's/^FTB_HASHES //')
if [ -z "$LINE" ]; then
  echo "ERROR: no FTB_HASHES line in deploy output" >&2
  exit 1
fi

echo "== writing web/config.js =="
node -e '
const fs = require("fs");
const h = JSON.parse(process.argv[1]);
let s = fs.readFileSync("web/config.js", "utf8");
for (const k of ["ftb","migrator","registry","ballot"]) {
  if (!h[k]) continue;
  s = s.replace(new RegExp("(" + k + ":\\s*)'"'"'0x[0-9a-f]{40}'"'"'"), "$1'"'"'" + h[k] + "'"'"'");
}
s = s.replace(
  /ballot: \{ question: '"'"''"'"', options: \[\] \}/,
  "ballot: { question: '"'"'ratios: r1 = 1 FLM : 1 FTB, r2 = 4 FLOCKS : 1 FTB — adopt?'"'"', options: ['"'"'adopt r1=1, r2=4'"'"', '"'"'wait for the union deal'"'"'] }"
);
fs.writeFileSync("web/config.js", s);
console.log("web/config.js updated:", JSON.stringify(h));
'

echo
echo "Done. Contract hashes are in web/config.js."
echo "NOTE: router stays a placeholder — there is no Flamingo AMM on TestNet (neo#3285),"
echo "so the market board and swap ticket stay in standby until pools exist."
echo "Migration, balances, and ballot tallies are live immediately."
