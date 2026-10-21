// rootswap3 wires the v2 claim tree (data/migration/ftb_claim_list_v2.csv — salvage pass, whale-cluster
// flags, unflagged #8, and the dev wallet NQzqB9AB…CwRD; root 0x9a36b6a0…, 5,194 wallets,
// 526,782,179.10 FLM-equiv = ratioDen exactly) onto TestNet. The migrator binds its root
// write-once at configure, so this is a redeploy under a renamed manifest (FtbMigratorR3 —
// bytecode byte-identical to R2) plus rewiring: the token's minter moves to R3 and the seam
// board's "migrator" entry follows through the timelock. The ballot is NOT touched: the
// genesis-vote electorate stays the 4,830-wallet tree per DECISIONS.md (18 Oct).
//
//	FTB_DEPLOY_PASS=... go run ./cmd/rootswap3 -config cmd/rootswap3/config.testnet.json -phase swap
//	# ~20 blocks later:
//	FTB_DEPLOY_PASS=... go run ./cmd/rootswap3 -config cmd/rootswap3/config.testnet.json -phase apply
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
	MerkleRootHex  string `json:"merkle_root_hex"` // the v2 root (0x9a36b6a0…)
	TimelockBlocks int    `json:"timelock_blocks"`
	Token          string `json:"token"`
	Registry       string `json:"registry"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "rootswap3:", err)
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

func main() {
	cfgPath := flag.String("config", "config.json", "rootswap3 config JSON")
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

	if *phase == "swap" {
		nb, err := os.ReadFile(filepath.Join(cfg.ContractsDir, "ftb_migrator_r3.nef")) // #nosec G304 -- operator tool
		check(err)
		ne, err := nef.FileFromBytes(nb)
		check(err)
		mb, err := os.ReadFile(filepath.Join(cfg.ContractsDir, "ftb_migrator_r3.manifest.json")) // #nosec G304
		check(err)
		m := &manifest.Manifest{}
		check(json.Unmarshal(mb, m))

		h := state.CreateContractHash(sender, ne.Checksum, m.Name)
		txh, vub, err := mgmt.Deploy(&ne, m, sender.BytesBE())
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		migrator := h
		fmt.Printf("deployed ftb_migrator_r3 -> 0x%s  tx 0x%s\n", h.StringLE(), txh.StringLE())

		call := func(hash util.Uint160, method string, args ...any) {
			txh, vub, err := act.SendCall(hash, method, args...)
			check(err)
			_, err = act.WaitSuccess(context.Background(), txh, vub, err)
			check(err)
			fmt.Printf("  wired %-22s tx 0x%s\n", method, txh.StringLE())
		}

		call(migrator, "configure", token, root)
		// the token's only minter moves to the new migrator (old root's claims stop;
		// every R2 wallet is also in the v2 tree, so no TestNet claimer is orphaned)
		call(token, "setMigrator", migrator)
		// seam board: record the new migrator behind the timelock
		call(registry, "propose", "migrator", migrator)

		fmt.Printf("\nSWAP STAGED. new migrator 0x%s\n", migrator.StringLE())
		fmt.Printf("update web/config.js (migrator, merkleRoot) after -phase apply lands.\n")
		fmt.Printf("apply becomes legal ~%d blocks after the propose tx; run -phase apply then.\n", cfg.TimelockBlocks)
		return
	}

	if *phase == "apply" {
		txh, vub, err := act.SendCall(registry, "apply", "migrator")
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-22s tx 0x%s\n", "apply", txh.StringLE())

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
