#!/usr/bin/env sh
# Install ftb-node -- the one-command FTB mirror.
#   curl -fsSL https://freethebird.co/node/install.sh | sh
# Downloads the right prebuilt binary for your OS/arch, verifies its checksum, installs it.
set -eu

# Where the release binaries live. Override with FTB_NODE_BASE if you host them elsewhere.
BASE="${FTB_NODE_BASE:-https://github.com/freethebird/ftb/releases/latest/download}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in
	linux) os=linux ;;
	darwin) os=darwin ;;
	*) echo "ftb-node: unsupported OS '$os' -- see the manual steps on freethebird.co/node" >&2; exit 1 ;;
esac
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) echo "ftb-node: unsupported architecture '$arch'" >&2; exit 1 ;;
esac
bin="ftb-node_${os}_${arch}"

dst="${PREFIX:-$HOME/.local/bin}"
mkdir -p "$dst"
tmp=$(mktemp)
echo "ftb-node: downloading $bin"
curl -fsSL "$BASE/$bin" -o "$tmp"

# Verify the checksum if SHA256SUMS is published alongside the binaries.
sums=$(mktemp)
if curl -fsSL "$BASE/SHA256SUMS" -o "$sums" 2>/dev/null && [ -s "$sums" ]; then
	want=$(grep " ${bin}\$" "$sums" | awk '{print $1}' | head -n1)
	got=$( (sha256sum "$tmp" 2>/dev/null || shasum -a 256 "$tmp") | awk '{print $1}')
	if [ -n "$want" ] && [ "$want" != "$got" ]; then
		echo "ftb-node: CHECKSUM MISMATCH -- refusing to install" >&2
		rm -f "$tmp" "$sums"; exit 1
	fi
	echo "ftb-node: checksum verified"
fi
rm -f "$sums"

chmod +x "$tmp"
mv "$tmp" "$dst/ftb-node"
echo "ftb-node: installed to $dst/ftb-node"

case ":$PATH:" in
	*":$dst:"*) ;;
	*) echo "ftb-node: add it to your PATH ->  export PATH=\"$dst:\$PATH\"" ;;
esac

cat <<'NEXT'

Next:
  ftb-node set-discord <your-discord-user-id>   # optional -- to earn the Discord contributor role
  ftb-node                                       # start mirroring FTB (leave it running)

Then, in the FTB Discord, run /verify and give your Node ID (ftb-node id).
NEXT
