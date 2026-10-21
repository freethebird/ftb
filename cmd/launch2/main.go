// launch2 flips Launch 2 on TestNet from "deployed" to "live": it sets the
// lend rate curve (rates ship at zero by design — lend.go: "set a real curve
// with SetRates before launch"), relaxes the oracle freshness window for
// TestNet (no reporter daemon yet; the committee model lands with the 5
// community nodes), and posts the first NEO/GAS price reports from the deploy
// wallet, which is an enabled reporter.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/launch2 -config cmd/sendgas/config.testnet.json
//
// Prices are FTB base units per 1 WHOLE asset (oracle.go:11) and TestNet-only
// launch ratios: 1 NEO = 100 FTB, 1 GAS = 40 FTB. Any enabled reporter can
// move them at any time; the lend market consumes the median of fresh reports.
//
// Rate curve (1e18 fixed-point per block, 2,102,400 blocks/yr, lend.go:461):
//
//	base    2% APR                      = 0.020e18 / 2102400
//	slope1  +10 APR points to the kink  = 0.125e18 / 2102400   -> 12% at 80% util
//	slope2  +60 APR points past it      = 3.000e18 / 2102400   -> 72% at 100%
//	kink 8000 bps, reserve 3333 bps (the holders' 1/3, docs/governance/ECONOMICS.md)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type config struct {
	RPC         string `json:"rpc"`
	Wallet      string `json:"wallet"`
	PasswordEnv string `json:"password_env"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "launch2:", err)
		os.Exit(1)
	}
}

func main() {
	cfgPath := flag.String("config", "config.json", "config JSON (same shape as cmd/sendgas)")
	flag.Parse()

	// TestNet contract hashes (web/config.testnet.js).
	lend, err := util.Uint160DecodeStringLE("c3d249675bd870b43bf4c36496a9952131f96a83")
	check(err)
	oracle, err := util.Uint160DecodeStringLE("1c92a93d438b9468b45706cbc4b1ddcc345df875")
	check(err)
	neoToken, err := util.Uint160DecodeStringLE("ef4073a0f2b305a38ec4050e4d3d28bc40ea63f5")
	check(err)
	gasToken, err := util.Uint160DecodeStringLE("d2a4cff31913016155e38e474a2c06d08be276cf")
	check(err)

	const blocksPerYear = 2_102_400 // lend.go: 15-second target, linear annualization
	mkRate := func(aprMilli int64) int64 {
		// aprMilli: APR in thousandths (20 = 2%). per-block rate on the 1e18 scale.
		return aprMilli * 1_000_000_000_000_000 / blocksPerYear // = apr/1000 * 1e18 / blocksPerYear
	}
	base := mkRate(20)     // 2% APR
	slope1 := mkRate(125)  // +10 APR points by the 80% kink (0.125 * 0.8 = 0.10)
	slope2 := mkRate(3000) // +60 APR points from kink to full (3.0 * 0.2 = 0.60)

	var cfg config
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(json.Unmarshal(raw, &cfg))

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

	act, err := actor.NewSimple(cli, acc)
	check(err)
	send := func(hash util.Uint160, op string, args ...any) {
		txh, vub, err := act.SendCall(hash, op, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("ok  %-22s tx 0x%s\n", op, txh.StringLE())
	}

	send(lend, "setRates", base, slope1, slope2, int64(8000), int64(3333))
	// TestNet: 24h freshness, 1 reporter — the 10-minute multi-reporter window
	// is the MainNet posture (DECISIONS.md: committee = the 5 community nodes).
	send(oracle, "setParams", int64(86_400_000), int64(1))
	send(oracle, "report", acc.ScriptHash(), neoToken, int64(10_000_000_000)) // 1 NEO = 100 FTB
	send(oracle, "report", acc.ScriptHash(), gasToken, int64(4_000_000_000))  // 1 GAS = 40 FTB

	fmt.Println("launch2 live: rates set, oracle seeded (1 NEO = 100 FTB, 1 GAS = 40 FTB, 24h freshness)")
}
