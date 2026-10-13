NEOGO_VERSION ?= v0.123.0

.PHONY: all tools fmt vet build test test-race vuln nefhash conformance clean

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
	sh scripts/nef_hash.sh

## the Article 1 guarantee in one named gate: every check, every build
## (the Go canon's `make conformance` analog; a failure anywhere is a build failure)
conformance: fmt vet build test test-race vuln nefhash
	@echo "conformance: all gates green"

clean:
	rm -rf build
