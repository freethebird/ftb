#!/usr/bin/env sh
# Compile-twice determinism check — QRN class D (proof-system and implementation
# integrity). A reproducible build must be byte-stable not only across machines but
# across runs: neo-go v0.123.0 emitted different (semantically equal) NEFs per run for
# one contract (FTB-33: multi-argument append of computed values goes through a
# map-ordered emission path in the compiler). Recording a hash of whichever permutation
# a build happened to produce is not a reproducibility gate, so every contract is
# compiled a second time here and the two outputs must be byte-identical.
set -eu
cd "$(dirname "$0")/.."

tmp="build/.determinism"
rm -rf "$tmp"
mkdir -p "$tmp"
fail=0
for c in ftb_token ftb_migrator registry oracle lend ballot bondvault treasury dividends dbondvault accusation; do
	neo-go contract compile \
		-i "contracts/$c/$c.go" \
		-c "contracts/$c/$c.yml" \
		-o "$tmp/$c.nef" \
		-m "$tmp/$c.manifest.json" >/dev/null
	a=$(shasum -a 256 "build/$c.nef" | awk '{print $1}')
	b=$(shasum -a 256 "$tmp/$c.nef" | awk '{print $1}')
	if [ "$a" != "$b" ]; then
		echo "nef-determinism: $c differs between two compiles of the SAME source" >&2
		echo "  run A: $a" >&2
		echo "  run B: $b" >&2
		fail=1
	fi
done
rm -rf "$tmp"
if [ "$fail" -ne 0 ]; then
	echo "nef-determinism: FAIL — the compiler is emitting per-run permutations; see FTB-33" >&2
	exit 1
fi
echo "nef-determinism: every contract byte-identical across two compiles"
