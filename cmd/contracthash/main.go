// contracthash prints the Neo N3 contract hash a compiled NEF will have when
// deployed by a given address: hash = f(sender, nef checksum, manifest name).
// Needed BEFORE deploying when one contract's constants must name another
// (the AMM router names the factory; the pair names the whitelist).
//
//	go run ./cmd/contracthash <deployer-address> <file.nef> [more.nef...]
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: contracthash <deployer-address> <file.nef> [more.nef...]")
		os.Exit(2)
	}
	sender, err := address.StringToUint160(os.Args[1])
	check(err)
	for _, path := range os.Args[2:] {
		// #nosec G703 G304 -- CLI tool: reading operator-supplied NEF paths from argv
		// is the entire function of this command; there is no other input source.
		raw, err := os.ReadFile(path)
		check(err)
		n, err := nef.FileFromBytes(raw)
		check(err)
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		h := state.CreateContractHash(sender, n.Checksum, name)
		fmt.Printf("0x%s  %s  (checksum %d, name %s)\n", h.StringLE(), path, n.Checksum, name)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
