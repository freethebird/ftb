#!/usr/bin/env sh
# Compile every deployable contract to NEF + manifest in build/.
# Requires neo-go on PATH (see `make tools` or the neo-go releases page).
set -eu

mkdir -p build
for c in ftb_token ftb_migrator registry oracle lend ballot bondvault treasury dividends; do
	echo "compiling $c"
	neo-go contract compile \
		-i "contracts/$c/$c.go" \
		-c "contracts/$c/$c.yml" \
		-o "build/$c.nef" \
		-m "build/$c.manifest.json"
done
# R2 deploy variants (renamed manifests -> new contract hashes). R1 artifacts in
# build/ are historical (already deployed code) and are NOT rebuilt here.
for v in ftb_migrator_r2 ballot_r2; do
	src="${v%_r2}"
	echo "compiling $v"
	neo-go contract compile \
		-i "contracts/$src/$src.go" \
		-c "contracts/$src/$v.yml" \
		-o "build/$v.nef" \
		-m "build/$v.manifest.json"
done
# R3/R4 variants (same rename pattern).
for v in ftb_migrator_r3 ftb_migrator_r4 ballot_r3 ballot_r4; do
	src="${v%_r?}"
	echo "compiling $v"
	neo-go contract compile \
		-i "contracts/$src/$src.go" \
		-c "contracts/$src/$v.yml" \
		-o "build/$v.nef" \
		-m "build/$v.manifest.json"
done
# Per-proposal ballots (genesis votes 2 and 3; one deployment per question).
for v in ballot_launchdate ballot_claimwindow; do
	echo "compiling $v"
	neo-go contract compile \
		-i contracts/ballot/ballot.go \
		-c "contracts/ballot/$v.yml" \
		-o "build/$v.nef" \
		-m "build/$v.manifest.json"
done
# R4 reset variants: same module code, renamed manifests -> fresh contract hashes,
# so the reset deploys clean state instead of inheriting old-FTB accounting.
for v in dividends_r2 lend_r2 bondvault_r2 treasury_r2; do
	src="${v%_r2}"
	echo "compiling $v"
	neo-go contract compile \
		-i "contracts/$src/$src.go" \
		-c "contracts/$src/$v.yml" \
		-o "build/$v.nef" \
		-m "build/$v.manifest.json"
done
echo "contracts compiled to build/"
