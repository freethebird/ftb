// deploy stands up and wires the FTB core on a Neo N3 node (neo-express, TestNet, then
// MainNet): it deploys the token, migrator, and registry, points the token at the
// migrator, configures the migrator with the published Merkle root, and bootstraps the
// registry seam behind a timelock. One language through to orchestration.
//
// Pre-req: compile each contract first with `neo-go contract compile`, placing the .nef
// and .manifest.json in config.contracts_dir.
//
//	go run ./cmd/deploy -config config.json
//
// Written to the neo-go v0.123 SDK. SDK signatures shift between versions, so verify
// against your pinned neo-go; the stable alternative is the neo-go CLI runbook in README.
package main

import (
	"context"
	"crypto/elliptic"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/crypto/hash"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/management"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/nef"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
)

// multisigConfig names the published owner multisig by its members' public keys, so the
// deploy tool computes the owner hash itself instead of trusting a hand-copied address.
type multisigConfig struct {
	M       int      `json:"m"`
	PubKeys []string `json:"pubkeys"` // compressed P-256, hex, with or without 0x
}

type config struct {
	RPC            string `json:"rpc"`
	Wallet         string `json:"wallet"`
	Password       string `json:"password"`     // fallback; prefer password_env (FTB-12)
	PasswordEnv    string `json:"password_env"` // name of the env var holding the password
	ContractsDir   string `json:"contracts_dir"`
	MerkleRootHex  string `json:"merkle_root_hex"`
	TimelockBlocks int    `json:"timelock_blocks"`
	// Owner (audit item FTB-14): who the contracts' admin is from the first block. Set
	// owner_multisig (preferred) or owner_scripthash. If neither is set, the signing
	// wallet's single key becomes the owner, which is fine for dev but NOT for any launch.
	OwnerScriptHash string          `json:"owner_scripthash"` // N-address or 0x BE hash
	OwnerMultisig   *multisigConfig `json:"owner_multisig"`
	// BallotOptions, when >= 2, also deploys the governance ballot and configures it
	// write-once with the same Merkle root (single-key path only; with a multisig owner
	// the ballot deploy/configure is printed as multisig calls instead).
	BallotOptions int `json:"ballot_options"`
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy config JSON")
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
	check(acc.Decrypt(resolvePassword(cfg), w.Scrypt))
	sender := acc.ScriptHash()

	// Audit item FTB-14: the owner (admin authority) is decoupled from the signing wallet.
	// For any real launch the owner is the published multisig, set here from its members'
	// public keys (or a raw scripthash), so the contracts are born with the multisig as
	// owner and no single hot key ever holds admin power.
	owner, ownerIsSigner := resolveOwner(cfg, sender)

	act, err := actor.NewSimple(cli, acc)
	check(err)
	mgmt := management.New(act)

	deploy := func(name string, data any) util.Uint160 {
		ne, m := load(cfg.ContractsDir, name)
		h := state.CreateContractHash(sender, ne.Checksum, m.Name) // hash derives from the deploy TX sender
		txh, vub, err := mgmt.Deploy(&ne, m, data)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		// print both byte orders so there is no 0x BE vs LE ambiguity when you paste it
		fmt.Printf("deployed %-12s -> 0x%x  (le 0x%x)\n", name, h.BytesBE(), h.BytesLE())
		return h
	}

	// 1. deploy core (owner passed explicitly as deploy data; the multisig does not need
	// to sign anything yet, _deploy just stores the hash)
	token := deploy("ftb_token", owner.BytesBE())
	migrator := deploy("ftb_migrator", owner.BytesBE())
	registry := deploy("registry", owner.BytesBE())

	// 2. validate the migrator's snapshot root here so both wiring paths, automatic and
	// printed, use the same checked value
	root, err := hex.DecodeString(strip0x(cfg.MerkleRootHex))
	check(err)
	if len(root) != 32 {
		check(fmt.Errorf("merkle root must be 32 bytes, got %d", len(root)))
	}

	if !ownerIsSigner {
		// The owner is the multisig and this wallet cannot sign for it, which is exactly
		// the point. Print the wiring as ready-to-execute calls for the multisig members.
		printMultisigWiring(cfg, token, migrator, registry, root, owner)
		return
	}

	call := func(hash util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(hash, method, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %s\n", method)
	}

	// 2. the token's only minter is the migrator
	call(token, "setMigrator", migrator)

	// 3. configure the migrator (write-once, FTB-02: double-check root before running)
	call(migrator, "configure", token, root)

	// 4. seam board: set the timelock, record the modules, finish bootstrap
	call(registry, "setInitialDelay", cfg.TimelockBlocks)
	call(registry, "bootstrap", "token", token)
	call(registry, "bootstrap", "migrator", migrator)
	call(registry, "finishBootstrap")

	// 5. governance ballot (optional): one deployment per proposal, configured
	// write-once with the SAME Merkle root, so holders vote with their claim proof.
	var ballot util.Uint160
	if cfg.BallotOptions >= 2 {
		ballot = deploy("ballot", owner.BytesBE())
		call(ballot, "configure", root, cfg.BallotOptions)
	}

	fmt.Println("\nDeployed and wired.")
	// machine-readable line for scripts/deploy_testnet.sh to build web/config.js
	ballotStr := ""
	if cfg.BallotOptions >= 2 {
		ballotStr = ballot.String()
	}
	fmt.Printf("FTB_HASHES {\"ftb\":%q,\"migrator\":%q,\"registry\":%q,\"ballot\":%q}\n",
		token.String(), migrator.String(), registry.String(), ballotStr)
	if cfg.OwnerMultisig == nil && cfg.OwnerScriptHash == "" {
		fmt.Println("WARNING: the owner is the deployer's SINGLE key. Dev/TestNet only;")
		fmt.Println("for any real launch set owner_multisig in the config and redeploy (FTB-14).")
	}
	fmt.Println("Next: run the migration window (holders claim), then multisig-sign")
	fmt.Println("migrator.close to seal the mint forever, then multisig-sign renounceOwnership.")
}

func load(dir, name string) (nef.File, *manifest.Manifest) {
	nb, err := os.ReadFile(filepath.Join(dir, name+".nef"))
	check(err)
	ne, err := nef.FileFromBytes(nb)
	check(err)
	mb, err := os.ReadFile(filepath.Join(dir, name+".manifest.json"))
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

// resolvePassword reads the wallet password from the environment when password_env is set
// (FTB-12: keep secrets out of config files), falling back to the plaintext field.
func resolvePassword(cfg config) string {
	if cfg.PasswordEnv != "" {
		p := os.Getenv(cfg.PasswordEnv)
		if p == "" {
			check(fmt.Errorf("password_env %q is set in config but empty in the environment", cfg.PasswordEnv))
		}
		return p
	}
	return cfg.Password
}

// resolveOwner decides who administers the contracts from their first block. Returns the
// owner hash and whether the signing wallet can sign for it (single-key dev mode).
func resolveOwner(cfg config, sender util.Uint160) (util.Uint160, bool) {
	if cfg.OwnerMultisig != nil && cfg.OwnerScriptHash != "" {
		check(fmt.Errorf("set owner_multisig OR owner_scripthash, not both"))
	}
	if cfg.OwnerMultisig != nil {
		mo := cfg.OwnerMultisig
		n := len(mo.PubKeys)
		if mo.M < 2 || n < mo.M {
			check(fmt.Errorf("owner_multisig: need m >= 2 and at least m pubkeys, got m=%d, n=%d", mo.M, n))
		}
		pubs := make(keys.PublicKeys, n)
		for i, s := range mo.PubKeys {
			b, err := hex.DecodeString(strip0x(strings.TrimSpace(s)))
			check(err)
			p, err := keys.NewPublicKeyFromBytes(b, elliptic.P256())
			check(err)
			pubs[i] = p
		}
		script, err := smartcontract.CreateMultiSigRedeemScript(mo.M, pubs)
		check(err)
		owner := hash.Hash160(script)
		fmt.Printf("owner: %d-of-%d multisig  %s  (0x%x)\n", mo.M, n,
			address.Uint160ToString(owner), owner.BytesBE())
		fmt.Println("verify every member independently derives this SAME address from the pubkey set")
		return owner, owner == sender
	}
	if cfg.OwnerScriptHash != "" {
		s := strings.TrimSpace(cfg.OwnerScriptHash)
		var (
			owner util.Uint160
			err   error
		)
		if strings.HasPrefix(s, "N") {
			owner, err = address.StringToUint160(s)
		} else {
			owner, err = util.Uint160DecodeStringBE(strip0x(s))
		}
		check(err)
		fmt.Printf("owner: %s  (0x%x)\n", address.Uint160ToString(owner), owner.BytesBE())
		return owner, owner == sender
	}
	fmt.Println("owner: deployer wallet Accounts[0] (single key) — acceptable for dev/TestNet only")
	return sender, true
}

// printMultisigWiring emits the exact owner-authorized calls the multisig must execute,
// with the concrete hashes filled in, since this wallet cannot sign for the multisig owner.
func printMultisigWiring(cfg config, token, migrator, registry util.Uint160, root []byte, owner util.Uint160) {
	fmt.Println("\nDeployed with the multisig as owner. The remaining wiring needs the owner's")
	fmt.Println("witness, so the multisig members execute it (see ops/DECENTRALIZED_OPS.md).")
	fmt.Println("Calls, in order, each signed by the multisig:")
	fmt.Printf("  1. %s setMigrator 0x%x\n", token.StringBE(), migrator.BytesBE())
	fmt.Printf("  2. %s configure 0x%x 0x%s\n", migrator.StringBE(), token.BytesBE(), hex.EncodeToString(root))
	fmt.Printf("  3. %s setInitialDelay %d\n", registry.StringBE(), cfg.TimelockBlocks)
	fmt.Printf("  4. %s bootstrap \"token\" 0x%x\n", registry.StringBE(), token.BytesBE())
	fmt.Printf("  5. %s bootstrap \"migrator\" 0x%x\n", registry.StringBE(), migrator.BytesBE())
	fmt.Printf("  6. %s finishBootstrap\n", registry.StringBE())
	fmt.Println("After the migration window, in order and multisig-signed:")
	fmt.Printf("  7. %s close            (seals the mint forever)\n", migrator.StringBE())
	fmt.Printf("  8. %s renounceOwnership (makes the token immutable; close FIRST, FTB-03)\n", token.StringBE())
	fmt.Println("Publish this output next to proofs.json so anyone can verify the wiring.")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
