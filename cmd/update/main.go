// update pushes a freshly compiled NEF + manifest onto an ALREADY-DEPLOYED contract,
// keeping its hash and storage. This is the designed pre-launch fix path: the FTB
// contracts expose an owner-guarded Update method that MUST be renounced before value
// flows. Use it for constants compiled into the NEF (like maxSupply) that must change
// without breaking every hash reference downstream.
//
//	go run ./cmd/update -config config.json -contract ftb_token -hash 0xdc82bb0c...
//
// Written to the neo-go v0.123 SDK. The update fee is execution + storage only (no
// 10 GAS deploy fee), so it is far cheaper than a redeploy.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

type config struct {
	RPC          string `json:"rpc"`
	Wallet       string `json:"wallet"`
	Password     string `json:"password"`
	PasswordEnv  string `json:"password_env"`
	ContractsDir string `json:"contracts_dir"`
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy config JSON (same shape as cmd/deploy)")
	name := flag.String("contract", "", "contract name in contracts_dir (e.g. ftb_token)")
	hashStr := flag.String("hash", "", "on-chain contract hash, canonical 0x form")
	flag.Parse()
	if *name == "" || *hashStr == "" {
		fmt.Fprintln(os.Stderr, "need -contract and -hash")
		os.Exit(2)
	}

	var cfg config
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(json.Unmarshal(raw, &cfg))

	// canonical 0x form decodes with Uint160DecodeStringLE in this SDK (see the
	// byte-order note in cmd/deploy) — BE decoding silently reverses the hash.
	target, err := util.Uint160DecodeStringLE(strings.TrimPrefix(strings.TrimSpace(*hashStr), "0x"))
	check(err)

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

	// self-check: the target must be on-chain, and the wallet must be able to sign for
	// the owner (the contract's Update method enforces it; fail here before burning fees).
	if _, err := cli.GetContractStateByAddressOrName("0x" + target.StringLE()); err != nil {
		check(fmt.Errorf("contract 0x%s not found on-chain: %w", target.StringLE(), err))
	}

	nefBytes, err := os.ReadFile(filepath.Join(cfg.ContractsDir, *name+".nef")) // #nosec G304 -- operator tool: args are the operator's own CLI input
	check(err)
	manifestBytes, err := os.ReadFile(filepath.Join(cfg.ContractsDir, *name+".manifest.json")) // #nosec G304
	check(err)

	act, err := actor.NewSimple(cli, acc)
	check(err)
	// the contract's own owner-guarded update(nef, manifest, data) — lowercase, as the
	// compiler writes exported Go methods into the manifest. nil data means
	// _deploy(isUpdate=true) just returns, preserving all storage.
	txh, vub, err := act.SendCall(target, "update", nefBytes, manifestBytes, nil)
	check(err)
	aer, err := act.WaitSuccess(context.Background(), txh, vub, err)
	check(err)
	fmt.Printf("updated %s at 0x%s  tx 0x%s  gas %d\n", *name, target.StringLE(), txh.StringLE(), aer.GasConsumed)
	fmt.Println("verify the contract answers with the new behavior (e.g. maxSupply()) before moving on.")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
