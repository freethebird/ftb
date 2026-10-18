// ratioswap commits the 18 Oct governance parameters on TestNet (DECISIONS.md):
// hard cap 100,000,000 FTB and the conversion ratio r = 100,000,000 ÷ 526,782,179.10
// = 0.189832 FTB per FLM/FLOCKS, applied by the migrator at mint time so the snapshot
// tree stays in FLM units and the genesis vote weighs exactly 1 FLM = 1 vote.
//
// The token is UPDATED IN PLACE through its owner-gated, pre-launch-only Update —
// its hash (0xdc82bb0c…) is referenced write-once by lend, dividends, bondvault and
// treasury, so a redeploy would orphan every module. Update preserves storage.
// The migrator is immutable, so the ratio math arrives as a redeploy (FtbMigratorR2),
// and the new question ("1 FLM/FLOCKS = 0.189832 FTB — adopt?") is a new ballot
// (FtbBallotR2): the published contract hash IS the proposal's identity.
//
//	go run ./cmd/ratioswap -config cmd/ratioswap/config.testnet.json -phase deploy # update + deploy + wire + propose
//	go run ./cmd/ratioswap -config cmd/ratioswap/config.testnet.json -phase apply  # after the timelock
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
	MerkleRootHex  string `json:"merkle_root_hex"` // the investigation root (0x9b4be5c9…), unchanged
	TimelockBlocks int    `json:"timelock_blocks"`
	BallotOptions  int    `json:"ballot_options"`
	Token          string `json:"token"`    // existing token, updated in place
	Registry       string `json:"registry"` // seam board, migrator entry rewired
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ratioswap:", err)
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

func load(dir, name string) (nef.File, *manifest.Manifest, []byte, []byte) {
	nb, err := os.ReadFile(filepath.Join(dir, name+".nef")) // #nosec G304 -- operator tool
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(dir, name+".manifest.json")) // #nosec G304 -- operator tool
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))
	return ne, m, nb, mb
}

func main() {
	cfgPath := flag.String("config", "config.json", "ratioswap config JSON")
	phase := flag.String("phase", "deploy", "deploy | apply")
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
		ne, m, _, _ := load(cfg.ContractsDir, name)
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
	verify := func(hash util.Uint160, method string, args ...smartcontract.Parameter) {
		// non-nil slice: a nil parameter list marshals to JSON null, which some RPC
		// servers reject ("failed to convert to script parameter") — [] is required.
		ps := append([]smartcontract.Parameter{}, args...)
		res, err := cli.InvokeFunction(hash, method, ps, nil)
		check(err)
		if res.State != "HALT" || len(res.Stack) == 0 {
			check(fmt.Errorf("verify %s: state=%s", method, res.State))
		}
		j, _ := json.Marshal(res.Stack[0])
		fmt.Printf("  verify %-22s -> %s\n", method, j)
	}

	if *phase == "deploy" {
		// 1. commit the 100M cap into the live token through its pre-launch Update;
		//    the hash — and every module bound to it — is preserved.
		_, _, nb, mb := load(cfg.ContractsDir, "ftb_token")
		call(token, "update", nb, mb, nil)
		verify(token, "maxSupply")

		// 2. the ratio-at-mint migrator and the ratio ballot (same root, unchanged tree)
		migrator := deploy("ftb_migrator_r2")
		call(migrator, "configure", token, root)
		verify(migrator, "ratioNum")
		verify(migrator, "ratioDen")

		var ballot util.Uint160
		if cfg.BallotOptions >= 2 {
			ballot = deploy("ballot_r2")
			call(ballot, "configure", root, cfg.BallotOptions)
		}

		// 3. the token's only minter moves to the R2 migrator
		call(token, "setMigrator", migrator)
		// 4. seam board: record the R2 migrator behind the timelock
		call(registry, "propose", "migrator", migrator)

		fmt.Printf("\nDEPLOY STAGED. token (updated in place) 0x%s\n", token.StringLE())
		fmt.Printf("new migrator 0x%s  new ballot 0x%s\n", migrator.StringLE(), ballot.StringLE())
		fmt.Printf("update web/config.js (migrator, ballot, ballot.question) after -phase apply lands.\n")
		fmt.Printf("apply becomes legal ~%d blocks after the propose tx; run -phase apply then.\n", cfg.TimelockBlocks)
		return
	}

	if *phase == "apply" {
		// single attempt: if the timelock ETA hasn't passed this faults (cheap, ~0.01
		// GAS) — check the propose tx height + timelock_blocks before running.
		call(registry, "apply", "migrator")

		verify(registry, "get", smartcontract.Parameter{Type: smartcontract.StringType, Value: "migrator"})
		verify(token, "maxSupply")
		return
	}

	check(fmt.Errorf("unknown phase %q: want deploy or apply", *phase))
}
