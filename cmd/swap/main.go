// swap replaces ONE live Launch-2 module behind the registry timelock — the designed
// seam (registry.go: modules resolve each other by name, so a module can be swapped
// without touching its peers). It exists because cmd/deploy is all-or-nothing: its
// wire phase re-calls Configure on every module, and Configure is write-once, so a
// single-module fix cannot reuse it. cmd/update cannot help either when the deployed
// module predates the Update method (the v1 lend did — see DECISIONS.md).
//
//	FTB_DEPLOY_PASS=... go run ./cmd/swap -config cmd/deploy/config.testnet.json -module lend
//	# wait out the timelock, then (apply is permissionless, anyone can run it):
//	go run ./cmd/swap -config cmd/deploy/config.testnet.json -module lend -apply
//
// Steps: deploy build/<module>.{nef,manifest.json} (hash is deterministic on
// sender+nef+name, so a re-run after a crash reuses the on-chain contract), run the
// module's post-deploy wiring, propose the new hash on the registry, and with -apply
// apply the proposal once the timelock has elapsed. Old storage stays in the old
// contract — a swap is a fresh module, not a migration.
//
// Written to the neo-go v0.123 SDK. Single-key TestNet path only: owner = deploy
// wallet (the same posture cmd/deploy prints a warning for).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	RPC          string `json:"rpc"`
	Wallet       string `json:"wallet"`
	Password     string `json:"password"`
	PasswordEnv  string `json:"password_env"`
	ContractsDir string `json:"contracts_dir"`
	Core         *struct {
		FTB      string `json:"ftb"`
		Registry string `json:"registry"`
	} `json:"core"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "swap:", err)
		os.Exit(1)
	}
}

func mustHash(label, s string) util.Uint160 {
	h, err := util.Uint160DecodeStringLE(strings.TrimPrefix(strings.TrimSpace(s), "0x"))
	if err != nil {
		check(fmt.Errorf("%s: bad hash %q: %w", label, s, err))
	}
	return h
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy config JSON (same shape as cmd/deploy)")
	module := flag.String("module", "", "module to swap (today: lend)")
	apply := flag.Bool("apply", false, "skip deploy/wire/propose; apply the pending registry proposal")
	flag.Parse()
	if *module == "" {
		fmt.Fprintln(os.Stderr, "need -module")
		os.Exit(2)
	}

	var cfg config
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(json.Unmarshal(raw, &cfg))
	if cfg.Core == nil {
		check(fmt.Errorf("config needs the core section (ftb, registry hashes)"))
	}
	token := mustHash("core.ftb", cfg.Core.FTB)
	registry := mustHash("core.registry", cfg.Core.Registry)

	cli, err := rpcclient.New(context.Background(), cfg.RPC, rpcclient.Options{})
	check(err)
	check(cli.Init())

	w, err := wallet.NewWalletFromFile(cfg.Wallet)
	check(err)
	acc := w.Accounts[0]
	pass := cfg.Password
	if cfg.PasswordEnv != "" {
		pass = os.Getenv(cfg.PasswordEnv)
		if pass == "" {
			check(fmt.Errorf("password_env %q set but empty in the environment", cfg.PasswordEnv))
		}
	}
	check(acc.Decrypt(pass, w.Scrypt))
	sender := acc.ScriptHash()

	act, err := actor.NewSimple(cli, acc)
	check(err)

	call := func(hash util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(hash, method, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-16s tx 0x%s\n", method, txh.StringLE())
	}

	if *apply {
		// Permissionless once the timelock elapsed; anyone can send this.
		call(registry, "apply", *module)
		cur, err := cli.InvokeFunction(registry, "get",
			[]smartcontract.Parameter{{Type: smartcontract.StringType, Value: *module}}, nil)
		check(err)
		if cur.State != "HALT" || len(cur.Stack) == 0 {
			check(fmt.Errorf("registry.get(%q) did not HALT cleanly: %s", *module, cur.State))
		}
		got := cur.Stack[0].Value()
		if b, ok := got.([]byte); ok && len(b) == 20 {
			u, err := util.Uint160DecodeBytesBE(b)
			if err == nil {
				got = "0x" + u.StringLE() // canonical form, same convention as deploy output
			}
		}
		fmt.Printf("applied. registry.get(%q) -> %v\n", *module, got)
		fmt.Println("now point web/config.js + config.testnet.js at the new hash and republish.")
		return
	}

	// Deploy (or reuse, if a previous run already put this exact build on-chain).
	nb, err := os.ReadFile(filepath.Join(cfg.ContractsDir, *module+".nef")) // #nosec G304 -- operator tool: paths are the operator's own args
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(cfg.ContractsDir, *module+".manifest.json")) // #nosec G304
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))

	h := state.CreateContractHash(sender, ne.Checksum, m.Name) // same byte order as cmd/deploy
	if _, err := cli.GetContractStateByAddressOrName("0x" + h.StringLE()); err == nil {
		fmt.Printf("reusing  %-12s -> 0x%s (already on-chain)\n", *module, h.StringLE())
	} else {
		mgmt := management.New(act)
		txh, vub, err := mgmt.Deploy(&ne, m, sender.BytesBE()) // owner = deploy wallet (TestNet posture)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		if _, err := cli.GetContractStateByAddressOrName("0x" + h.StringLE()); err != nil {
			check(fmt.Errorf("deploy tx 0x%s HALTed but contract not found on-chain: %w", txh.StringLE(), err))
		}
		fmt.Printf("deployed %-12s -> 0x%s  tx 0x%s\n", *module, h.StringLE(), txh.StringLE())
	}

	// Post-deploy wiring, per module. Keep in sync with cmd/deploy's wire phase and
	// cmd/launch2's activation values — a swapped module must come back with the SAME
	// parameters its predecessor had, or the economics silently change.
	switch *module {
	case "lend":
		call(h, "configure", registry, token)
		// The launch2 rate curve (cmd/launch2/main.go): 2% APR base, 12% at the 80%
		// kink, 72% peak; reserve 1/3 to FTB holders. 1e18 per-block fixed-point.
		const blocksPerYear = 2_102_400
		mkRate := func(aprMilli int64) int64 { return aprMilli * 1_000_000_000_000_000 / blocksPerYear }
		call(h, "setRates", mkRate(20), mkRate(125), mkRate(3000), 8000, 3333)
	default:
		check(fmt.Errorf("no wiring recipe for module %q — add one before swapping it", *module))
	}

	// Seam board: propose now, apply after the timelock. The delay comes from the
	// chain (registry.delay()), not the config, so this can never lie about the wait.
	delayRes, err := cli.InvokeFunction(registry, "delay", []smartcontract.Parameter{}, nil)
	check(err)
	delay := int64(0)
	if delayRes.State == "HALT" && len(delayRes.Stack) > 0 {
		switch v := delayRes.Stack[0].Value().(type) {
		case int64:
			delay = v
		case *big.Int:
			delay = v.Int64()
		}
	}
	if delay <= 0 {
		check(fmt.Errorf("registry.delay() unreadable (state %s) — refusing to guess the wait", delayRes.State))
	}
	call(registry, "propose", *module, h)

	start, err := cli.GetBlockCount()
	check(err)
	target := start + uint32(delay) + 1 // #nosec G115 -- registry delay is a small positive chain value
	fmt.Printf("proposed. timelock %d blocks (target height %d); waiting...\n", delay, target)
	deadline := time.Now().Add(20 * time.Minute)
	for {
		cur, err := cli.GetBlockCount()
		if err == nil && cur > target {
			break
		}
		if time.Now().After(deadline) {
			fmt.Printf("timelock wait timed out; apply when ready:\n  go run ./cmd/swap -config %s -module %s -apply\n", *cfgPath, *module)
			return
		}
		time.Sleep(15 * time.Second)
	}
	call(registry, "apply", *module)
	fmt.Printf("swapped %s -> 0x%s\n", *module, h.StringLE())
	fmt.Println("now point web/config.js + config.testnet.js at the new hash and republish.")
}
