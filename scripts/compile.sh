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
echo "contracts compiled to build/"
