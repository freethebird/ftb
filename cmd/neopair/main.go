// neopair adds the FTB/NEO pair to the community AMM on TestNet: deploys the
// constants-only FlamingoSwapPairFtbNeo variant (TokenA=FTB — the smaller hash,
// token0 by router convention — TokenB=NEO), registers it with the factory, and
// seeds 360 FTB + 5 NEO (ratio 72, consistent with NEO ≈ 1.8 GAS).
// Same self-check discipline as cmd/ammdeploy: the hash is recomputed from the
// build artifacts and verified with getcontractstate before use; a re-run skips
// what is already live.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/neopair -phase all
//
// Phases: deploy | wire (factory.registerExchangePair) | seed | all | freeze (setAdmin -> unspendable).
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

// seed: ratio 1 NEO = 72 FTB (R4 reset deep seed: 5 NEO + 360 FTB), consistent with
// NEO ≈ 1.8 GAS against the GAS pool's 1:40 - deep enough that the 0-decimal NEO
// expresses the ratio exactly (the 1-NEO first pool could not).
var (
	seedFTB = big.NewInt(36000000000) // 360 FTB (8 decimals)
	seedNEO = big.NewInt(5)           // 5 NEO (0 decimals, indivisible)
)

// native NEO (same on every N3 network), the TestNet FTB token (R4 reset: token v2),
// and the live AMM wiring from contracts/amm-flamingo/DEPLOYED.md.
var (
	neoHash = mustHash("0xef4073a0f2b305a38ec4050e4d3d28bc40ea63f5")
	ftbHash = mustHash("0xf85f93d672cbf79f7749fb50a2d46a48421d7e71")
	factory = mustHash("0xa809b0f523f7cef26b979707f23e849415dc970b")
	router  = mustHash("0x41ee590749816ef96a2adde02c375b8f64cb26af")
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
	phase := flag.String("phase", "all", "deploy | wire | seed | all | freeze")
	burnFlag := flag.String("burn", "0xfe3d48bab4eaf49c2102bfa5a979ff66d1da3822", "freeze target: unspendable admin (non-zero); default is an ABORT-script hash")
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
		fmt.Printf("seeded pool: %s FTB + %s NEO (1 NEO = 72 FTB)\n", seedFTB, seedNEO)
	}

	if *phase == "freeze" {
		// one-way: hand the NEO/FTB pair's admin to a provably-unspendable address.
		burn := mustHash(*burnFlag)
		txh, vub, err := act.SendCall(pair, "setAdmin", burn)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  frozen %-22s tx 0x%s (admin -> 0x%s)\n", "setAdmin", txh.StringLE(), burn.StringLE())
	}

	fmt.Printf("\nNEOPAIR_HASH \"0x%s\"\n", pair.StringLE())
}
