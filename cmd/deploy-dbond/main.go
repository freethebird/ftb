// deploy-dbond deploys and wires the FTB Discord-bond pair — FtbDiscordBond
// (dbondvault) + FtbBondAccusation (accusation) — on Neo N3. It deploys ONLY
// these two contracts; it never touches the core, the governance ballot, or the
// Launch-2 suite. Both are configured against the live FTB token.
//
//	go run ./cmd/deploy-dbond -config cmd/deploy-dbond/config.testnet.json
//
// Pre-req: `make build` (scripts/compile.sh) has placed dbondvault.nef/.manifest
// and accusation.nef/.manifest in contracts_dir. Written to the neo-go v0.123 SDK,
// same pattern as cmd/deploy.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	FTB          string `json:"ftb"`           // live FTB token hash (0x canonical)
	Governance   string `json:"governance"`    // N-address or 0x; gates setParams only. Empty => deployer.
	Burn         string `json:"burn"`          // unspendable sink (0x canonical)
	BondAmount   int64  `json:"bond_amount"`   // base units (1000 FTB = 100000000000)
	BanBps       int64  `json:"ban_bps"`       // 2000 = 20%
	VotingWindow int64  `json:"voting_window"` // blocks
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy-dbond config JSON")
	flag.Parse()

	var cfg config
	raw, err := os.ReadFile(*cfgPath) // #nosec G304 -- operator tool: path is the operator's own CLI arg
	check(err)
	check(json.Unmarshal(raw, &cfg))

	ftb := mustHash("ftb", cfg.FTB)
	burn := mustHash("burn", cfg.Burn)
	if cfg.BondAmount <= 0 || cfg.BanBps <= 0 || cfg.BanBps > 10000 || cfg.VotingWindow <= 0 {
		check(fmt.Errorf("bad params: bond_amount=%d ban_bps=%d voting_window=%d",
			cfg.BondAmount, cfg.BanBps, cfg.VotingWindow))
	}

	cli, err := rpcclient.New(context.Background(), cfg.RPC, rpcclient.Options{})
	check(err)
	check(cli.Init())

	w, err := wallet.NewWalletFromFile(cfg.Wallet)
	check(err)
	acc := w.Accounts[0]
	check(acc.Decrypt(resolvePassword(cfg), w.Scrypt))
	sender := acc.ScriptHash()

	owner := sender // single-key TestNet: the deployer is the write-once configure caller
	governance := sender
	if strings.TrimSpace(cfg.Governance) != "" {
		governance = mustAccount(cfg.Governance)
	}

	// The FTB token must actually exist on this chain before we wire a vault to it.
	if _, err := cli.GetContractStateByAddressOrName("0x" + ftb.StringLE()); err != nil {
		check(fmt.Errorf("FTB token 0x%s not found on this RPC — wrong token or wrong chain: %w", ftb.StringLE(), err))
	}

	act, err := actor.NewSimple(cli, acc)
	check(err)
	mgmt := management.New(act)

	getState := func(h util.Uint160) (*state.Contract, error) {
		return cli.GetContractStateByAddressOrName("0x" + h.StringLE())
	}
	deploy := func(name string, data any) util.Uint160 {
		ne, m := load(cfg.ContractsDir, name)
		h := state.CreateContractHash(sender, ne.Checksum, m.Name)
		if _, err := getState(h); err == nil {
			fmt.Printf("reusing  %-14s -> 0x%s (already on-chain)\n", name, h.StringLE())
			return h
		}
		txh, vub, derr := mgmt.Deploy(&ne, m, data)
		check(derr)
		_, derr = act.WaitSuccess(context.Background(), txh, vub, derr)
		check(derr)
		if _, derr := getState(h); derr != nil {
			check(fmt.Errorf("deploy of %s HALTed but 0x%s not found on-chain: %w", name, h.StringLE(), derr))
		}
		fmt.Printf("deployed %-14s -> 0x%s  tx 0x%s\n", name, h.StringLE(), txh.StringLE())
		return h
	}
	call := func(h util.Uint160, method string, args ...any) {
		txh, vub, cerr := act.SendCall(h, method, args...)
		check(cerr)
		_, cerr = act.WaitSuccess(context.Background(), txh, vub, cerr)
		check(cerr)
		fmt.Printf("  wired %-10s tx 0x%s\n", method, txh.StringLE())
	}

	fmt.Printf("deployer/owner: %s (0x%s)\n", address.Uint160ToString(sender), sender.StringLE())
	fmt.Printf("governance:     %s (0x%s)\n", address.Uint160ToString(governance), governance.StringLE())
	fmt.Printf("ftb:            0x%s\n", ftb.StringLE())
	fmt.Printf("params:         bond=%d banBps=%d window=%d burn=0x%s\n",
		cfg.BondAmount, cfg.BanBps, cfg.VotingWindow, burn.StringLE())

	// Deploy the pair (deploy data = owner; _deploy stores it for write-once configure).
	vault := deploy("dbondvault", owner.BytesBE())
	ballot := deploy("accusation", owner.BytesBE())

	// Configure (owner-witnessed; this wallet is the owner).
	call(vault, "configure", ftb, ballot, governance, burn, cfg.BondAmount, cfg.BanBps, cfg.VotingWindow)
	call(ballot, "configure", vault)

	fmt.Println("\nDiscord-bond pair deployed and wired.")
	fmt.Printf("DBOND_HASHES {\"dbondvault\":\"0x%s\",\"accusation\":\"0x%s\",\"ftb\":\"0x%s\"}\n",
		vault.StringLE(), ballot.StringLE(), ftb.StringLE())
	fmt.Println("\nWire the bots: set in config.json")
	fmt.Printf("  contracts.vault_hash  = 0x%s\n", vault.StringLE())
	fmt.Printf("  contracts.ballot_hash = 0x%s\n", ballot.StringLE())
	fmt.Printf("  neo.ftb_token_hash    = 0x%s\n", ftb.StringLE())
	fmt.Println("then: systemctl restart ftb-verifybot ftb-sentinelbot")
}

// ---- helpers (mirror cmd/deploy) ----

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

func mustAccount(s string) util.Uint160 {
	s = strings.TrimSpace(s)
	var (
		h   util.Uint160
		err error
	)
	if strings.HasPrefix(s, "N") {
		h, err = address.StringToUint160(s)
	} else {
		h, err = util.Uint160DecodeStringLE(strip0x(s))
	}
	check(err)
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
