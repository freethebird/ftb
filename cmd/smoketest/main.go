// smoketest runs the full lend cycle on a LIVE chain (TestNet, then pre-launch
// MainNet checklist): supply FTB, post GAS collateral, borrow, let interest accrue,
// repay in full, withdraw collateral, withdraw supply — asserting the position closes
// to EXACTLY zero at every step. It exists because the v1 dust-debt bug (DECISIONS.md
// 18 Oct: floor division stranded 1 base unit) only showed itself on a live chain;
// this is the standing proof that a loan now closes completely, end to end.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/smoketest -config cmd/deploy/config.testnet.json
//
// The wallet needs FTB (supply + interest buffer) and GAS (collateral + fees). If it
// is short, the tool prints the exact shortfall and exits before touching the chain.
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

	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

const dec = 100_000_000 // FTB/GAS base units

// Current TestNet deployment (web/config.testnet.js — keep in sync when modules swap).
const (
	ftbHex     = "dc82bb0c89085dfeb81a40cebce786cb1b73044c"
	lendHex    = "ae55c7b4950f9da4486baafdfd6ba2ae5ebdf8f4"
	gasHex     = "d2a4cff31913016155e38e474a2c06d08be276cf" // native, same everywhere
	accrueWait = 75 * time.Second                           // ~5 blocks at the 15s target, enough for the index to move
)

type config struct {
	RPC         string `json:"rpc"`
	Wallet      string `json:"wallet"`
	Password    string `json:"password"`
	PasswordEnv string `json:"password_env"`
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "smoketest:", err)
		os.Exit(1)
	}
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy config JSON (rpc, wallet, password_env)")
	supplyFTB := flag.Int64("supply", 5, "FTB to supply into the pool (whole FTB)")
	borrowFTB := flag.Int64("borrow", 1, "FTB to borrow (whole FTB)")
	collatGAS := flag.Int64("collateral", 1, "GAS to post as collateral (whole GAS)")
	flag.Parse()

	var cfg config
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(json.Unmarshal(raw, &cfg))

	ftb := mustHash(ftbHex)
	lend := mustHash(lendHex)
	gasT := mustHash(gasHex)

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
	me := acc.ScriptHash()
	act, err := actor.NewSimple(cli, acc)
	check(err)

	readInt := func(hash util.Uint160, method string, args ...smartcontract.Parameter) *big.Int {
		res, err := cli.InvokeFunction(hash, method, args, nil)
		check(err)
		if res.State != "HALT" || len(res.Stack) == 0 {
			check(fmt.Errorf("%s did not HALT: %s (%s)", method, res.State, res.FaultException))
		}
		v, ok := res.Stack[0].Value().(*big.Int)
		if !ok {
			check(fmt.Errorf("%s: unexpected stack value %v", method, res.Stack[0].Value()))
		}
		return v
	}
	meP := smartcontract.Parameter{Type: smartcontract.Hash160Type, Value: me}

	// Preflight: balances must cover supply + repay buffer (interest dust) and
	// collateral + fees. Print the exact shortfall and exit BEFORE any tx.
	ftbBal := readInt(ftb, "balanceOf", meP)
	gasBal := readInt(gasT, "balanceOf", meP)
	needFTB := (*supplyFTB + *borrowFTB) * dec // supply + worst-case interest buffer ~= borrow again
	needGAS := (*collatGAS)*dec + 2*dec        // collateral + ~2 GAS fees headroom
	fmt.Printf("wallet %s: %.8f FTB, %.8f GAS\n", acc.Address, float64(ftbBal.Int64())/dec, float64(gasBal.Int64())/dec)
	if ftbBal.Int64() < needFTB || gasBal.Int64() < needGAS {
		fmt.Printf("SHORT: need %.8f FTB and %.8f GAS (incl. buffers). Fund %s and rerun.\n",
			float64(needFTB)/dec, float64(needGAS)/dec, acc.Address)
		os.Exit(1)
	}

	call := func(hash util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(hash, method, args...)
		check(err)
		aer, err := act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  %-22s tx 0x%s (gas %d)\n", method, txh.StringLE(), aer.GasConsumed)
	}
	expect := func(label string, got *big.Int, want int64) {
		if got.Int64() != want {
			check(fmt.Errorf("%s: got %d base units, want %d", label, got.Int64(), want))
		}
		fmt.Printf("  assert %-16s = %d ✓\n", label, want)
	}

	fmt.Println("1) supply", *supplyFTB, "FTB")
	call(ftb, "transfer", me, lend, *supplyFTB*dec, []any{"supply"})
	expect("sharesOf", readInt(lend, "sharesOf", meP), *supplyFTB*dec)

	fmt.Println("2) post", *collatGAS, "GAS collateral")
	call(gasT, "transfer", me, lend, *collatGAS*dec, nil)
	expect("collateralOf", readInt(lend, "collateralOf", meP, smartcontract.Parameter{Type: smartcontract.BoolType, Value: false}), *collatGAS*dec)

	fmt.Println("3) borrow", *borrowFTB, "FTB")
	call(lend, "borrow", me, *borrowFTB*dec)
	debt0 := readInt(lend, "debtOf", meP)
	fmt.Printf("  debt after borrow: %d base units\n", debt0.Int64())

	fmt.Printf("4) waiting %s for interest to accrue...\n", accrueWait)
	time.Sleep(accrueWait)

	debt := readInt(lend, "debtOf", meP)
	fmt.Printf("5) repay full debt %d base units (+ buffer, overpay refunded)\n", debt.Int64())
	call(ftb, "transfer", me, lend, debt.Int64()+1000, []any{"repay"})
	expect("debtOf==0", readInt(lend, "debtOf", meP), 0) // THE assertion: no dust survives

	fmt.Println("6) claim collateral back")
	call(lend, "withdrawCollateral", me, 0, *collatGAS*dec)
	expect("collateralOf==0", readInt(lend, "collateralOf", meP, smartcontract.Parameter{Type: smartcontract.BoolType, Value: false}), 0)

	fmt.Println("7) withdraw supply (shares carry the accrued interest)")
	shares := readInt(lend, "sharesOf", meP)
	call(lend, "withdrawSupply", me, shares.Int64())
	expect("sharesOf==0", readInt(lend, "sharesOf", meP), 0)

	fmt.Println("\nPASS: loan closed to exactly zero, collateral returned, supply withdrawn. Clean sheet.")
}

func mustHash(hexLE string) util.Uint160 {
	h, err := util.Uint160DecodeStringLE(strings.TrimPrefix(hexLE, "0x"))
	check(err)
	return h
}
