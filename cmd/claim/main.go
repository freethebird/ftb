// claim submits a Merkle proof to the FTB migrator to mint a wallet's FTB
// entitlement. Claim asserts CheckWitness(account), so it is signed by the
// claiming wallet. The account's amount + proof are read from the proofs JSON
// the front end uses; the proof is folded to the root locally first, so no GAS
// is spent on a bad proof. Optionally transfers part of the minted FTB onward
// (-to / -amount), so one command can claim and fund another wallet (for
// example the AMM seed wallet). Reusable for the MainNet treasury/LP claim.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/claim -wallet <claimant.wallet.json> \
//	  -proofs web/proofs_v4.json [-account N...] [-to N... -amount 1500]
//
// Pinned to neo-go v0.123; N3 addresses are network-agnostic. Amount is an exact
// decimal string scaled by 8 (no float on the money path), same as cmd/sendtoken.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

const (
	migratorHash = "0x08c02e1887b652f9b1cb6a17c668fc9af28047ac"
	ftbTokenHash = "0xce37caa3f6c0ac6d4cbd82b14ccb8ec8740f3e43" // #nosec G101 -- public Neo N3 contract hash, not a credential
	rootHex      = "235e7118608baf1880ef0fd50ed291900cbff26f6447725765403f85da1e75b4"
	ftbDecimals  = 8
)

type entry struct {
	Address string   `json:"address"`
	Amount  string   `json:"amount"`
	Proof   []string `json:"proof"`
}
type proofsFile struct {
	Root    string  `json:"root"`
	Entries []entry `json:"entries"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func mustHash(s string) util.Uint160 {
	s = strings.TrimPrefix(s, "0x")
	h, err := util.Uint160DecodeStringLE(s)
	check(err)
	return h
}

func sha(b []byte) []byte { s := sha256.Sum256(b); return s[:] }

// fold recomputes the Merkle root from a leaf: leaf = sha256(0x00 ++ account ++ ':' ++ amount),
// internal nodes = sha256(0x01 ++ min ++ max). Mirrors FtbMigrator.Claim exactly.
func fold(account []byte, amount string, proof [][]byte) []byte {
	li := append([]byte{0x00}, account...)
	li = append(li, ':')
	li = append(li, []byte(amount)...)
	node := sha(li)
	for _, sib := range proof {
		if bytes.Compare(node, sib) < 0 {
			node = sha(append([]byte{0x01}, append(node, sib...)...))
		} else {
			node = sha(append([]byte{0x01}, append(sib, node...)...))
		}
	}
	return node
}

// toBaseUnits parses an exact decimal string into 8-decimal base units (no float).
func toBaseUnits(dec string) (*big.Int, error) {
	dec = strings.TrimSpace(dec)
	neg := strings.HasPrefix(dec, "-")
	if neg {
		return nil, fmt.Errorf("amount must be positive")
	}
	intPart, fracPart := dec, ""
	if i := strings.IndexByte(dec, '.'); i >= 0 {
		intPart, fracPart = dec[:i], dec[i+1:]
	}
	if len(fracPart) > ftbDecimals {
		return nil, fmt.Errorf("amount has more than %d decimal places", ftbDecimals)
	}
	for len(fracPart) < ftbDecimals {
		fracPart += "0"
	}
	whole := intPart + fracPart
	v, ok := new(big.Int).SetString(whole, 10)
	if !ok {
		return nil, fmt.Errorf("bad amount %q", dec)
	}
	return v, nil
}

func main() {
	rpcURL := flag.String("rpc", "https://testnet1.neo.coz.io:443", "RPC")
	walletPath := flag.String("wallet", "cmd/deploy/testnet.wallet.json", "claimant wallet (NEP-6)")
	passEnv := flag.String("passenv", "FTB_DEPLOY_PASS", "env var holding the wallet password")
	proofsPath := flag.String("proofs", "web/proofs_v4.json", "proofs JSON (entries[].address/amount/proof)")
	acctFlag := flag.String("account", "", "address to claim for (default: the wallet's account)")
	toFlag := flag.String("to", "", "optional: transfer part of the minted FTB to this address")
	amtFlag := flag.String("amount", "", "optional: FTB amount (decimal) to transfer to -to")
	migFlag := flag.String("migrator", migratorHash, "FtbMigrator hash")
	ftbFlag := flag.String("ftb", ftbTokenHash, "FTB token hash")
	rootFlag := flag.String("root", rootHex, "expected Merkle root (self-check)")
	flag.Parse()

	w, err := wallet.NewWalletFromFile(*walletPath)
	check(err)
	acc := w.Accounts[0]
	pass := os.Getenv(*passEnv)
	if pass == "" {
		check(fmt.Errorf("%s is empty", *passEnv))
	}
	check(acc.Decrypt(pass, w.Scrypt))

	account := acc.ScriptHash()
	if *acctFlag != "" {
		account, err = address.StringToUint160(*acctFlag)
		check(err)
	}

	// locate the entry
	pb, err := os.ReadFile(*proofsPath) // #nosec G304 -- operator-supplied proofs file
	check(err)
	var pf proofsFile
	check(json.Unmarshal(pb, &pf))
	want := address.Uint160ToString(account)
	var e *entry
	for i := range pf.Entries {
		if pf.Entries[i].Address == want {
			e = &pf.Entries[i]
			break
		}
	}
	if e == nil {
		check(fmt.Errorf("account %s not found in %s", want, *proofsPath))
	}
	proof := make([][]byte, len(e.Proof))
	for i, h := range e.Proof {
		b, err := hex.DecodeString(h)
		check(err)
		if len(b) != 32 {
			check(fmt.Errorf("proof[%d] is %d bytes, want 32", i, len(b)))
		}
		proof[i] = b
	}

	// local self-check: the proof must fold to the root with the same account bytes
	// the contract will see. Try BE then LE; the matching order is canonical.
	root, err := hex.DecodeString(strings.TrimPrefix(*rootFlag, "0x"))
	check(err)
	var ok bool
	for _, ab := range [][]byte{account.BytesBE(), account.BytesLE()} {
		if bytes.Equal(fold(ab, e.Amount, proof), root) {
			ok = true
			break
		}
	}
	if !ok {
		check(fmt.Errorf("proof for %s does not fold to root %s (bad proofs file or amount)", want, *rootFlag))
	}
	amount, okAmt := new(big.Int).SetString(e.Amount, 10)
	if !okAmt {
		check(fmt.Errorf("bad amount %q", e.Amount))
	}
	fmt.Printf("proof OK: %s entitlement %s folds to root\n", want, e.Amount)

	cli, err := rpcclient.New(context.Background(), *rpcURL, rpcclient.Options{})
	check(err)
	check(cli.Init())
	act, err := actor.NewSimple(cli, acc)
	check(err)

	// claim: signed by the account (CheckWitness(account)); mints Quote(amount) FTB to it
	txh, vub, err := act.SendCall(mustHash(*migFlag), "claim", account, amount, proof)
	check(err)
	_, err = act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("claimed: FTB minted to %s  tx 0x%s\n", want, txh.StringLE())

	// optional onward transfer, so one command funds the seed wallet
	if *toFlag != "" || *amtFlag != "" {
		if *toFlag == "" || *amtFlag == "" {
			check(fmt.Errorf("-to and -amount must be given together"))
		}
		to, err := address.StringToUint160(*toFlag)
		check(err)
		amt, err := toBaseUnits(*amtFlag)
		check(err)
		txh, vub, err := act.SendCall(mustHash(*ftbFlag), "transfer", account, to, amt, nil)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("transferred %s FTB -> %s  tx 0x%s\n", *amtFlag, *toFlag, txh.StringLE())
	}
}
