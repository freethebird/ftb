// neopair adds the FTB/NEO pair to the community AMM on TestNet: deploys the
// constants-only FlamingoSwapPairFtbNeo variant (TokenA=FTB — the smaller hash,
// token0 by router convention — TokenB=NEO), registers it with the factory, and
// seeds 40 FTB + 1 NEO (ratio 40, matching the GAS pool's placeholder).
// Same self-check discipline as cmd/ammdeploy: the hash is recomputed from the
// build artifacts and verified with getcontractstate before use; a re-run skips
// what is already live.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/neopair -phase all
//
// Phases: deploy | wire (factory.registerExchangePair) | seed | all.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/management"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

const pairDir = "contracts/amm-flamingo/Swap/flamingo-contract-swap/FlamingoSwapPairFtbNeo/bin/sc"
const pairName = "FlamingoSwapPairFtbNeo"

// seed: ratio 1 NEO = 40 FTB, matching the GAS pool's placeholder (Decision: the
// pool exists to exercise the swap path before the IAR; the community adds depth).
var (
	seedFTB = big.NewInt(4000000000) // 40 FTB (8 decimals)
	seedNEO = big.NewInt(1)          // 1 NEO (0 decimals, indivisible)
)

// native NEO (same on every N3 network), the TestNet FTB token, and the live
// AMM wiring from contracts/amm-flamingo/DEPLOYED.md.
var (
	neoHash = mustHash("0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5")
	ftbHash = mustHash("0xdc82bb0c89085dfeb81a40cebce786cb1b73044c")
	factory = mustHash("0x2b582cdc325e0788a6732a79671687580d985dc4")
	router  = mustHash("0x7e3fa9356e3294acf709c51062ca051f1df6cb5c")
)

func mustHash(s string) util.Uint160 {
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	h, err := util.Uint160DecodeStringLE(s)
	check(err)
	return h
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func main() {
	rpcURL := flag.String("rpc", "https://testnet1.neo.coz.io:443", "TestNet RPC")
	walletPath := flag.String("wallet", "cmd/deploy/testnet.wallet.json", "deploy wallet (NEP-6)")
	passEnv := flag.String("passenv", "FTB_DEPLOY_PASS", "env var holding the wallet password")
	phase := flag.String("phase", "all", "deploy | wire | seed | all")
	flag.Parse()

	cli, err := rpcclient.New(context.Background(), *rpcURL, rpcclient.Options{})
	check(err)
	check(cli.Init())

	w, err := wallet.NewWalletFromFile(*walletPath)
	check(err)
	acc := w.Accounts[0]
	pass := os.Getenv(*passEnv)
	if pass == "" {
		check(fmt.Errorf("%s is empty", *passEnv))
	}
	check(acc.Decrypt(pass, w.Scrypt))
	sender := acc.ScriptHash()

	act, err := actor.NewSimple(cli, acc)
	check(err)
	mgmt := management.New(act)

	nb, err := os.ReadFile(filepath.Join(pairDir, pairName+".nef")) // #nosec G304 -- fixed repo path
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(pairDir, pairName+".manifest.json")) // #nosec G304 -- fixed repo path
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))

	pair := state.CreateContractHash(sender, ne.Checksum, m.Name)
	getState := func(h util.Uint160) (*state.Contract, error) {
		return cli.GetContractStateByAddressOrName("0x" + h.StringLE())
	}

	if *phase == "deploy" || *phase == "all" {
		if _, err := getState(pair); err == nil {
			fmt.Printf("live    %-26s -> 0x%s (skip)\n", pairName, pair.StringLE())
		} else {
			txh, vub, err := mgmt.Deploy(&ne, m, nil)
			check(err)
			_, err = act.WaitSuccess(context.Background(), txh, vub, err)
			check(err)
			if _, err := getState(pair); err != nil {
				check(fmt.Errorf("deploy: tx HALTed but 0x%s not found: %w", pair.StringLE(), err))
			}
			fmt.Printf("deployed %-26s -> 0x%s  tx 0x%s\n", pairName, pair.StringLE(), txh.StringLE())
		}
	}
	if *phase == "wire" || *phase == "all" {
		// registerExchangePair reads TokenA/TokenB from the pair contract itself
		txh, vub, err := act.SendCall(factory, "registerExchangePair", pair)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-22s tx 0x%s\n", "registerExchangePair", txh.StringLE())
	}
	if *phase == "seed" || *phase == "all" {
		// the router PULLS both tokens from the sender inside addLiquidity, so the
		// sender's witness must hold inside the token contracts — CalledByEntry dies
		// at the router boundary. CustomContracts naming the router, both tokens,
		// and the pair (same discipline as the GAS/FTB seed).
		seedAct, err := actor.New(cli, []actor.SignerAccount{{
			Signer: transaction.Signer{
				Account:          sender,
				Scopes:           transaction.CustomContracts,
				AllowedContracts: []util.Uint160{router, ftbHash, neoHash, pair},
			},
			Account: acc,
		}})
		check(err)
		deadline := big.NewInt(time.Now().UnixMilli() + 10*60*1000)
		// first injection: amounts taken as-is (no quote), min bounds = desired
		txh, vub, err := seedAct.SendCall(router, "addLiquidity", sender, ftbHash, neoHash, seedFTB, seedNEO, seedFTB, seedNEO, deadline)
		check(err)
		_, err = seedAct.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-22s tx 0x%s\n", "addLiquidity", txh.StringLE())
		fmt.Printf("seeded pool: %s FTB + %s NEO (1 NEO = 40 FTB)\n", seedFTB, seedNEO)
	}

	fmt.Printf("\nNEOPAIR_HASH \"0x%s\"\n", pair.StringLE())
}
