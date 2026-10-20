// ammdeploy stands up the community AMM (the reused, audited Flamingo
// Factory/Pair/Router plus the pair whitelist) on Neo N3 TestNet, wires it,
// and seeds the first GAS/FTB pool. Same SDK and self-check discipline as
// cmd/deploy: every hash is recomputed from the build artifacts and verified
// with getcontractstate before use; a re-run skips contracts already live.
//
//	FTB_DEPLOY_PASS=... go run ./cmd/ammdeploy -phase all
//
// Phases: deploy (4 contracts) | wire (whitelist.addRouter + factory.registerExchangePair)
// | seed (router.addLiquidity pulls GAS+FTB from the deploy wallet) | all.
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

const ammRoot = "contracts/amm-flamingo/Swap/flamingo-contract-swap"

// seed size for the community pool - deep enough that test swaps don't slip
// (R4 reset: 20 GAS + 800 FTB keeps the 1 GAS = 40 FTB placeholder ratio).
var (
	seedGAS = big.NewInt(2000000000)  // 20 GAS (8 decimals)
	seedFTB = big.NewInt(80000000000) // 800 FTB (8 decimals) — 1 GAS = 40 FTB
)

var artifacts = map[string]string{ // contract name -> dir holding <name>.nef / <name>.manifest.json
	"FlamingoSwapPairWhiteList": ammRoot + "/FlamingoSwapPairWhiteList/bin/sc",
	"FlamingoSwapFactory":       ammRoot + "/FlamingoSwapFactory/bin/sc",
	"FlamingoSwapPair":          ammRoot + "/FlamingoSwapPair/bin/sc",
	"FlamingoSwapRouter":        ammRoot + "/FlamingoSwapRouter/bin/sc",
}

// deploy order matters for readability only; hashes are content-derived.
var order = []string{"FlamingoSwapPairWhiteList", "FlamingoSwapFactory", "FlamingoSwapPair", "FlamingoSwapRouter"}

// native GAS (same on every N3 network) and the TestNet FTB token (R4 reset: token v2,
// claimed ledger inside the token).
var (
	gasHash = mustHash("0xd2a4cff31913016155e38e474a2c06d08be276cf")
	ftbHash = mustHash("0x1ff1f51e63179867367ae8982af583e01e7cbdbd")
)

func mustHash(s string) util.Uint160 {
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	h, err := util.Uint160DecodeStringLE(s)
	check(err)
	return h
}

func load(name string) (nef.File, *manifest.Manifest) {
	dir := artifacts[name]
	nb, err := os.ReadFile(filepath.Join(dir, name+".nef")) // #nosec G304 -- fixed repo paths
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(dir, name+".manifest.json")) // #nosec G304 -- fixed repo paths
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))
	return ne, m
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

	getState := func(h util.Uint160) (*state.Contract, error) {
		return cli.GetContractStateByAddressOrName("0x" + h.StringLE())
	}

	hashes := map[string]util.Uint160{}
	for _, name := range order {
		ne, m := load(name)
		hashes[name] = state.CreateContractHash(sender, ne.Checksum, m.Name)
	}

	deploy := func(name string) {
		ne, m := load(name)
		h := hashes[name]
		if _, err := getState(h); err == nil {
			fmt.Printf("live    %-26s -> 0x%s (skip)\n", name, h.StringLE())
			return
		}
		txh, vub, err := mgmt.Deploy(&ne, m, nil)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		if _, err := getState(h); err != nil {
			check(fmt.Errorf("deploy of %s: tx HALTed but 0x%s not found: %w", name, h.StringLE(), err))
		}
		fmt.Printf("deployed %-26s -> 0x%s  tx 0x%s\n", name, h.StringLE(), txh.StringLE())
	}

	call := func(h util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(h, method, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-22s tx 0x%s\n", method, txh.StringLE())
	}

	wl, fact, pair, router := hashes["FlamingoSwapPairWhiteList"], hashes["FlamingoSwapFactory"],
		hashes["FlamingoSwapPair"], hashes["FlamingoSwapRouter"]

	if *phase == "deploy" || *phase == "all" {
		for _, name := range order {
			deploy(name)
		}
	}
	if *phase == "wire" || *phase == "all" {
		// pairs only accept calls from whitelisted routers; the factory maps GAS/FTB -> the pair
		call(wl, "addRouter", router)
		call(fact, "registerExchangePair", pair)
	}
	if *phase == "seed" || *phase == "all" {
		// the router PULLS both tokens from the sender inside addLiquidity, so the
		// sender's witness must hold inside the token contracts — CalledByEntry dies
		// at the router boundary. CustomContracts naming the router, both tokens, and
		// the pair is exactly the scope the Flamingo front-end used for this call.
		seedAct, err := actor.New(cli, []actor.SignerAccount{{
			Signer: transaction.Signer{
				Account:          sender,
				Scopes:           transaction.CustomContracts,
				AllowedContracts: []util.Uint160{router, gasHash, ftbHash, pair},
			},
			Account: acc,
		}})
		check(err)
		deadline := big.NewInt(time.Now().UnixMilli() + 10*60*1000)
		// first injection: amounts are taken as-is (no quote), min bounds = desired
		txh, vub, err := seedAct.SendCall(router, "addLiquidity", sender, gasHash, ftbHash, seedGAS, seedFTB, seedGAS, seedFTB, deadline)
		check(err)
		_, err = seedAct.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-22s tx 0x%s\n", "addLiquidity", txh.StringLE())
		fmt.Printf("seeded pool: %s GAS + %s FTB\n", seedGAS, seedFTB)
	}

	fmt.Printf("\nAMM_HASHES {\"whitelist\":\"0x%s\",\"factory\":\"0x%s\",\"pair\":\"0x%s\",\"router\":\"0x%s\"}\n",
		wl.StringLE(), fact.StringLE(), pair.StringLE(), router.StringLE())
}
