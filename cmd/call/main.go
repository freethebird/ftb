// call sends ONE contract call from the deploy wallet — the generic ops
// companion to cmd/swap (which deploys) and cmd/update (which patches). Use it
// for permissionless or owner-guarded maintenance calls: reserve sweeps,
// registry applies, oracle nudges.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/call -config cmd/deploy/config.testnet.json \
//	  -contract 0xae55c7b4950f9da4486baafdfd6ba2ae5ebdf8f4 -method sweepDividends
//
// No arguments support yet: every current maintenance call is nullary. Add
// typed args here when one needs them.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type config struct {
	RPC         string `json:"rpc"`
	Wallet      string `json:"wallet"`
	Password    string `json:"password"`
	PasswordEnv string `json:"password_env"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "call:", err)
		os.Exit(1)
	}
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy config JSON (rpc, wallet, password_env)")
	target := flag.String("contract", "", "contract hash, canonical 0x form")
	method := flag.String("method", "", "method to invoke (no args)")
	flag.Parse()
	if *target == "" || *method == "" {
		fmt.Fprintln(os.Stderr, "need -contract and -method")
		os.Exit(2)
	}

	var cfg config
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(json.Unmarshal(raw, &cfg))

	h, err := util.Uint160DecodeStringLE(strings.TrimPrefix(strings.TrimSpace(*target), "0x"))
	check(err)

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

	act, err := actor.NewSimple(cli, acc)
	check(err)
	txh, vub, err := act.SendCall(h, *method)
	check(err)
	aer, err := act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("%s on 0x%s  tx 0x%s  gas %d\n", *method, h.StringLE(), txh.StringLE(), aer.GasConsumed)
}
