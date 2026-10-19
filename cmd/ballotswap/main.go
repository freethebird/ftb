// ballotswap redeploys the genesis ballot on TestNet as FtbBallotR3, bound to the v2
// claim tree (root 0x9a36b6a0…, 5,194 wallets incl. the dev wallet NQzqB9AB…CwRD and the
// salvage sets). R2's Configure is write-once against the 4,830-wallet electorate
// (DECISIONS.md, Decision 10), so letting the dev team vote in the TestNet dry-run of the
// genesis ballot means a redeploy under a renamed manifest — bytecode byte-identical to
// R2. TestNet tallies are test votes; nothing real is reset. The MainNet electorate is a
// separate, still-open decision (see DECISIONS.md, Decision 12).
//
//	FTB_DEPLOY_PASS=... go run ./cmd/ballotswap -config cmd/ballotswap/config.testnet.json
//
// Same SDK caveats as cmd/deploy: pinned to neo-go v0.123, canonical 0x hashes are
// StringLE(), strings decode with Uint160DecodeStringLE.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/management"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type config struct {
	RPC           string `json:"rpc"`
	Wallet        string `json:"wallet"`
	PasswordEnv   string `json:"password_env"`
	ContractsDir  string `json:"contracts_dir"`
	MerkleRootHex string `json:"merkle_root_hex"` // the v2 root (0x9a36b6a0…)
	NumOptions    int    `json:"num_options"`     // same question as R2: the ratio vote, 2 options
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ballotswap:", err)
		os.Exit(1)
	}
}

func strip0x(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}

func main() {
	cfgPath := flag.String("config", "config.json", "ballotswap config JSON")
	flag.Parse()

	var cfg config
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(json.Unmarshal(raw, &cfg))

	root, err := hex.DecodeString(strip0x(cfg.MerkleRootHex))
	check(err)
	if len(root) != 32 {
		check(fmt.Errorf("merkle root must be 32 bytes, got %d", len(root)))
	}

	cli, err := rpcclient.New(context.Background(), cfg.RPC, rpcclient.Options{})
	check(err)
	check(cli.Init())

	w, err := wallet.NewWalletFromFile(cfg.Wallet)
	check(err)
	acc := w.Accounts[0]
	pass := os.Getenv(cfg.PasswordEnv)
	if pass == "" {
		check(fmt.Errorf("password_env %q is empty in the environment", cfg.PasswordEnv))
	}
	check(acc.Decrypt(pass, w.Scrypt))
	sender := acc.ScriptHash()

	act, err := actor.NewSimple(cli, acc)
	check(err)
	mgmt := management.New(act)

	nb, err := os.ReadFile(filepath.Join(cfg.ContractsDir, "ballot_r3.nef")) // #nosec G304 -- operator tool
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(cfg.ContractsDir, "ballot_r3.manifest.json")) // #nosec G304
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))

	h := state.CreateContractHash(sender, ne.Checksum, m.Name)
	txh, vub, err := mgmt.Deploy(&ne, m, sender.BytesBE())
	check(err)
	_, err = act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("deployed ballot_r3 -> 0x%s  tx 0x%s\n", h.StringLE(), txh.StringLE())

	txh, vub, err = act.SendCall(h, "configure", root, cfg.NumOptions)
	check(err)
	_, err = act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("  wired %-22s tx 0x%s\n", "configure", txh.StringLE())

	fmt.Printf("\nBALLOT R3 LIVE. new ballot 0x%s (root 0x%s, %d options)\n",
		h.StringLE(), hex.EncodeToString(root), cfg.NumOptions)
	fmt.Printf("update web/config.testnet.js + web/config.js (ballot) and redeploy the front end.\n")
}
