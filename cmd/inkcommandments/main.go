// inkcommandments deploys FtbCommandments, inks the three genesis "stone commandments"
// (the decided TestNet governance votes that set the MainNet launch terms), then renounces
// ownership so the record is frozen forever. It deploys ONLY this one contract; it never
// touches the core, the ballots, or the Launch-2 suite.
//
//	go run ./cmd/inkcommandments -config cmd/inkcommandments/config.testnet.json
//
// Pre-req: `make build` has placed ftb_commandments.nef/.manifest in contracts_dir.
// After it prints FTB_COMMANDMENTS, set that hash as claim_window_proposal in the deploy
// config and as the governance record the front end reads. Written to the neo-go v0.123 SDK,
// same pattern as cmd/deploy-dbond.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/management"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type config struct {
	RPC          string `json:"rpc"`
	Wallet       string `json:"wallet"`
	Password     string `json:"password"`     // fallback; prefer password_env
	PasswordEnv  string `json:"password_env"` // env var holding the wallet password
	ContractsDir string `json:"contracts_dir"`
}

// genesis is one stone commandment: a decided vote, recorded verbatim with its final tally.
// These are FIXED FACTS (the TestNet genesis governance, closed 31 Oct 2026) -- the tallies
// were read directly off each ballot contract. They are baked in, not configurable.
type genesis struct {
	number   int
	question string
	ballot   string // source ballot, 0x canonical (StringLE)
	options  []any
	tallies  []any // final snapshot weight behind each option, base units (1e8)
	winner   int   // 1-based
	ratified string
}

var commandments = []genesis{
	{
		number:   1,
		question: "1 FLM/FLOCKS = 0.189832 FTB - adopt? (100M cap, pro-rata)",
		ballot:   "0x2f76d354c1345946b0dc22d189724b71c7539690",
		options:  []any{"adopt", "wait"},
		tallies:  []any{int64(24476310508175981), int64(0)},
		winner:   1,
		ratified: "adopted: 100M cap, pro-rata; rate finalized 0.173173 (v4 tree, incl. open-order broker FLM; cap unchanged)",
	},
	{
		number:   2,
		question: "MainNet launch date?",
		ballot:   "0xd9b19731ca159f956ab71e6d42dec7fef28a72e9",
		options:  []any{"1 November", "7 November"},
		tallies:  []any{int64(24289105916503609), int64(387068261480261)},
		winner:   1,
		ratified: "1 November 2026 (actual deploy block recorded on-chain)",
	},
	{
		number:   3,
		question: "FTB claim window - how long can holders claim?",
		ballot:   "0xc62269897f6747d52c90ca8fe6b9c9eb02d7f9ad",
		options:  []any{"6 months", "1 year"},
		tallies:  []any{int64(24603980714372870), int64(72193463611000)},
		winner:   1,
		ratified: "6 months",
	},
}

func main() {
	cfgPath := flag.String("config", "config.json", "inkcommandments config JSON")
	flag.Parse()

	var cfg config
	raw, err := os.ReadFile(*cfgPath) // #nosec G304 -- operator tool: path is the operator's own CLI arg
	check(err)
	check(json.Unmarshal(raw, &cfg))

	cli, err := rpcclient.New(context.Background(), cfg.RPC, rpcclient.Options{RequestTimeout: 60 * time.Second})
	check(err)
	check(cli.Init())

	w, err := wallet.NewWalletFromFile(cfg.Wallet)
	check(err)
	acc := w.Accounts[0]
	check(acc.Decrypt(resolvePassword(cfg), w.Scrypt))
	sender := acc.ScriptHash()

	act, err := actor.NewSimple(cli, acc)
	check(err)
	mgmt := management.New(act)

	getState := func(h util.Uint160) (*state.Contract, error) {
		return cli.GetContractStateByAddressOrName("0x" + h.StringLE())
	}
	deploy := func(name string, data any) util.Uint160 {
		ne, m := load(cfg.ContractsDir, name)
		h := state.CreateContractHash(sender, ne.Checksum, m.Name)
		if _, derr := getState(h); derr == nil {
			fmt.Printf("reusing  %-16s -> 0x%s (already on-chain)\n", name, h.StringLE())
			return h
		}
		txh, vub, derr := mgmt.Deploy(&ne, m, data)
		check(derr)
		_, derr = act.WaitSuccess(context.Background(), txh, vub, derr)
		check(derr)
		if _, derr := getState(h); derr != nil {
			check(fmt.Errorf("deploy of %s HALTed but 0x%s not found on-chain: %w", name, h.StringLE(), derr))
		}
		fmt.Printf("deployed %-16s -> 0x%s  tx 0x%s\n", name, h.StringLE(), txh.StringLE())
		return h
	}
	call := func(h util.Uint160, method string, args ...any) {
		txh, vub, cerr := act.SendCall(h, method, args...)
		check(cerr)
		_, cerr = act.WaitSuccess(context.Background(), txh, vub, cerr)
		check(cerr)
		fmt.Printf("  %-18s tx 0x%s\n", method, txh.StringLE())
	}

	fmt.Printf("deployer/owner: %s (0x%s)\n", address.Uint160ToString(sender), sender.StringLE())

	cmd := deploy("ftb_commandments", sender.BytesBE())

	for _, g := range commandments {
		ballot := mustHash("ballot", g.ballot)
		fmt.Printf("ink %d: %s -> %s\n", g.number, g.question, g.ratified)
		call(cmd, "ink", g.number, g.question, ballot, g.options, g.tallies, g.winner, g.ratified)
	}

	// Freeze the record forever: no more Ink, no owner. The commandments stand in stone.
	call(cmd, "renounceOwnership")

	fmt.Println("\nGenesis commandments inked and the record renounced (immutable).")
	fmt.Printf("FTB_COMMANDMENTS 0x%s\n", cmd.StringLE())
	fmt.Println("\nNext: in the deploy config set")
	fmt.Printf("  \"claim_window_proposal\": \"0x%s\"\n", cmd.StringLE())
	fmt.Println("and point the front-end governance record at the same hash.")
}

// ---- helpers (mirror cmd/deploy-dbond) ----

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

func strip0x(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}

func mustHash(field, s string) util.Uint160 {
	h, err := util.Uint160DecodeStringLE(strip0x(strings.TrimSpace(s)))
	if err != nil {
		check(fmt.Errorf("%s: bad hash %q: %w", field, s, err))
	}
	return h
}

func resolvePassword(cfg config) string {
	if cfg.PasswordEnv != "" {
		p := os.Getenv(cfg.PasswordEnv)
		if p == "" {
			check(fmt.Errorf("password_env %q set but empty in environment", cfg.PasswordEnv))
		}
		return p
	}
	return cfg.Password
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
