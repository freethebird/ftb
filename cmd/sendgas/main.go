// sendgas tops up a TestNet address with GAS from the deploy wallet — the
// repeatable onboarding step for operators and testers who need fee money for
// votes and claims (faucet routes are unreliable; see DECISIONS.md / ops notes).
//
//	go run ./cmd/sendgas -config cmd/sendgas/config.testnet.json -to N... -amount 10
//
// Same SDK caveats as cmd/deploy: pinned to neo-go v0.123; N3 addresses are
// network-agnostic, the same address exists on TestNet and MainNet.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
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
		fmt.Fprintln(os.Stderr, "sendgas:", err)
		os.Exit(1)
	}
}

func main() {
	cfgPath := flag.String("config", "config.json", "sendgas config JSON")
	to := flag.String("to", "", "recipient N3 address (N...)")
	amount := flag.Float64("amount", 0, "whole GAS to send (e.g. 10)")
	flag.Parse()

	if *to == "" || *amount <= 0 {
		check(fmt.Errorf("need -to N... and -amount > 0"))
	}
	recipient, err := address.StringToUint160(*to)
	check(err)
	// GAS token contract, same hash on every Neo N3 network (canonical StringLE form).
	gasToken, err := util.Uint160DecodeStringLE("d2a4cff31913016155e38e474a2c06d08be276cf")
	check(err)

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

	base := int64(*amount * 100_000_000) // GAS has 8 decimals
	if float64(base) != *amount*100_000_000 {
		check(fmt.Errorf("amount %v does not fit GAS base units exactly", *amount))
	}

	act, err := actor.NewSimple(cli, acc)
	check(err)
	txh, vub, err := act.SendCall(gasToken, "transfer", acc.ScriptHash(), recipient, base, nil)
	check(err)
	_, err = act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("sent %s GAS to %s  tx 0x%s\n",
		strings.TrimRight(strings.TrimRight(strconv.FormatFloat(*amount, 'f', 8, 64), "0"), "."),
		*to, txh.StringLE())
}
