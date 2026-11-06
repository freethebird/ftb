// loanbooktest exercises the live v4 FtbLoanBook (offer-based, oracleless P2P term lending)
// end to end on-chain, asserting the zero-close and no-bad-debt properties the design promises.
// One wallet plays both lender and borrower (the contract does not forbid self-dealing), using
// GAS as collateral, so no second funded wallet is needed. It runs four cycles:
//
//  1. repay in good standing  -> collateral returned, lender withdraws principal+interest,
//     the contract ends holding EXACTLY zero of both tokens, FTB supply unchanged.
//  2. default at maturity     -> permissionless Close credits the collateral to the lender,
//     the borrower keeps the principal, no FTB stranded, no bad debt.
//  3. overpay on repay        -> the surplus over repayAmount is refunded to the payer.
//  4. cancel an open offer     -> escrowed principal fully refunded.
//
// Every amount is checked in exact base units (interest = principal*rateBps/10000, integer),
// so a decimal slip is a failed assertion. Signed by the wallet you pass (CalledByEntry).
//
//	FTB_DEPLOY_PASS=... go run ./cmd/loanbooktest -wallet ~/ftb-keys/ftb-testnet-v4-deploy.wallet.json
//
// Pinned to neo-go v0.123. Cycle 2 waits out a 60s minimum term, so the run takes ~5 minutes.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/unwrap"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

const e8 = 100_000_000

func mustHash(s string) util.Uint160 {
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	h, err := util.Uint160DecodeStringLE(s)
	fatal(err)
	return h
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "loanbooktest:", err)
		os.Exit(1)
	}
}

func main() {
	rpcURL := flag.String("rpc", "https://testnet1.neo.coz.io:443", "RPC")
	walletPath := flag.String("wallet", "cmd/deploy/testnet.wallet.json", "wallet (NEP-6)")
	passEnv := flag.String("passenv", "FTB_DEPLOY_PASS", "env var with the wallet password")
	lbF := flag.String("loanbook", "0x231441f837c674d198177d122c23d4d65c438107", "FtbLoanBook")
	ftbF := flag.String("ftb", "0xce37caa3f6c0ac6d4cbd82b14ccb8ec8740f3e43", "FTB token")
	gasF := flag.String("gas", "0xd2a4cff31913016155e38e474a2c06d08be276cf", "GAS token")
	flag.Parse()

	lb, ftb, gas := mustHash(*lbF), mustHash(*ftbF), mustHash(*gasF)

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
	act, err := actor.NewSimple(cli, acc)
	fatal(err)

	readInt := func(h util.Uint160, m string, args ...any) *big.Int {
		v, err := unwrap.BigInt(act.Call(h, m, args...))
		fatal(err)
		return v
	}
	bal := func(tok util.Uint160, who util.Uint160) *big.Int { return readInt(tok, "balanceOf", who) }
	statusOf := func(id int64) int64 { return readInt(lb, "statusOf", big.NewInt(id)).Int64() }
	claimable := func(tok util.Uint160) *big.Int { return readInt(lb, "claimable", me, tok) }
	supply := func() *big.Int { return readInt(ftb, "totalSupply") }

	var fails []string
	bad := func(f string, a ...any) {
		fails = append(fails, fmt.Sprintf(f, a...))
		fmt.Println("  FAIL:", fmt.Sprintf(f, a...))
	}
	eq := func(label string, got, want *big.Int) {
		if got.Cmp(want) != 0 {
			bad("%s: got %s want %s", label, got, want)
		}
	}
	// send fires a signed call and waits; returns true on HALT, records a fault otherwise.
	send := func(what string, h util.Uint160, m string, args ...any) bool {
		txh, vub, err := act.SendCall(h, m, args...)
		if err == nil {
			_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		}
		if err != nil {
			bad("%s FAULTED: %v", what, err)
			return false
		}
		return true
	}
	offerData := func(colTok util.Uint160, colAmt, rate, term int64) []any {
		return []any{"offer", colTok, big.NewInt(colAmt), big.NewInt(rate), big.NewInt(term)}
	}

	S0 := supply()
	fmt.Printf("loanbook %s  FTB supply %s  starting\n\n", lb.StringLE(), S0)

	// ---- Cycle 1: repay in good standing, assert zero-close ----
	fmt.Println("cycle 1: offer -> take -> repay -> withdraw (zero-close)")
	id := readInt(lb, "offerCount").Int64() // next id
	lbFtb0, lbGas0 := bal(ftb, lb), bal(gas, lb)
	// offer: 100 FTB principal, 5 GAS collateral, 5% flat, 1-day term
	if send("offer", ftb, "transfer", me, lb, big.NewInt(100*e8), offerData(gas, 5*e8, 500, 86_400_000)) {
		if statusOf(id) != 1 {
			bad("after offer, status=%d want OPEN(1)", statusOf(id))
		}
		eq("escrow FTB", bal(ftb, lb), new(big.Int).Add(lbFtb0, big.NewInt(100*e8)))
		meFtbPreTake := bal(ftb, me)
		if send("take", gas, "transfer", me, lb, big.NewInt(5*e8), []any{"take", big.NewInt(id)}) {
			if statusOf(id) != 2 {
				bad("after take, status=%d want TAKEN(2)", statusOf(id))
			}
			eq("repayAmount (100 + 5%)", readInt(lb, "repayAmountOf", big.NewInt(id)), big.NewInt(105*e8))
			eq("principal to borrower", bal(ftb, me), new(big.Int).Add(meFtbPreTake, big.NewInt(100*e8)))
			eq("collateral escrowed", bal(gas, lb), new(big.Int).Add(lbGas0, big.NewInt(5*e8)))
			if send("repay", ftb, "transfer", me, lb, big.NewInt(105*e8), []any{"repay", big.NewInt(id)}) {
				if statusOf(id) != 3 {
					bad("after repay, status=%d want REPAID(3)", statusOf(id))
				}
				eq("lender claimable FTB", claimable(ftb), big.NewInt(105*e8))
				// collateral return is verified fee-free via the contract's GAS balance below
				if send("withdraw", lb, "withdraw", me, ftb) {
					eq("claimable zeroed", claimable(ftb), big.NewInt(0))
					eq("contract FTB back to start", bal(ftb, lb), lbFtb0)
					eq("contract GAS back to start", bal(gas, lb), lbGas0)
				}
			}
		}
	}

	// ---- Cycle 2: default at maturity, assert collateral-to-lender, no bad debt ----
	fmt.Println("cycle 2: offer -> take -> (wait maturity) -> Close -> withdraw (default)")
	id2 := readInt(lb, "offerCount").Int64()
	lbGasC2 := bal(gas, lb)
	if send("offer2", ftb, "transfer", me, lb, big.NewInt(50*e8), offerData(gas, 3*e8, 1000, 60_000)) { // 60s term
		meFtbAfterOffer := bal(ftb, me)
		if send("take2", gas, "transfer", me, lb, big.NewInt(3*e8), []any{"take", big.NewInt(id2)}) {
			eq("borrower keeps principal", bal(ftb, me), new(big.Int).Add(meFtbAfterOffer, big.NewInt(50*e8)))
			fmt.Println("  waiting out the 60s term...")
			time.Sleep(85 * time.Second)
			if send("close", lb, "close", big.NewInt(id2)) {
				if statusOf(id2) != 4 {
					bad("after close, status=%d want DEFAULTED(4)", statusOf(id2))
				}
				eq("collateral credited to lender", claimable(gas), big.NewInt(3*e8))
				if send("withdraw2", lb, "withdraw", me, gas) {
					eq("collateral claim zeroed", claimable(gas), big.NewInt(0))
					// collateral release verified fee-free via the contract's own GAS balance:
					// take2 added 3 GAS, withdraw2 pays it to the lender, so the contract returns to lbGasC2
					eq("contract GAS back to start (collateral released)", bal(gas, lb), lbGasC2)
				}
			}
		}
	}

	// ---- Cycle 3: overpay refund + an odd rate (decimal direction) ----
	fmt.Println("cycle 3: offer(rate 3.33%) -> take -> OVERPAY repay (refund)")
	id3 := readInt(lb, "offerCount").Int64()
	// 100 FTB, rate 333 bps -> interest 100*333/10000 = 3.33 FTB exactly; repay 103.33 FTB
	if send("offer3", ftb, "transfer", me, lb, big.NewInt(100*e8), offerData(gas, 5*e8, 333, 86_400_000)) {
		if send("take3", gas, "transfer", me, lb, big.NewInt(5*e8), []any{"take", big.NewInt(id3)}) {
			eq("repayAmount (100 + 3.33%)", readInt(lb, "repayAmountOf", big.NewInt(id3)), big.NewInt(10333*e8/100))
			meFtbPre := bal(ftb, me)
			// overpay by 10 FTB; expect 10 - (103.33-?) ... pay 110, due 103.33, refund 6.67
			if send("repay-overpay", ftb, "transfer", me, lb, big.NewInt(110*e8), []any{"repay", big.NewInt(id3)}) {
				eq("lender claimable (exact due)", claimable(ftb), big.NewInt(10333*e8/100))
				// net FTB out of our wallet for repay should be exactly repayAmount (overpay refunded),
				// and we got 5 GAS collateral back. FTB delta = -(103.33) + nothing else.
				paid := new(big.Int).Sub(meFtbPre, bal(ftb, me))
				eq("overpay refunded (net paid == due)", paid, big.NewInt(10333*e8/100))
				send("withdraw3", lb, "withdraw", me, ftb)
			}
		}
	}

	// ---- Cycle 4: cancel an open offer, full refund ----
	fmt.Println("cycle 4: offer -> cancel (full refund)")
	id4 := readInt(lb, "offerCount").Int64()
	lbFtbPre := bal(ftb, lb)
	if send("offer4", ftb, "transfer", me, lb, big.NewInt(77*e8), offerData(gas, 2*e8, 250, 86_400_000)) {
		eq("escrow before cancel", bal(ftb, lb), new(big.Int).Add(lbFtbPre, big.NewInt(77*e8)))
		if send("cancel", lb, "cancelOffer", big.NewInt(id4)) {
			if statusOf(id4) != 5 {
				bad("after cancel, status=%d want CANCELLED(5)", statusOf(id4))
			}
			eq("escrow refunded on cancel", bal(ftb, lb), lbFtbPre)
		}
	}

	// ---- global invariant: trading/lending never minted or burned FTB ----
	eq("FTB supply unchanged end-to-end", supply(), S0)

	fmt.Printf("\n%d failure(s)\n", len(fails))
	if len(fails) == 0 {
		fmt.Println("ALL LOANBOOK INVARIANTS HELD — repay zero-closes, default has no bad debt, no decimal slip")
		return
	}
	os.Exit(1)
}
