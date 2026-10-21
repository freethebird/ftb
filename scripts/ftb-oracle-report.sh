#!/usr/bin/env bash
# ftb-oracle-report.sh — push fresh NEO + GAS prices to the FTB oracle so the
# testnet lend module can price collateral.
#
# Why: the oracle ignores any report older than its freshness window (currently
# 24h) and needs at least `minReports` fresh ones (currently 1). If nothing pushes,
# the feed goes stale and `startLoan` reverts with "oracle: not enough fresh prices".
# Run this on a timer well inside the window — every 6h is ample headroom.
#
# One-time setup:
#   1. Put your ENABLED reporter wallet somewhere readable only by this user, e.g.
#        /opt/ftb/reporter.wallet.json
#   2. Create a neo-go wallet-config YAML so the run is non-interactive (no prompt):
#        # /opt/ftb/reporter.yml
#        Path: /opt/ftb/reporter.wallet.json
#        Accounts:
#          - Address: <reporterAddress>
#            Password: "<walletPassword>"
#      chmod 600 it — it holds the password.
#   3. Fill REPORTER / WALLET_CONFIG / RPC below.
#   4. Install the cron line at the bottom of this file.
set -euo pipefail

# ---- config --------------------------------------------------------------
ORACLE="0x1c92a93d438b9468b45706cbc4b1ddcc345df875"   # FTB oracle (what registry.get("oracle") resolves to)
REPORTER="<reporterAddress>"                           # your ENABLED reporter (Neo address, N...)
WALLET_CONFIG="/opt/ftb/reporter.yml"                  # neo-go wallet-config (path + password)
RPC="https://testnet1.neo.coz.io:443"                  # a SYNCED node — switch to your own once it catches up
NEOGO="neo-go"                                         # path to the neo-go binary if not on PATH

# assets + prices. price = FTB base units (8 decimals) per WHOLE unit of the asset,
# matching the lend module's "1 NEO = 100 FTB / 1 GAS = 40 FTB" ratios.
NEO_HASH="0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5"
GAS_HASH="0xd2a4cff31913016155e38e474a2c06d08be276cf"
NEO_PRICE=10000000000   # 100 FTB per NEO  (100 * 1e8)
GAS_PRICE=4000000000    #  40 FTB per GAS  ( 40 * 1e8)
#   To make these live later: fetch NEO/USD and GAS/USD, divide by an FTB/USD
#   reference, * 1e8. For a testnet keep-alive, fixed values are fine.

LOG="${FTB_ORACLE_LOG:-/var/log/ftb-oracle-report.log}"
# --------------------------------------------------------------------------

ts() { date -u +'%Y-%m-%dT%H:%M:%SZ'; }

push() {
  local asset="$1" price="$2" label="$3"
  echo "$(ts)  report $label = $price" >> "$LOG"
  # report(reporter Hash160, asset Hash160, price int); the tx is signed by the
  # reporter (CheckWitness). neo-go accepts a Neo address for a Hash160 param and
  # for the signer; if your neo-go rejects the address as the first arg, convert it
  # once with `neo-go util convert <address>` and use the 0x... script hash instead.
  if "$NEOGO" contract invokefunction \
        -r "$RPC" \
        --wallet-config "$WALLET_CONFIG" \
        --await \
        "$ORACLE" report \
        "$REPORTER" "$asset" "int:$price" \
        -- "$REPORTER" >> "$LOG" 2>&1; then
    echo "$(ts)  $label OK" >> "$LOG"
  else
    echo "$(ts)  $label FAILED (detail above)" >> "$LOG"; return 1
  fi
}

echo "$(ts)  === oracle report run ===" >> "$LOG"
rc=0
push "$NEO_HASH" "$NEO_PRICE" "NEO" || rc=1
push "$GAS_HASH" "$GAS_PRICE" "GAS" || rc=1
exit "$rc"

# ---- install: crontab -e, then add (every 6 hours) -----------------------
#   0 */6 * * *  /opt/ftb/ftb-oracle-report.sh
# ---- or a systemd timer (ftb-oracle-report.timer: OnCalendar=*-*-* 00/6:00:00)
