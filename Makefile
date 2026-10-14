NEOGO_VERSION ?= v0.123.0

.PHONY: all tools fmt vet build test test-race vuln nefhash deepcheck conformance clean

## fmt + vet + build + test, the whole audit in one command
all: fmt vet build test

## install the neo-go compiler (also provides the SDK and node)
## note: at v0.123.0 the CLI package is cli/ (not cli/neo-go) and installs as `cli`
tools:
	go install github.com/nspcc-dev/neo-go/cli@$(NEOGO_VERSION)
	mv "$$(go env GOPATH)/bin/cli" "$$(go env GOPATH)/bin/neo-go"

## gofmt must be clean
fmt:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed in:"; echo "$$out"; exit 1; fi

## static analysis
vet:
	go vet ./...

## compile all contracts to build/ (requires neo-go on PATH)
build:
	sh scripts/compile.sh

## run every neotest and unit test
test:
	go test ./...

## the race detector over the whole suite (Go canon gate)
test-race:
	go test -race ./...

## dependency vulnerability scan against the pinned module set (Go canon Article 2 gate)
vuln:
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck missing: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	govulncheck ./...

## record/verify contract NEF hashes (QRN class D: reproducible build)
nefhash: build
	sh scripts/nef_determinism.sh
	sh scripts/nef_hash.sh

## deep static analysis (FTB-32): three independent analyzers beyond vet.
## staticcheck — full SA/U/ST/QF suite; gosec — security rules; deadcode — unreachable code.
## deadcode runs with -test (test-only APIs are reachable) and contract entry points
## (_deploy and the exported ABI methods) are filtered: the VM calls them, not Go.
deepcheck:
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck missing: go install honnef.co/go/tools/cmd/staticcheck@master"; exit 1; }
	@command -v gosec >/dev/null 2>&1 || { echo "gosec missing: go install github.com/securego/gosec/v2/cmd/gosec@master"; exit 1; }
	@command -v deadcode >/dev/null 2>&1 || { echo "deadcode missing: go install golang.org/x/tools/cmd/deadcode@latest"; exit 1; }
	staticcheck ./...
	gosec -quiet ./...
	@out="$$(deadcode -test ./... | grep -v '^contracts/')"; if [ -n "$$out" ]; then echo "deadcode:"; echo "$$out"; exit 1; fi
	@echo "deepcheck: staticcheck, gosec, deadcode clean"

## the Article 1 guarantee in one named gate: every check, every build
## (the Go canon's `make conformance` analog; a failure anywhere is a build failure)
conformance: fmt vet build test test-race vuln nefhash deepcheck
	@echo "conformance: all gates green"

clean:
	rm -rf build
