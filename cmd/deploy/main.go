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
	"time"

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
	// Launch2, when true, also deploys the Launch-2 suite (oracle, lend, bondvault,
	// treasury, dividends), configures each against the core, and registers them on
	// the seam board: Propose now, Apply once the timelock elapses (the driver waits).
	// Single-key path only; a multisig owner gets the core printed wiring instead.
	Launch2 bool `json:"launch2"`
	// Core, when set, skips deploying the core and reuses already-deployed contract
	// hashes — resume mode for Launch 2 on a chain where the core is already live
	// (N3 contract hashes are deterministic on sender+nef+name, so a redeploy of an
	// unchanged core would be rejected as "already exists"). Every hash is verified
	// with getcontractstate before any wiring call uses it.
	Core *coreReuse `json:"core"`
}

type coreReuse struct {
	FTB      string `json:"ftb"`
	Migrator string `json:"migrator"`
	Registry string `json:"registry"`
	Ballot   string `json:"ballot"` // optional; skips the ballot deploy when set
}

func main() {
	cfgPath := flag.String("config", "config.json", "deploy config JSON")
	phase := flag.String("phase", "all", "launch-2 step: all | deploy | wire | apply (see launch-2 comment)")
	flag.Parse()
	if *phase != "all" && *phase != "deploy" && *phase != "wire" && *phase != "apply" {
		check(fmt.Errorf("unknown phase %q: want all, deploy, wire, or apply", *phase))
	}

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

	// BYTE ORDER (verified on TestNet against the C# RPCs): this SDK's Uint160 stores
	// the hash digest bytes as-is; the canonical "0x..." form that RPCs, explorers, and
	// config.js use is StringLE() — the byte-reverse of StringBE(). All human-facing
	// output and every RPC string query below use StringLE(). Contract hashes decoded
	// FROM canonical strings must use Uint160DecodeStringLE for the same reason.
	// (GetContractStateByHash is unusable against C# RPCs: it sends bare hex they
	// reject as "Unknown contract" -102, so query by the canonical 0x string.)
	getState := func(h util.Uint160) (*state.Contract, error) {
		return cli.GetContractStateByAddressOrName("0x" + h.StringLE())
	}

	deploy := func(name string, data any) util.Uint160 {
		ne, m := load(cfg.ContractsDir, name)
		h := state.CreateContractHash(sender, ne.Checksum, m.Name) // hash derives from the deploy TX sender
		txh, vub, err := mgmt.Deploy(&ne, m, data)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		// self-check: the contract must answer getcontractstate at the computed hash
		// before anything downstream (wiring calls, config.js) uses it.
		if _, err := getState(h); err != nil {
			check(fmt.Errorf("deploy of %s: tx 0x%s HALTed but contract 0x%s not found on-chain: %w",
				name, txh.StringLE(), h.StringLE(), err))
		}
		// canonical 0x form (StringLE) — the one RPC calls and explorers expect.
		fmt.Printf("deployed %-12s -> 0x%s  tx 0x%s\n", name, h.StringLE(), txh.StringLE())
		return h
	}

	call := func(hash util.Uint160, method string, args ...any) {
		txh, vub, err := act.SendCall(hash, method, args...)
		check(err)
		_, err = act.WaitSuccess(context.Background(), txh, vub, err)
		check(err)
		fmt.Printf("  wired %-16s tx 0x%s\n", method, txh.StringLE())
	}

	var token, migrator, registry, ballot util.Uint160
	hasBallot := false

	if cfg.Core != nil {
		// Resume mode: the core is already live on this chain (a redeploy of the
		// unchanged core would be rejected — the hash is deterministic), so reuse the
		// published hashes after verifying each one answers getcontractstate.
		cr := cfg.Core
		token = mustHash("core.ftb", cr.FTB)
		migrator = mustHash("core.migrator", cr.Migrator)
		registry = mustHash("core.registry", cr.Registry)
		existing := map[string]util.Uint160{"ftb": token, "migrator": migrator, "registry": registry}
		if cr.Ballot != "" {
			ballot = mustHash("core.ballot", cr.Ballot)
			existing["ballot"] = ballot
			hasBallot = true
		}
		for name, h := range existing {
			if _, err := getState(h); err != nil {
				check(fmt.Errorf("core %s 0x%s not found on-chain: %w", name, h.StringLE(), err))
			}
		}
		fmt.Printf("core: reusing live contracts (ftb 0x%s, migrator 0x%s, registry 0x%s)\n",
			token.StringLE(), migrator.StringLE(), registry.StringLE())
		if !cfg.Launch2 {
			check(fmt.Errorf("core reuse is set but launch2 is false: nothing to do"))
		}
		if !ownerIsSigner {
			check(fmt.Errorf("reuse mode: the launch-2 wiring needs the owner's witness, and this wallet is not the owner; run the launch-2 calls from the owner (see ops/DECENTRALIZED_OPS.md)"))
		}
	} else {
		// 1. deploy core (owner passed explicitly as deploy data; the multisig does not need
		// to sign anything yet, _deploy just stores the hash)
		token = deploy("ftb_token", owner.BytesBE())
		migrator = deploy("ftb_migrator", owner.BytesBE())
		registry = deploy("registry", owner.BytesBE())

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
		if cfg.BallotOptions >= 2 {
			ballot = deploy("ballot", owner.BytesBE())
			call(ballot, "configure", root, cfg.BallotOptions)
			hasBallot = true
		}
	}

	// 6. Launch-2 suite (optional): oracle, lend, bondvault, treasury, dividends.
	// Bootstrap is already finished on-chain, so the modules join the seam board through
	// the timelock: propose now, apply once the delay elapses.
	//
	// The phase flag splits this into operator-sized steps, because a full run (5
	// deploys, 11 wiring calls, a 20-block timelock wait) outlives a single shell
	// session. Hashes are deterministic (sender + nef checksum + manifest name), so
	// each phase re-derives them from the build artifacts — no state file, and a
	// re-run after a crash picks up exactly where the chain is:
	//
	//	go run ./cmd/deploy -config c.json -phase deploy   # deploy the 5 contracts
	//	go run ./cmd/deploy -config c.json -phase wire     # configure + propose
	//	# wait for the timelock (delay blocks), then:
	//	go run ./cmd/deploy -config c.json -phase apply    # anyone can apply
	//
	// -phase all (default) does everything, waiting out the timelock itself.
	l2 := map[string]util.Uint160{}
	if cfg.Launch2 {
		order := []string{"oracle", "lend", "bondvault", "treasury", "dividends"}
		l2hash := func(name string) util.Uint160 {
			ne, m := load(cfg.ContractsDir, name)
			return state.CreateContractHash(sender, ne.Checksum, m.Name)
		}
		if *phase == "all" || *phase == "deploy" {
			for _, n := range order {
				h := l2hash(n)
				if _, err := getState(h); err == nil {
					fmt.Printf("reusing  %-12s -> 0x%s (already on-chain)\n", n, h.StringLE())
					l2[n] = h
					continue
				}
				l2[n] = deploy(n, owner.BytesBE())
			}
		} else {
			for _, n := range order {
				h := l2hash(n)
				if _, err := getState(h); err != nil {
					check(fmt.Errorf("launch-2 %s 0x%s not on-chain; run -phase deploy first: %w", n, h.StringLE(), err))
				}
				l2[n] = h
			}
		}
		if *phase == "all" || *phase == "wire" {
			call(l2["oracle"], "setReporter", sender, true) // TestNet: deployer is the reporter; real quorum before MainNet
			call(l2["lend"], "configure", registry, token)
			call(l2["bondvault"], "configure", token, owner) // committee = owner on single-key TestNet
			call(l2["treasury"], "configure", token, owner, registry)
			call(l2["dividends"], "configure", token)
			for _, n := range order {
				call(registry, "propose", n, l2[n])
			}
		}
		if *phase == "apply" {
			// The caller has waited out the timelock; apply is permissionless.
			for _, n := range order {
				call(registry, "apply", n)
			}
		}
		if *phase == "all" {
			// Wait out the timelock, then apply. If the wait fails, print the manual
			// calls — never hard-fail after successful deploys (anyone can apply).
			start, err := cli.GetBlockCount()
			if err != nil {
				fmt.Println("could not read block height; apply manually after the timelock:")
				for _, n := range order {
					fmt.Printf("  %s apply %q\n", registry.StringLE(), n)
				}
			} else {
				target := start + uint32(cfg.TimelockBlocks) + 1 // #nosec G115 -- timelock_blocks is a small positive operator config value
				fmt.Printf("waiting out the %d-block timelock (target height %d)...\n", cfg.TimelockBlocks, target)
				deadline := time.Now().Add(15 * time.Minute)
				for {
					h, err := cli.GetBlockCount()
					if err == nil && h > target {
						break
					}
					if time.Now().After(deadline) {
						fmt.Println("timelock wait timed out; apply manually when ready:")
						for _, n := range order {
							fmt.Printf("  %s apply %q\n", registry.StringLE(), n)
						}
						break
					}
					time.Sleep(15 * time.Second)
				}
				if h, err := cli.GetBlockCount(); err == nil && h > target {
					for _, n := range order {
						call(registry, "apply", n)
					}
				}
			}
		}
	}

	fmt.Println("\nDeployed and wired.")
	// machine-readable line for scripts/deploy_testnet.sh to build web/config.js;
	// hashes are 0x-prefixed canonical (StringLE), the exact form RPC calls and
	// config.js expect.
	ballotStr := ""
	if hasBallot {
		ballotStr = "0x" + ballot.StringLE()
	}
	l2h := func(k string) string {
		if h, ok := l2[k]; ok {
			return "0x" + h.StringLE()
		}
		return ""
	}
	fmt.Printf("FTB_HASHES {\"ftb\":%q,\"migrator\":%q,\"registry\":%q,\"ballot\":%q,\"oracle\":%q,\"lend\":%q,\"bondvault\":%q,\"treasury\":%q,\"dividends\":%q}\n",
		"0x"+token.StringLE(), "0x"+migrator.StringLE(), "0x"+registry.StringLE(), ballotStr,
		l2h("oracle"), l2h("lend"), l2h("bondvault"), l2h("treasury"), l2h("dividends"))
	if cfg.OwnerMultisig == nil && cfg.OwnerScriptHash == "" {
		fmt.Println("WARNING: the owner is the deployer's SINGLE key. Dev/TestNet only;")
		fmt.Println("for any real launch set owner_multisig in the config and redeploy (FTB-14).")
	}
	fmt.Println("Next: run the migration window (holders claim), then multisig-sign")
	fmt.Println("migrator.close to seal the mint forever, then multisig-sign renounceOwnership.")
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

// mustHash parses a 0x-prefixed (or bare) canonical contract hash from a config field,
// exiting on any malformed value — a wrong hash here would wire money to nothing.
// Canonical (RPC/explorer) form decodes with Uint160DecodeStringLE in this SDK; the
// BE variant would silently yield the byte-reversed scripthash (see the byte-order
// note in main).
func mustHash(field, s string) util.Uint160 {
	h, err := util.Uint160DecodeStringLE(strip0x(strings.TrimSpace(s)))
	check(err)
	return h
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
		fmt.Printf("owner: %d-of-%d multisig  %s  (0x%s)\n", mo.M, n,
			address.Uint160ToString(owner), owner.StringLE())
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
			owner, err = util.Uint160DecodeStringLE(strip0x(s)) // canonical 0x form (see byte-order note)
		}
		check(err)
		fmt.Printf("owner: %s  (0x%s)\n", address.Uint160ToString(owner), owner.StringLE())
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
	fmt.Printf("  1. %s setMigrator 0x%s\n", token.StringLE(), migrator.StringLE())
	fmt.Printf("  2. %s configure 0x%s 0x%s\n", migrator.StringLE(), token.StringLE(), hex.EncodeToString(root))
	fmt.Printf("  3. %s setInitialDelay %d\n", registry.StringLE(), cfg.TimelockBlocks)
	fmt.Printf("  4. %s bootstrap \"token\" 0x%s\n", registry.StringLE(), token.StringLE())
	fmt.Printf("  5. %s bootstrap \"migrator\" 0x%s\n", registry.StringLE(), migrator.StringLE())
	fmt.Printf("  6. %s finishBootstrap\n", registry.StringLE())
	fmt.Println("After the migration window, in order and multisig-signed:")
	fmt.Printf("  7. %s close            (seals the mint forever)\n", migrator.StringLE())
	fmt.Printf("  8. %s renounceOwnership (makes the token immutable; close FIRST, FTB-03)\n", token.StringLE())
	fmt.Println("Publish this output next to proofs.json so anyone can verify the wiring.")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
