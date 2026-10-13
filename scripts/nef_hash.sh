#!/usr/bin/env sh
# NEF hash record/verify — QRN class D and Go canon Book II rule 10 (reproducible builds).
# Prints the sha256 of every compiled contract NEF. If NEF_RELEASE.sha256 exists at the
# repo root, the current build must match it exactly; a mismatch fails the gate, because
# a release artifact that does not reproduce is a divergence, and divergence fails closed.
set -eu
cd "$(dirname "$0")/.."

out="build/NEF_HASHES.sha256"
if ! ls build/*.nef >/dev/null 2>&1; then
	echo "no build/*.nef; run 'make build' first" >&2
	exit 1
fi

shasum -a 256 build/*.nef | sed 's|  build/|  |' | tee "$out"

if [ -f NEF_RELEASE.sha256 ]; then
	if diff -q NEF_RELEASE.sha256 "$out" >/dev/null; then
		echo "nefhash: current build matches the recorded release set"
	else
		echo "nefhash: MISMATCH vs NEF_RELEASE.sha256 (recorded release set)" >&2
		diff NEF_RELEASE.sha256 "$out" >&2 || true
		exit 1
	fi
else
	echo "nefhash: no NEF_RELEASE.sha256 yet; record a release with: cp $out NEF_RELEASE.sha256"
fi
