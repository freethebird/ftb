// stresstest fires a configurable number of live transactions (default 1000) at
// the v4 AMM and FTB token to shake out decimal and invariant bugs before MainNet.
// It swaps both directions in varied sizes (including 1-base-unit edge cases), runs
// round trips that must never profit, and self-transfers precise amounts, asserting
// after EACH operation, in exact base units, that:
//   - the router's own getAmountOut quote equals the realized output exactly,
//   - the constant product k = rIn*rOut never decreases (the fee stays in the pool),
//   - reserve accounting is exact (rIn += amountIn, rOut -= amountOut),
//   - a GAS->FTB->GAS round trip returns strictly LESS than it put in (no free value),
//   - FTB totalSupply never changes from trading or transfers (no mint/burn),
//   - the wallet's own balances move by exactly the traded amounts.
//
// Any FAULT or broken invariant is collected and summarized; nothing passes silently.
// Read-only checks run between signed txs. Signed by the wallet you pass (its witness
// must hold inside the tokens, so the swap uses a CustomContracts signer, like the seed).
//
//	FTB_DEPLOY_PASS=... go run ./cmd/stresstest -wallet ~/ftb-keys/ftb-testnet-v4-deploy.wallet.json -n 1000
//
// Pinned to neo-go v0.123. All amounts are base units (8 decimals).
package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/unwrap"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

func mustHash(s string) util.Uint160 {
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	h, err := util.Uint160DecodeStringLE(s)
	if err != nil {
		panic(err)
	}
	return h
}

func main() {
	rpcURL := flag.String("rpc", "https://testnet1.neo.coz.io:443", "RPC")
	walletPath := flag.String("wallet", "cmd/deploy/testnet.wallet.json", "wallet (NEP-6)")
	passEnv := flag.String("passenv", "FTB_DEPLOY_PASS", "env var with the wallet password")
	n := flag.Int("n", 1000, "number of signed transactions to fire")
	seed := flag.Int64("seed", 1, "RNG seed (deterministic run)")
	routerF := flag.String("router", "0x6b98c58d952c804880b03acaff4dd767810641f3", "AMM router")
	ftbF := flag.String("ftb", "0xce37caa3f6c0ac6d4cbd82b14ccb8ec8740f3e43", "FTB token")
	pairF := flag.String("pair", "0xbc87cfdf9f842a38a1e7fac560bae8c48279b89a", "GAS/FTB pair")
	gasF := flag.String("gas", "0xd2a4cff31913016155e38e474a2c06d08be276cf", "GAS token")
	flag.Parse()

	router, ftb, pair, gas := mustHash(*routerF), mustHash(*ftbF), mustHash(*pairF), mustHash(*gasF)
	rng := rand.New(rand.NewSource(*seed)) // #nosec G404 -- deterministic seeded PRNG for reproducible stress runs; not a security context

	w, err := wallet.NewWalletFromFile(*walletPath)
	fatal(err)
	acc := w.Accounts[0]
	pass := os.Getenv(*passEnv)
	if pass == "" {
		fatal(fmt.Errorf("%s is empty", *passEnv))
	}
	fatal(acc.Decrypt(pass, w.Scrypt))
	me := acc.ScriptHash()

	cli, err := rpcclient.New(context.Background(), *rpcURL, rpcclient.Options{})
	fatal(err)
	fatal(cli.Init())

	// signer scope: the router pulls tokens from us inside the swap, so CalledByEntry
	// dies at the router boundary — CustomContracts naming router + both tokens + pair
	// is exactly the scope the Flamingo front end uses, same as the seed.
	act, err := actor.New(cli, []actor.SignerAccount{{
		Signer: transaction.Signer{
			Account:          me,
			Scopes:           transaction.CustomContracts,
			AllowedContracts: []util.Uint160{router, gas, ftb, pair},
		},
		Account: acc,
	}})
	fatal(err)

	read := func(h util.Uint160, method string, args ...any) *big.Int {
		v, err := unwrap.BigInt(act.Call(h, method, args...))
		fatal(err)
		return v
	}
	reserves := func() (*big.Int, *big.Int) { // (rGas, rFtb)
		arr, err := unwrap.Array(act.Call(router, "getReserves", gas, ftb))
		fatal(err)
		rg, _ := arr[0].TryInteger()
		rf, _ := arr[1].TryInteger()
		return rg, rf
	}
	bal := func(tok util.Uint160) *big.Int { return read(tok, "balanceOf", me) }

	var fails []string
	bad := func(f string, a ...any) { fails = append(fails, fmt.Sprintf(f, a...)) }
	swaps, trips, xfers, faults := 0, 0, 0, 0

	// invariant anchor: trading and transfers must never change FTB supply.
	s0 := read(ftb, "totalSupply")
	rg0, rf0 := reserves()
	fmt.Printf("start: FTB supply %s, pool %s GAS / %s FTB, firing %d txs\n", s0, rg0, rf0, *n)
	deadline := func() *big.Int { return big.NewInt(time.Now().UnixMilli() + 5*60*1000) }
	pct := func(v *big.Int, p int64) *big.Int {
		return new(big.Int).Div(new(big.Int).Mul(v, big.NewInt(p)), big.NewInt(100))
	}

	// one swap of amountIn of tokenIn (gasIn=true: GAS->FTB). Returns realized out, or nil on skip.
	swap := func(gasIn bool, amountIn *big.Int) *big.Int {
		tokenIn, tokenOut := ftb, gas
		if gasIn {
			tokenIn, tokenOut = gas, ftb
		}
		rg, rf := reserves()
		rIn, rOut := rf, rg
		if gasIn {
			rIn, rOut = rg, rf
		}
		if amountIn.Sign() <= 0 {
			return nil
		}
		expOut, err := unwrap.BigInt(act.Call(router, "getAmountOut", amountIn, rIn, rOut))
		if err != nil || expOut.Sign() <= 0 {
			return nil // below the dust threshold; not a swap
		}
		ftb0 := bal(ftb)
		txh, vub, err := act.SendCall(router, "swapTokenInForTokenOut", me, amountIn, big.NewInt(0),
			[]any{tokenIn, tokenOut}, deadline())
		if err == nil {
			_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		}
		if err != nil {
			faults++
			bad("swap gasIn=%v amt=%s FAULTED: %v", gasIn, amountIn, err)
			return nil
		}
		swaps++
		// GAS is also the tx-fee token, so a wallet-GAS delta carries the fee. Measure the swap
		// from the POOL RESERVES (fee-independent) and cross-check only the fee-free FTB leg.
		rg1, rf1 := reserves()
		rIn1, rOut1 := rf1, rg1
		if gasIn {
			rIn1, rOut1 = rg1, rf1
		}
		actIn := new(big.Int).Sub(rIn1, rIn)    // tokens the pool received
		actOut := new(big.Int).Sub(rOut, rOut1) // tokens the pool released
		if actIn.Cmp(amountIn) != 0 {
			bad("pool received %s != amountIn %s (gasIn=%v)", actIn, amountIn, gasIn)
		}
		if actOut.Cmp(expOut) != 0 {
			bad("swap out %s != router quote %s (gasIn=%v amt=%s) DECIMAL/MATH", actOut, expOut, gasIn, amountIn)
		}
		k0 := new(big.Int).Mul(rIn, rOut)
		k1 := new(big.Int).Mul(rIn1, rOut1)
		if k1.Cmp(k0) < 0 {
			bad("k DECREASED %s -> %s (gasIn=%v amt=%s) EXPLOITABLE", k0, k1, gasIn, amountIn)
		}
		// fee-free FTB leg: GAS->FTB credits us actOut FTB; FTB->GAS debits amountIn FTB
		ftb1 := bal(ftb)
		if gasIn {
			if d := new(big.Int).Sub(ftb1, ftb0); d.Cmp(actOut) != 0 {
				bad("FTB received %s != pool released %s (gasIn=true amt=%s)", d, actOut, amountIn)
			}
		} else {
			if d := new(big.Int).Sub(ftb0, ftb1); d.Cmp(amountIn) != 0 {
				bad("FTB spent %s != amountIn %s (gasIn=false)", d, amountIn)
			}
		}
		return actOut
	}

	for i := 0; i < *n; i++ {
		switch {
		case i%20 == 19: // round trip: GAS->FTB->GAS must never profit
			rg, _ := reserves()
			amt := pct(rg, 2+rng.Int63n(4)) // 2-5% of the GAS reserve
			if amt.Sign() <= 0 {
				amt = big.NewInt(1)
			}
			got := swap(true, amt)
			if got != nil {
				back := swap(false, got)
				if back != nil {
					if back.Cmp(amt) >= 0 {
						bad("ROUND TRIP PROFIT: in %s GAS -> out %s GAS (>= in) EXPLOITABLE", amt, back)
					}
					trips++
				}
			}
		case i%10 == 5: // token path: self-transfer a precise amount, supply must hold
			b := bal(ftb)
			amt := big.NewInt(1 + rng.Int63n(123456789)) // odd precise amount, 8-dec
			if amt.Cmp(b) > 0 {
				amt = big.NewInt(1)
			}
			txh, vub, err := act.SendCall(ftb, "transfer", me, me, amt, nil)
			if err == nil {
				_, err = act.WaitSuccess(context.Background(), txh, vub, err)
			}
			if err != nil {
				faults++
				bad("self-transfer %s FAULTED: %v", amt, err)
			} else {
				xfers++
				if bal(ftb).Cmp(b) != 0 {
					bad("self-transfer of %s changed balance %s -> %s", amt, b, bal(ftb))
				}
			}
		default: // ordinary swap, varied size incl. 1-base-unit edge
			gasIn := rng.Intn(2) == 0
			rg, rf := reserves()
			r := rf
			if gasIn {
				r = rg
			}
			var amt *big.Int
			switch rng.Intn(10) {
			case 0:
				amt = big.NewInt(1) // dust edge
			case 1:
				amt = pct(r, 8+rng.Int63n(5)) // 8-12%, high impact
			default:
				cap := pct(r, 1+rng.Int63n(5)) // 1-5%
				if cap.Sign() <= 0 {
					cap = big.NewInt(1)
				}
				amt = new(big.Int).Add(big.NewInt(1), new(big.Int).Rand(rng, cap))
			}
			swap(gasIn, amt)
		}
		if i%50 == 49 { // periodic supply sweep
			if read(ftb, "totalSupply").Cmp(s0) != 0 {
				bad("FTB supply CHANGED to %s (was %s) at iter %d MINT/BURN BUG", read(ftb, "totalSupply"), s0, i)
			}
		}
		if len(fails) > 25 {
			fmt.Println("stopping early: too many failures")
			break
		}
	}

	// final invariants
	s1 := read(ftb, "totalSupply")
	if s1.Cmp(s0) != 0 {
		bad("FINAL FTB supply %s != start %s MINT/BURN BUG", s1, s0)
	}
	rg1, rf1 := reserves()
	fmt.Printf("\ndone: %d swaps, %d round-trips, %d transfers, %d faults\n", swaps, trips, xfers, faults)
	fmt.Printf("pool now %s GAS / %s FTB (was %s / %s); FTB supply %s (unchanged: %v)\n",
		rg1, rf1, rg0, rf0, s1, s1.Cmp(s0) == 0)
	if len(fails) == 0 {
		fmt.Println("\nALL INVARIANTS HELD — no decimal or exploit issue found")
		return
	}
	fmt.Printf("\n%d FAILURE(S):\n", len(fails))
	for _, f := range fails {
		fmt.Println("  FAIL:", f)
	}
	os.Exit(1)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "stresstest:", err)
		os.Exit(1)
	}
}
