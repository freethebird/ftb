package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/freethebird/ftb/pkg/chain/neon3"
	"github.com/freethebird/ftb/pkg/chain/neox"
	"github.com/freethebird/ftb/pkg/fasp"
)

// legCfg configures one leg of a swap, including its signing material and the
// chain-timing profile the ladder needs. It holds secrets, so the config file
// must be kept out of version control.
type legCfg struct {
	Chain      string `json:"chain"` // "n3" or "neox"
	RPC        string `json:"rpc"`
	Contract   string `json:"contract"`
	Token      string `json:"token"`
	Amount     string `json:"amount"`
	Key        string `json:"key,omitempty"`        // neox: 32-byte secp256k1 hex
	Wallet     string `json:"wallet,omitempty"`     // n3: NEP-6 wallet path
	WalletPass string `json:"walletPass,omitempty"` // n3
	Account    string `json:"account,omitempty"`    // n3: signer address
	BlockMs    int64  `json:"blockMs"`
	Confs      int    `json:"confs"`
	ClaimS     int64  `json:"claimSeconds"`
	Decimals   int    `json:"decimals"` // token decimals: n3 GAS=8, neox GAS=18
}

type runCfg struct {
	M    int    `json:"m"`    // congestion multiplier (>= 2)
	LegA legCfg `json:"legA"` // the user's leg, longer timelock (T_A)
	LegB legCfg `json:"legB"` // the solver's leg, shorter timelock (T_B)
}

type leg struct {
	htlc    fasp.HTLC
	addr    string
	profile fasp.Chain
	height  func(context.Context) (uint64, error)
}

// runCmd drives a full atomic swap across the two configured legs: it validates
// the timelock ladder, derives the absolute timelocks, builds the Swap, and runs
// the maker/taker sequence (solver locks B, user verifies B and locks A, user
// claims B revealing the secret, solver reads it and claims A).
func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "", "path to the swap config JSON")
	secretHex := fs.String("secret", "", "32-byte secret hex (generated if empty)")
	_ = fs.Parse(args)
	if *cfgPath == "" {
		fail("run: -config is required")
	}
	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		fail(err.Error())
	}
	var cfg runCfg
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fail("run: config: " + err.Error())
	}
	if cfg.M < fasp.MinMultiplier {
		fail(fmt.Sprintf("run: m must be >= %d", fasp.MinMultiplier))
	}
	amtA, okA := new(big.Int).SetString(cfg.LegA.Amount, 10)
	if !okA || amtA.Sign() < 0 {
		fail("run: legA.amount must be a non-negative decimal integer string")
	}
	amtB, okB := new(big.Int).SetString(cfg.LegB.Amount, 10)
	if !okB || amtB.Sign() < 0 {
		fail("run: legB.amount must be a non-negative decimal integer string")
	}

	var secret [32]byte
	if *secretHex == "" {
		if secret, err = fasp.NewSecret(); err != nil {
			fail(err.Error())
		}
	} else {
		b, derr := hex.DecodeString(*secretHex)
		if derr != nil || len(b) != 32 {
			fail("run: secret must be 32-byte hex")
		}
		copy(secret[:], b)
	}
	hashlock := fasp.Hashlock(secret)

	ctx := context.Background()
	legA, err := openSwapLeg(ctx, cfg.LegA)
	if err != nil {
		fail("run: leg A: " + err.Error())
	}
	legB, err := openSwapLeg(ctx, cfg.LegB)
	if err != nil {
		fail("run: leg B: " + err.Error())
	}

	userT, solverT, err := fasp.ComputeTimelocks(legA.profile, legB.profile, cfg.M)
	if err != nil {
		fail("run: ladder: " + err.Error())
	}
	tlB, err := absTimelock(ctx, legB, solverT)
	if err != nil {
		fail("run: timelock B: " + err.Error())
	}
	tlA, err := absTimelock(ctx, legA, userT)
	if err != nil {
		fail("run: timelock A: " + err.Error())
	}

	sw := &fasp.Swap{
		ChainA: legA.htlc, ChainB: legB.htlc,
		UserA: legA.addr, SolverA: legA.addr,
		UserB: legB.addr, SolverB: legB.addr,
		TokenA: cfg.LegA.Token, TokenB: cfg.LegB.Token,
		AmountA: amtA, AmountB: amtB,
		TimelockA: tlA, TimelockB: tlB,
		Hashlock: hashlock,
	}

	fmt.Printf("swap hashlock=%x (m=%d)\n", hashlock, cfg.M)
	fmt.Printf("  leg A %-6s %s token=%s amount=%s T_A=%d\n", legA.htlc.Name(), legA.addr, cfg.LegA.Token, cfg.LegA.Amount, tlA)
	fmt.Printf("  leg B %-6s %s token=%s amount=%s T_B=%d\n", legB.htlc.Name(), legB.addr, cfg.LegB.Token, cfg.LegB.Amount, tlB)

	steps := []struct {
		label string
		fn    func() error
	}{
		{"1/4 solver locks B", sw.SolverLockB},
		{"2/4 user verifies B and locks A", sw.UserVerifyAndLockA},
		{"3/4 user claims B (reveals secret)", func() error { return sw.UserClaimB(secret) }},
		{"4/4 solver reads secret and claims A", sw.SolverClaimA},
	}
	for _, st := range steps {
		fmt.Printf("-> %s ...\n", st.label)
		if err := st.fn(); err != nil {
			fmt.Printf("   FAILED: %v\n", err)
			fmt.Fprintln(os.Stderr, "refund: if B is locked, the solver refunds B after T_B (SolverRefundB); if A is also")
			fmt.Fprintln(os.Stderr, "locked, the user refunds A after T_A (UserRefundA). Never claim past the counterparty's refund window.")
			os.Exit(1)
		}
		fmt.Println("   ok")
	}
	fmt.Printf("swap complete: secret %x revealed on B and used to claim A\n", secret)
}

func openSwapLeg(ctx context.Context, c legCfg) (leg, error) {
	prof := fasp.Chain{
		Name:        c.Chain,
		BlockTime:   time.Duration(c.BlockMs) * time.Millisecond,
		Confs:       c.Confs,
		ClaimBudget: time.Duration(c.ClaimS) * time.Second,
	}
	switch c.Chain {
	case "neox":
		prof.Kind = fasp.Timestamp
		a, err := neox.New(ctx, neox.Config{RPCURL: c.RPC, Contract: c.Contract, PrivKeyHex: c.Key})
		if err != nil {
			return leg{}, err
		}
		return leg{htlc: a, addr: a.Address(), profile: prof, height: a.Height}, nil
	case "n3":
		prof.Kind = fasp.Height
		a, err := neon3.New(ctx, neon3.Config{RPCURL: c.RPC, Contract: c.Contract, WalletPath: c.Wallet, WalletPass: c.WalletPass, Account: c.Account, TokenHash: c.Token})
		if err != nil {
			return leg{}, err
		}
		return leg{htlc: a, addr: a.Address(), profile: prof, height: a.Height}, nil
	default:
		return leg{}, fmt.Errorf("unknown chain %q", c.Chain)
	}
}

func absTimelock(ctx context.Context, l leg, d time.Duration) (int64, error) {
	if l.profile.Kind == fasp.Timestamp {
		return l.profile.TimelockUnix(time.Now(), d)
	}
	h, err := l.height(ctx)
	if err != nil {
		return 0, err
	}
	abs, err := l.profile.TimelockHeight(int(h), d)
	return int64(abs), err
}
