#!/usr/bin/env sh
# Compile every deployable contract to NEF + manifest in build/.
# Requires neo-go on PATH (see `make tools` or the neo-go releases page).
set -eu

mkdir -p build
for c in ftb_token ftb_migrator registry oracle lend ballot; do
	echo "compiling $c"
	neo-go contract compile \
		-i "contracts/$c/$c.go" \
		-c "contracts/$c/$c.yml" \
		-o "build/$c.nef" \
		-m "build/$c.manifest.json"
done
echo "contracts compiled to build/"
