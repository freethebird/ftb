// Package fasp is the client-side safety core of the FTB atomic-swap primitive (FASP): the
// cross-leg timelock-ladder check, the swap plan it validates, and the preimage/hashlock
// helpers. It is the off-chain counterpart to the on-chain FtbHtlc contracts (contracts/ftb_htlc
// on Neo N3, contracts/ftb_htlc/evm on Neo X) and is governed by the client rule in
// docs/expansion/neox/HTLC_LINK_SPEC.md section 5: the contracts enforce only the LOCAL rule
// (refund only after the local timelock, claim only with the preimage), and the cross-leg
// ordering is a client invariant because a contract cannot see the other chain.
//
// The swap, and why the ladder matters. The user holds asset A on chain A and wants asset B on
// chain B. The SOLVER locks B first under HTLC(h, user, T_B); the user then locks A under
// HTLC(h, solver, T_A). The user claims B by revealing the secret s, which makes s public; the
// solver reads s and claims A. If T_A <= T_B, the user could claim B (revealing s) and then still
// refund A after T_A has passed, keeping both legs and stealing from the solver. The ladder
// forbids that: T_A must exceed T_B by enough that, once s is public, the solver can always claim
// A before the user's refund on A opens.
//
// Dependency-free (stdlib only), no recursion. Every refusal is an explicit error; the client
// MUST refuse to sign or broadcast a lock when Validate returns one (fail closed).
package fasp

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
)

// TimelockKind is how a chain encodes an on-chain HTLC timelock.
type TimelockKind int

const (
	// Height means the timelock is an absolute block height (Neo N3, via ledger.CurrentIndex()).
	Height TimelockKind = iota
	// Timestamp means the timelock is an absolute unix-second time (Neo X EVM, via block.timestamp).
	Timestamp
)

// Chain is the timing profile of one chain, as the client measures it. BlockTime is the average
// time per block, Confs the confirmations the client waits before treating an inclusion as final,
// and ClaimBudget the extra wall-clock slack to get a claim transaction confirmed through a fee
// spike. These are measured and refreshed by the client, never trusted from the counterparty.
type Chain struct {
	Name        string
	Kind        TimelockKind
	BlockTime   time.Duration
	Confs       int
	ClaimBudget time.Duration
}

// window is the minimum real-time duration a party needs to confirm a claim on this chain: the
// congestion-multiplied confirmation time plus the claim budget (HTLC_LINK_SPEC section 5).
func (c Chain) window(m int) time.Duration {
	return time.Duration(m*c.Confs)*c.BlockTime + c.ClaimBudget
}

// Leg is one side of a swap: the chain it settles on and the timelock duration proposed for it,
// measured from the lock's confirmed inclusion (never from wall-clock signing time).
type Leg struct {
	Chain    Chain
	Timelock time.Duration
}

// Errors returned by Validate. A client treats any of them as a hard refusal to sign.
var (
	// ErrMultiplier means the congestion multiplier is below the floor (m must be >= 2).
	ErrMultiplier = errors.New("fasp: congestion multiplier m must be >= 2")
	// ErrShortClaimWindow means the solver-leg timelock is too short for the user to claim B in time.
	ErrShortClaimWindow = errors.New("fasp: solver-leg timelock shorter than the claim window")
	// ErrLadder means T_A does not exceed T_B by the A-claim window: the theft case. Refuse to sign.
	ErrLadder = errors.New("fasp: timelock ladder violated (T_A must exceed T_B by the A-claim window)")
)

// MinMultiplier is the floor for the congestion multiplier m. m >= 2 gives the fee market room
// during a spike without shortening the safety margins.
const MinMultiplier = 2

// Validate checks the cross-leg timelock safety for a swap where the user locks on userLeg (the
// longer timelock, T_A) and the solver locks on solverLeg (the shorter, T_B), with congestion
// multiplier m. It returns nil only if the swap is safe to sign; otherwise it returns the first
// violated rule, and the client MUST NOT sign or broadcast the lock.
func Validate(userLeg, solverLeg Leg, m int) error {
	if m < MinMultiplier {
		return fmt.Errorf("%w: got %d", ErrMultiplier, m)
	}
	// The user must be able to claim B before the solver's refund on B opens.
	reqB := solverLeg.Chain.window(m)
	if solverLeg.Timelock < reqB {
		return fmt.Errorf("%w: T_B=%s < required %s on %s", ErrShortClaimWindow,
			solverLeg.Timelock, reqB, solverLeg.Chain.Name)
	}
	// After the secret is public, the solver must be able to claim A before the user's refund on A
	// opens: T_A >= T_B + (A-claim window). This is the maker/taker ordering, the anti-theft rule.
	reqA := userLeg.Chain.window(m)
	if userLeg.Timelock < solverLeg.Timelock+reqA {
		return fmt.Errorf("%w: T_A=%s < T_B=%s + A-window=%s on %s", ErrLadder,
			userLeg.Timelock, solverLeg.Timelock, reqA, userLeg.Chain.Name)
	}
	return nil
}

// ComputeTimelocks returns the minimal safe timelock durations (T_A for the user's leg, T_B for
// the solver's leg) for the given chains and multiplier. The result always satisfies Validate.
// A client may add headroom on top, never subtract.
func ComputeTimelocks(userChain, solverChain Chain, m int) (userT, solverT time.Duration, err error) {
	if m < MinMultiplier {
		return 0, 0, fmt.Errorf("%w: got %d", ErrMultiplier, m)
	}
	solverT = solverChain.window(m)
	userT = solverT + userChain.window(m)
	return userT, solverT, nil
}

// TimelockHeight converts a timelock duration to an absolute on-chain value for a Height chain
// (Neo N3): inclusionHeight plus the duration in blocks, rounded up so the real window is never
// shortened by truncation. It errors if the chain is not height-based.
func (c Chain) TimelockHeight(inclusionHeight int, d time.Duration) (int, error) {
	if c.Kind != Height {
		return 0, fmt.Errorf("fasp: %s is not a height-based chain", c.Name)
	}
	if c.BlockTime <= 0 {
		return 0, fmt.Errorf("fasp: %s has no block time", c.Name)
	}
	blocks := int((d + c.BlockTime - 1) / c.BlockTime) // ceil
	return inclusionHeight + blocks, nil
}

// TimelockUnix converts a timelock duration to an absolute unix-second value for a Timestamp
// chain (Neo X): the inclusion time plus the duration. It errors if the chain is not
// timestamp-based.
func (c Chain) TimelockUnix(inclusion time.Time, d time.Duration) (int64, error) {
	if c.Kind != Timestamp {
		return 0, fmt.Errorf("fasp: %s is not a timestamp-based chain", c.Name)
	}
	return inclusion.Add(d).Unix(), nil
}

// ---- preimage and hashlock ----

// NewSecret returns a fresh 32-byte swap secret from the crypto RNG.
func NewSecret() ([32]byte, error) {
	var s [32]byte
	if _, err := rand.Read(s[:]); err != nil {
		return s, fmt.Errorf("fasp: secret: %w", err)
	}
	return s, nil
}

// Hashlock is SHA-256 of the secret: the hashlock used identically on every chain (Neo N3, Neo X,
// and Bitcoin), the same function the FtbHtlc contracts check against.
func Hashlock(secret [32]byte) [32]byte {
	return sha256.Sum256(secret[:])
}

// VerifyPreimage reports whether a revealed preimage hashes to the hashlock.
func VerifyPreimage(preimage, hashlock [32]byte) bool {
	h := Hashlock(preimage)
	return h == hashlock
}
