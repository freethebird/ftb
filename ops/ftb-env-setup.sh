#!/usr/bin/env bash
#
# ftb-env-setup.sh : build-and-test environment for the FTB repo on a fresh VPS
# (Debian 12 or Ubuntu 22.04+; amd64 or arm64). Installs build tools and a pinned Go,
# installs the neo-go compiler, then compiles every contract and runs every test.
#
# It is idempotent (safe to re-run), needs no root if you can sudo, and touches only
# standard locations (/usr/local/go, your Go path, this repo). It does NOT run a node:
# the contract tests use an in-memory chain, so nothing listens on the network here.
#
# Usage, from inside the repo:
#     GO_SHA256=<sha from https://go.dev/dl/> bash ops/ftb-env-setup.sh
#
# Environment variables:
#     GO_VERSION   Go version to install           (default 1.26.0; repo requires 1.26)
#     GO_SHA256    sha256 of the Go tarball         (optional but recommended; see go.dev/dl)
#     REPO_DIR     path to the FTB repo             (default: the repo this script lives in)
#     RUN_BUILD    "yes" to compile+test at the end (default yes)
#
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.26.0}"
GO_SHA256="${GO_SHA256:-}"
REPO_DIR="${REPO_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
RUN_BUILD="${RUN_BUILD:-yes}"

say() { printf '\n\033[1;35m==> %s\033[0m\n' "$*"; }
err() { printf '\n\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  command -v sudo >/dev/null 2>&1 || err "not root and no sudo; run as root or install sudo"
  SUDO="sudo"
fi

say "1/6  OS packages"
$SUDO apt-get update -y
$SUDO DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  build-essential git curl ca-certificates make tar

# Go tarball architecture
arch="$(dpkg --print-architecture 2>/dev/null || uname -m)"
case "$arch" in
  amd64|x86_64)  GOARCH=amd64 ;;
  arm64|aarch64) GOARCH=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

say "2/6  Go ${GO_VERSION} (${GOARCH})"
need_go=yes
if [ -x /usr/local/go/bin/go ]; then
  cur="$(/usr/local/go/bin/go version | awk '{print $3}')"
  if [ "$cur" = "go${GO_VERSION}" ]; then need_go=no; echo "already installed: ${cur}"; fi
fi
if [ "$need_go" = yes ]; then
  tarball="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "downloading https://go.dev/dl/${tarball}"
  curl -fSL "https://go.dev/dl/${tarball}" -o "${tmp}/${tarball}"
  if [ -n "$GO_SHA256" ]; then
    echo "${GO_SHA256}  ${tmp}/${tarball}" | sha256sum -c - || err "Go tarball checksum FAILED"
    echo "checksum ok"
  else
    echo "WARNING: GO_SHA256 not set, skipping tarball verification."
    echo "         For a verified install, copy the sha256 from https://go.dev/dl/ and re-run."
  fi
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf "${tmp}/${tarball}"
fi

# Put Go and the Go bin dir (where neo-go lands) on PATH, now and for future logins.
GOPATH_BIN="$(/usr/local/go/bin/go env GOPATH)/bin"
export PATH="/usr/local/go/bin:${GOPATH_BIN}:${PATH}"
if ! grep -q 'ftb-env-setup.sh' "${HOME}/.profile" 2>/dev/null; then
  cat >> "${HOME}/.profile" <<'EOF'

# Go + neo-go on PATH (added by ftb-env-setup.sh)
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
EOF
fi
go version

say "3/6  neo-go compiler (make tools)"
[ -f "${REPO_DIR}/Makefile" ] || err "no Makefile in REPO_DIR=${REPO_DIR}; point REPO_DIR at the FTB repo"
cd "${REPO_DIR}"
make tools
if command -v neo-go >/dev/null 2>&1; then neo-go --version || true; else err "neo-go not on PATH after 'make tools'"; fi

if [ "$RUN_BUILD" != yes ]; then
  say "DONE (build skipped: RUN_BUILD=${RUN_BUILD}). Run 'make' in ${REPO_DIR} when ready."
  exit 0
fi

say "4/6  resolve modules (go mod tidy)"
go mod tidy

say "5/6  compile every contract (make build)"
make build

say "6/6  run every test (make test)"
make test

say "DONE: contracts compiled to ${REPO_DIR}/build and all tests passed."
echo "Next: see DECISIONS.md (cap number, oracle governance) and README.md (deploy/seed runbooks)."
