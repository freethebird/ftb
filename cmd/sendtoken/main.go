// sendtoken transfers an exact NEP-17 amount from the deploy wallet — the
// general form of cmd/sendgas (which is GAS-only). Built for the dev-team
// FTB distribution: 500,000 FTB split equally across seven wallets.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/sendtoken -config cmd/sendgas/config.testnet.json \
//	  -token 0x1ff1f51e63179867367ae8982af583e01e7cbdbd \
//	  -to N... -amount 71428.57142857
//
// Amount is a decimal STRING parsed to base units exactly (8 decimals) -
// no float64 anywhere near the money path. Same SDK caveats as cmd/deploy:
// pinned to neo-go v0.123; N3 addresses are network-agnostic.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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
		fmt.Fprintln(os.Stderr, "sendtoken:", err)
		os.Exit(1)
	}
}

// parseDecimal8 converts "71428.57142857" to base units with exact decimal
// math; more than 8 fractional digits is an error, never a silent rounding.
func parseDecimal8(s string) (int64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	if neg {
		return 0, fmt.Errorf("amount must be positive: %q", s)
	}
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 8 {
		return 0, fmt.Errorf("amount %q has more than 8 decimals", s)
	}
	frac = frac + strings.Repeat("0", 8-len(frac))
	var w, f int64
	if _, err := fmt.Sscanf(whole, "%d", &w); err != nil {
		return 0, fmt.Errorf("bad whole part %q: %v", whole, err)
	}
	if _, err := fmt.Sscanf(frac, "%d", &f); err != nil {
		return 0, fmt.Errorf("bad fraction part %q: %v", frac, err)
	}
	return w*100_000_000 + f, nil
}

func main() {
	cfgPath := flag.String("config", "config.json", "sendtoken config JSON")
	token := flag.String("token", "", "NEP-17 contract hash, canonical 0x form")
	to := flag.String("to", "", "recipient N3 address (N...)")
	amount := flag.String("amount", "", "decimal amount, max 8 decimals (e.g. 71428.57142857)")
	flag.Parse()

	if *to == "" || *token == "" || *amount == "" {
		check(fmt.Errorf("need -token 0x... -to N... -amount <decimal>"))
	}
	base, err := parseDecimal8(*amount)
	check(err)
	if base <= 0 {
		check(fmt.Errorf("amount must be > 0"))
	}
	recipient, err := address.StringToUint160(*to)
	check(err)
	tok, err := util.Uint160DecodeStringLE(strings.TrimPrefix(strings.TrimSpace(*token), "0x"))
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

	act, err := actor.NewSimple(cli, acc)
	check(err)
	txh, vub, err := act.SendCall(tok, "transfer", acc.ScriptHash(), recipient, base, nil)
	check(err)
	_, err = act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("sent %s to %s  tx 0x%s\n", *amount, *to, txh.StringLE())
}
