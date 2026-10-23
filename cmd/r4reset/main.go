// r4reset stands up the R4 reset stack on TestNet: token v2 (claimed ledger inside the
// token) + migrator R4 (stateless proof-checker) + fresh lend/dividends/bondvault/
// treasury (R2 manifest variants — same audited code, new names, so the reset starts
// with clean state instead of inheriting old-FTB accounting). The registry, ballots,
// oracle, and the AMM whitelist/factory/router are REUSED; the registry re-points its
// module keys through the timelocked Propose/Apply seam.
//
// Phases (resume-safe; every hash is recomputed from build/ and verified with
// getcontractstate before use):
//
//	FTB_DEPLOY_PASS=... go run ./cmd/r4reset -phase deploy   # 6 deploys
//	FTB_DEPLOY_PASS=... go run ./cmd/r4reset -phase wire     # setMigrator + configures
//	FTB_DEPLOY_PASS=... go run ./cmd/r4reset -phase propose  # registry re-point (timelock starts)
//	FTB_DEPLOY_PASS=... go run ./cmd/r4reset -phase apply    # anyone can apply, once the delay elapses
//	FTB_DEPLOY_PASS=... go run ./cmd/r4reset -phase all      # everything, waiting out the timelock
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/management"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

// The live TestNet registry (deployed 18 Oct, bootstrap finished, 20-block timelock) and
// the published v2 snapshot root (5,194 wallets — the SAME root the ballots and the R3
// migrator use, so one proof carries both the claim and the vote). The older
// investigation root (9b4be5c9…, 4,830 wallets) is historical; do not deploy with it.
const defaultRegistry = "0x0d59f3bb8018f5668ac4448249f21927fe961614"
const defaultRoot = "9a36b6a0faf6adddf727788e5a35ba9db48db14958043637fad67fb468243003"

// deploySet: artifact name in build/ -> registry key it answers to. Order is readability
// only; hashes are content-derived.
var moduleArtifacts = map[string]string{ // artifact -> registry key
	"lend_r2":      "lend",
	"bondvault_r2": "bondvault",
	"treasury_r2":  "treasury",
	"dividends_r2": "dividends",
}
var moduleOrder = []string{"lend_r2", "bondvault_r2", "treasury_r2", "dividends_r2"}

func main() {
	rpcURL := flag.String("rpc", "https://testnet1.neo.coz.io:443", "TestNet RPC")
	walletPath := flag.String("wallet", "cmd/deploy/testnet.wallet.json", "deploy wallet (NEP-6)")
	passEnv := flag.String("passenv", "FTB_DEPLOY_PASS", "env var holding the wallet password")
	contractsDir := flag.String("dir", "build", "compiled artifacts directory")
	rootHex := flag.String("root", defaultRoot, "snapshot Merkle root, hex")
	registryStr := flag.String("registry", defaultRegistry, "live registry contract hash")
	timelockBlocks := flag.Int("timelock", 20, "registry timelock, blocks (must match the on-chain delay)")
	migratorArtifact := flag.String("migrator-artifact", "ftb_migrator", "migrator artifact in build/ (ftb_migrator_r4 after a bricked configure forces a fresh name)")
	phase := flag.String("phase", "all", "deploy | wire | propose | apply | all")
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

	// Canonical 0x form (StringLE), same byte-order discipline as cmd/deploy.
	getState := func(h util.Uint160) (*state.Contract, error) {
		return cli.GetContractStateByAddressOrName("0x" + h.StringLE())
	}
	hashOf := func(name string) util.Uint160 {
		ne, m := load(*contractsDir, name)
		return state.CreateContractHash(sender, ne.Checksum, m.Name)
	}
	deploy := func(name string, data any) util.Uint160 {
		h := hashOf(name)
		if _, err := getState(h); err == nil {
			fmt.Printf("reusing  %-14s -> 0x%s (already on-chain)\n", name, h.StringLE())
			return h
		}
		ne, m := load(*contractsDir, name)
		txh, vub, err := mgmt.Deploy(&ne, m, data)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		if _, err := getState(h); err != nil {
			check(fmt.Errorf("deploy of %s: tx 0x%s HALTed but contract 0x%s not found on-chain: %w",
				name, txh.StringLE(), h.StringLE(), err))
		}
		fmt.Printf("deployed %-14s -> 0x%s  tx 0x%s\n", name, h.StringLE(), txh.StringLE())
		return h
	}
	call := func(h util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(h, method, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-16s tx 0x%s\n", method, txh.StringLE())
	}

	registry := mustHash(*registryStr)
	if _, err := getState(registry); err != nil {
		check(fmt.Errorf("registry 0x%s not found on-chain: %w", registry.StringLE(), err))
	}
	root, err := hex.DecodeString(strip0x(*rootHex))
	check(err)
	if len(root) != 32 {
		check(fmt.Errorf("merkle root must be 32 bytes, got %d", len(root)))
	}

	token := hashOf("ftb_token")
	migrator := hashOf(*migratorArtifact)
	mods := map[string]util.Uint160{}
	for _, n := range moduleOrder {
		mods[n] = hashOf(n)
	}

	if *phase == "deploy" || *phase == "all" {
		owner := sender.BytesBE() // TestNet: deployer is the owner (FTB-14 caveat)
		token = deploy("ftb_token", owner)
		migrator = deploy(*migratorArtifact, owner)
		for _, n := range moduleOrder {
			mods[n] = deploy(n, owner)
		}
	} else {
		for name, h := range map[string]util.Uint160{"ftb_token": token, *migratorArtifact: migrator} {
			if _, err := getState(h); err != nil {
				check(fmt.Errorf("%s 0x%s not on-chain; run -phase deploy first: %w", name, h.StringLE(), err))
			}
		}
		for _, n := range moduleOrder {
			if _, err := getState(mods[n]); err != nil {
				check(fmt.Errorf("%s 0x%s not on-chain; run -phase deploy first: %w", n, mods[n].StringLE(), err))
			}
		}
	}

	if *phase == "wire" || *phase == "all" {
		call(token, "setMigrator", migrator)
		call(migrator, "configure", token, root)
		call(mods["lend_r2"], "configure", registry, token)
		call(mods["dividends_r2"], "configure", token)
		call(mods["bondvault_r2"], "configure", token, sender)
		call(mods["treasury_r2"], "configure", token, sender, registry)
	}

	// wiremigrator: the recovery path for a bricked migrator configure (write-once root).
	// Deploys nothing; re-points the token's minter and the registry's pending migrator
	// key to the -migrator-artifact variant, configured with the correct root.
	if *phase == "wiremigrator" {
		call(token, "setMigrator", migrator)
		call(migrator, "configure", token, root)
		call(registry, "propose", "migrator", migrator)
	}

	proposeAll := func() {
		call(registry, "propose", "token", token)
		call(registry, "propose", "migrator", migrator)
		for _, n := range moduleOrder {
			call(registry, "propose", moduleArtifacts[n], mods[n])
		}
	}
	applyAll := func() {
		call(registry, "apply", "token")
		call(registry, "apply", "migrator")
		for _, n := range moduleOrder {
			call(registry, "apply", moduleArtifacts[n])
		}
	}

	if *phase == "propose" || *phase == "all" {
		proposeAll()
	}
	if *phase == "apply" {
		applyAll()
	}
	if *phase == "all" {
		// wait out the timelock, then apply (permissionless); on any wait failure print
		// the manual calls — never hard-fail after successful deploys.
		start, err := cli.GetBlockCount()
		if err != nil {
			fmt.Println("could not read block height; apply manually after the timelock")
		} else {
			target := start + uint32(*timelockBlocks) + 1 // #nosec G115 -- small positive operator value
			fmt.Printf("waiting out the %d-block timelock (target height %d)...\n", *timelockBlocks, target)
			deadline := time.Now().Add(15 * time.Minute)
			for time.Now().Before(deadline) {
				h, err := cli.GetBlockCount()
				if err == nil && h > target {
					break
				}
				time.Sleep(15 * time.Second)
			}
			if h, err := cli.GetBlockCount(); err == nil && h > target {
				applyAll()
			} else {
				fmt.Println("timelock wait timed out; re-run with -phase apply once the delay elapses")
			}
		}
	}

	fmt.Printf("\nR4_HASHES {\"ftb\":\"0x%s\",\"migrator\":\"0x%s\",\"registry\":\"0x%s\",\"lend\":\"0x%s\",\"bondvault\":\"0x%s\",\"treasury\":\"0x%s\",\"dividends\":\"0x%s\"}\n",
		token.StringLE(), migrator.StringLE(), registry.StringLE(),
		mods["lend_r2"].StringLE(), mods["bondvault_r2"].StringLE(), mods["treasury_r2"].StringLE(), mods["dividends_r2"].StringLE())
	fmt.Println("Next: pairs recompiled against the new FTB hash (contracts/amm-flamingo),")
	fmt.Println("factory.registerExchangePair re-points each pool, then seed deep and publish config.")
}

func load(dir, name string) (nef.File, *manifest.Manifest) {
	nb, err := os.ReadFile(filepath.Join(dir, name+".nef")) // #nosec G304 -- operator tool: dir and name are the operator's own CLI args, not remote input
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(dir, name+".manifest.json")) // #nosec G304 -- operator tool: dir and name are the operator's own CLI args, not remote input
	check(err)
	m := &manifest.Manifest{}
	check(json.Unmarshal(mb, m))
	return ne, m
}

func strip0x(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}

// mustHash parses a canonical (StringLE) 0x contract hash — the exact form RPC calls
// and config.js expect (see the byte-order note in cmd/deploy).
func mustHash(s string) util.Uint160 {
	h, err := util.Uint160DecodeStringLE(strip0x(strings.TrimSpace(s)))
	check(err)
	return h
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
