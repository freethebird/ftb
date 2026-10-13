// seed puts protocol-owned liquidity into the reused, audited Flamingo swap so FTB can
// trade. For each configured pool it calls the Router's AddLiquidity with the deployer as
// the liquidity provider, then reads GetReserves back to confirm the pool took the funds.
//
// Pre-reqs:
//
//   - The reused swap (Factory, Pair x3, Router) is already deployed and the three pairs
//     are created and whitelisted (see README; those are external audited contracts).
//
//   - The deployer wallet holds the tokens to seed. NEO is indivisible, so seed bNEO:
//     wrap NEO -> bNEO via NeoBurger first.
//
//   - Initial seed sets the opening price, so AddLiquidity min amounts are 0 (no slippage
//     on an empty pool).
//
//     go run ./cmd/seed -config config.json
//
// Written to the neo-go v0.123 SDK; verify signatures against your pinned version. The
// stable alternative is the neo-go CLI invokefunction commands in README.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type pool struct {
	Name    string `json:"name"`
	TokenA  string `json:"tokenA"`
	TokenB  string `json:"tokenB"`
	AmountA string `json:"amountA"` // base units, decimal; underscores allowed
	AmountB string `json:"amountB"`
}
type config struct {
	RPC             string `json:"rpc"`
	Wallet          string `json:"wallet"`
	Password        string `json:"password"`
	Router          string `json:"router"`
	DeadlineMinutes int    `json:"deadline_minutes"`
	Pools           []pool `json:"pools"`
}

func main() {
	cfgPath := flag.String("config", "config.json", "seed config JSON")
	force := flag.Bool("force", false, "seed even if a pool already has reserves (unsafe: min amounts are 0)")
	flag.Parse()

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
	check(acc.Decrypt(cfg.Password, w.Scrypt))
	sender := acc.ScriptHash()

	act, err := actor.NewSimple(cli, acc)
	check(err)

	router := mustU160(cfg.Router)
	if cfg.DeadlineMinutes <= 0 {
		cfg.DeadlineMinutes = 10
	}

	for _, p := range cfg.Pools {
		tA := mustU160(p.TokenA)
		tB := mustU160(p.TokenB)
		aA := mustAmount(p.AmountA)
		aB := mustAmount(p.AmountB)
		deadline := big.NewInt(time.Now().Add(time.Duration(cfg.DeadlineMinutes) * time.Minute).UnixMilli())

		fmt.Printf("seeding %-10s  %s + %s\n", p.Name, p.AmountA, p.AmountB)
		// FTB-04: only seed an empty pool. With min amounts of 0, a non-empty pool lets a
		// front-runner set a skewed opening ratio and skim the seed, so abort unless empty.
		pre, err := act.Call(router, "getReserves", tA, tB)
		check(err)
		if pre.State != "HALT" {
			check(fmt.Errorf("getReserves for %s did not HALT: %s", p.Name, pre.State))
		}
		if s := stackSum(pre.Stack); s.Sign() > 0 && !*force {
			check(fmt.Errorf("pool %s already has reserves (sum=%s); refusing to seed with min=0; re-run with -force only if certain", p.Name, s))
		}
		// AddLiquidity(sender, tokenA, tokenB, amtADesired, amtBDesired, amtAMin=0, amtBMin=0, deadline)
		txh, vub, err := act.SendCall(router, "addLiquidity",
			sender, tA, tB, aA, aB, big.NewInt(0), big.NewInt(0), deadline)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)

		// read back reserves to confirm the pool took the funds (audit every step)
		res, err := act.Call(router, "getReserves", tA, tB)
		check(err)
		if len(res.Stack) > 0 {
			fmt.Printf("  state=%s  reserves=%v\n", res.State, res.Stack[0].Value())
		} else {
			fmt.Printf("  state=%s  (no reserves returned)\n", res.State)
		}
	}

	fmt.Println("\nSeeded. Do a small manual swap to confirm pricing, e.g.:")
	fmt.Println("  neo-go contract invokefunction -r $RPC -w wallet.json <router> \\")
	fmt.Println("    swapTokenInForTokenOut <sender> <amountIn> <minOut> [<tokenIn>,<tokenOut>] <deadline> -- <sender>")
}

// stackSum folds all integer values in an invocation result (including one level of array
// nesting) into a single magnitude, so a non-empty pool is detected regardless of the exact
// getReserves return shape. Audit item FTB-04.
func stackSum(items []stackitem.Item) *big.Int {
	sum := new(big.Int)
	for _, it := range items {
		switch v := it.Value().(type) {
		case *big.Int:
			sum.Add(sum, new(big.Int).Abs(v))
		case []stackitem.Item:
			sum.Add(sum, stackSum(v))
		}
	}
	return sum
}

func mustU160(s string) util.Uint160 {
	s = strings.TrimSpace(s)
	var (
		u   util.Uint160
		err error
	)
	switch {
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		u, err = util.Uint160DecodeStringBE(s[2:])
	case strings.HasPrefix(s, "N"):
		u, err = address.StringToUint160(s)
	default:
		u, err = util.Uint160DecodeStringLE(s)
	}
	check(err)
	return u
}

func mustAmount(s string) *big.Int {
	s = strings.ReplaceAll(strings.TrimSpace(s), "_", "")
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		check(fmt.Errorf("bad amount %q", s))
	}
	return n
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
