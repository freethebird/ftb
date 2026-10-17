// rootswap moves the TestNet migrator and ballot from the OLD snapshot root
// (0x639aaa78…, 4,847 wallets) to the corrected investigation root
// (0x9b4be5c9…, 4,830 wallets — see findings/ and DECISIONS.md). Both contracts bind
// the root write-once at configure, so the swap is a redeploy under renamed manifests
// (FtbMigratorR1 / FtbBallotR1 — the N3 hash is sender+nef+name, so a rename is what
// makes the redeploy legal) plus rewiring: the token's minter moves to the new
// migrator, and the seam board's "migrator" entry follows through the timelock.
//
//	go run ./cmd/rootswap -config cmd/rootswap/config.testnet.json -phase swap   # deploy + wire + propose
//	go run ./cmd/rootswap -config cmd/rootswap/config.testnet.json -phase apply  # after the timelock
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
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type config struct {
	RPC            string `json:"rpc"`
	Wallet         string `json:"wallet"`
	PasswordEnv    string `json:"password_env"`
	ContractsDir   string `json:"contracts_dir"`
	MerkleRootHex  string `json:"merkle_root_hex"` // the NEW root (0x9b4be5c9…)
	TimelockBlocks int    `json:"timelock_blocks"`
	BallotOptions  int    `json:"ballot_options"`
	Token          string `json:"token"`
	Registry       string `json:"registry"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "rootswap:", err)
		os.Exit(1)
	}
}

func strip0x(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}

func mustHash(what, s string) util.Uint160 {
	h, err := util.Uint160DecodeStringLE(strip0x(s))
	if err != nil {
		check(fmt.Errorf("%s: %w", what, err))
	}
	return h
}

func load(dir, name string) (nef.File, *manifest.Manifest) {
	nb, err := os.ReadFile(filepath.Join(dir, name+".nef")) // #nosec G304 -- operator tool
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(dir, name+".manifest.json")) // #nosec G304 -- operator tool
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))
	return ne, m
}

func main() {
	cfgPath := flag.String("config", "config.json", "rootswap config JSON")
	phase := flag.String("phase", "swap", "swap | apply")
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
	token := mustHash("token", cfg.Token)
	registry := mustHash("registry", cfg.Registry)

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

	deploy := func(name string) util.Uint160 {
		ne, m := load(cfg.ContractsDir, name)
		h := state.CreateContractHash(sender, ne.Checksum, m.Name)
		txh, vub, err := mgmt.Deploy(&ne, m, sender.BytesBE())
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("deployed %-16s -> 0x%s  tx 0x%s\n", name, h.StringLE(), txh.StringLE())
		return h
	}
	call := func(hash util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(hash, method, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-22s tx 0x%s\n", method, txh.StringLE())
	}

	if *phase == "swap" {
		migrator := deploy("ftb_migrator_r1")
		call(migrator, "configure", token, root)

		var ballot util.Uint160
		if cfg.BallotOptions >= 2 {
			ballot = deploy("ballot_r1")
			call(ballot, "configure", root, cfg.BallotOptions)
		}

		// the token's only minter moves to the new migrator
		call(token, "setMigrator", migrator)
		// seam board: record the new migrator behind the timelock
		call(registry, "propose", "migrator", migrator)

		fmt.Printf("\nSWAP STAGED. new migrator 0x%s  new ballot 0x%s\n", migrator.StringLE(), ballot.StringLE())
		fmt.Printf("update web/config.js (migrator, ballot, merkleRoot) after -phase apply lands.\n")
		fmt.Printf("apply becomes legal ~%d blocks after the propose tx; run -phase apply then.\n", cfg.TimelockBlocks)
		return
	}

	if *phase == "apply" {
		// single attempt: if the timelock ETA hasn't passed this faults (cheap, ~0.01
		// GAS) — check the propose tx height + timelock_blocks before running.
		call(registry, "apply", "migrator")

		// verify: the seam board must now resolve "migrator" to the R1 deployment.
		res, err := cli.InvokeFunction(registry, "get", []smartcontract.Parameter{
			{Type: smartcontract.StringType, Value: "migrator"},
		}, nil)
		check(err)
		if res.State != "HALT" || len(res.Stack) == 0 {
			check(fmt.Errorf("verify: registry.get(migrator) state=%s", res.State))
		}
		j, _ := json.Marshal(res.Stack[0])
		fmt.Printf("  verify registry.get(migrator) -> %s\n", j)
		return
	}

	check(fmt.Errorf("unknown phase %q: want swap or apply", *phase))
}
