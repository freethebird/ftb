module github.com/freethebird/ftb/discord

go 1.26.0

// Pinned to a patched toolchain (matches the ftb contract module): go1.27.3
// carries the crypto/x509 std-lib fixes govulncheck flags via discordgo's TLS.
toolchain go1.27.3

require (
	github.com/bwmarrin/discordgo v0.29.0
	github.com/nspcc-dev/neo-go v0.123.0
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/decred/dcrd/crypto/ripemd160 v1.0.2 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/golang/snappy v0.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mr-tron/base58 v1.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nspcc-dev/bbolt v0.0.0-20260404200350-24f70ceb2bd9 // indirect
	github.com/nspcc-dev/go-ordered-json v0.0.0-20260826165212-78b9a86d413c // indirect
	github.com/nspcc-dev/hrw/v2 v2.0.4 // indirect
	github.com/nspcc-dev/neofs-sdk-go v1.0.0-rc.21 // indirect
	github.com/nspcc-dev/rfc6979 v0.2.4 // indirect
	github.com/nspcc-dev/tzhash v1.8.4 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20210305035536-64b5b1c73954 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/exp v0.0.0-20250911091902-df9299821621 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
