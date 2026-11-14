package fasp

import (
	"errors"
	"fmt"
	"math/big"
)

// This file is the swap orchestration: the maker/taker flow that drives an atomic swap across two
// chains through their FtbHtlc contracts, with the safety checks a client must perform. It is
// chain-agnostic: it talks to each chain only through the HTLC adapter below, so the same logic
// drives Neo N3, Neo X, and (later) Bitcoin/Monero/Zcash, and is unit-tested offline against an
// in-memory mock that mirrors the contract. The timelock ladder (Validate, fasp.go) is enforced at
// quote time on durations; the absolute per-chain timelocks it yields are carried in Swap.
//
// The invariant the orchestration protects: the user never locks asset A until it has verified,
// on-chain, that the solver's lock on asset B names the user as recipient for the agreed amount
// and timelock. Because the solver locks first, an abort before that verification costs the user
// nothing.

// SwapState mirrors the on-chain FtbHtlc state.
type SwapState int

const (
	StateNone SwapState = iota
	StateLocked
	StateClaimed
	StateRefunded
)

// SwapRecord is one chain's FtbHtlc record for a hashlock, as an adapter reports it. Amount is the
// delivered amount the contract actually escrowed (measured receipt), Timelock the chain-native
// absolute value (block height on Neo N3, unix seconds on Neo X).
type SwapRecord struct {
	Sender    string
	Recipient string
	Token     string
	Amount    *big.Int
	Timelock  int64
	State     SwapState
}

// HTLC is the minimal surface the orchestration needs over one chain's FtbHtlc contract. A
// production adapter wraps that chain's RPC and wallet; the test mock mirrors the contract
// semantics. Claim and Refund are permissionless on-chain, but a caller still submits and pays,
// so the submitting identity is passed.
type HTLC interface {
	Name() string
	Lock(from, recipient, token string, amount *big.Int, hashlock [32]byte, timelock int64) error
	Get(hashlock [32]byte) (SwapRecord, bool)
	Claim(caller string, hashlock, preimage [32]byte) error
	Refund(caller string, hashlock [32]byte) error
	// Preimage returns the secret the claimer revealed (read from the Claimed event), if any.
	Preimage(hashlock [32]byte) ([32]byte, bool)
}

// LockSpec is what a party requires of a counterparty's lock before committing its own funds.
type LockSpec struct {
	Recipient string // must be the verifying party: the escrow must pay out to them
	Token     string
	MinAmount *big.Int // the escrow must hold at least this (measured receipt may be below nominal)
	Timelock  int64    // the exact timelock the ladder was validated against
}

// Errors from verification and orchestration.
var (
	ErrNoLock    = errors.New("fasp: no lock found for hashlock")
	ErrNotLocked = errors.New("fasp: lock is not in the LOCKED state")
	ErrSpec      = errors.New("fasp: lock does not match the agreed terms")
	ErrNoSecret  = errors.New("fasp: counterparty secret not yet revealed")
	ErrBadSecret = errors.New("fasp: revealed secret does not open the hashlock")
)

// VerifyLock returns nil only if a fetched record is a live LOCKED escrow that pays the verifying
// party the agreed token, at least the agreed amount, under the agreed timelock. Any mismatch is a
// refusal: the verifying party must not commit its own funds.
func VerifyLock(r SwapRecord, ok bool, spec LockSpec) error {
	if !ok {
		return ErrNoLock
	}
	if r.State != StateLocked {
		return fmt.Errorf("%w: state=%d", ErrNotLocked, r.State)
	}
	if r.Recipient != spec.Recipient {
		return fmt.Errorf("%w: recipient got %q want %q", ErrSpec, r.Recipient, spec.Recipient)
	}
	if r.Token != spec.Token {
		return fmt.Errorf("%w: token got %q want %q", ErrSpec, r.Token, spec.Token)
	}
	if r.Amount == nil || spec.MinAmount == nil || r.Amount.Cmp(spec.MinAmount) < 0 {
		return fmt.Errorf("%w: amount %v below required %v", ErrSpec, r.Amount, spec.MinAmount)
	}
	if r.Timelock != spec.Timelock {
		return fmt.Errorf("%w: timelock got %d want %d", ErrSpec, r.Timelock, spec.Timelock)
	}
	return nil
}

// Swap is one atomic swap: the user locks asset A on ChainA under the longer timelock, the solver
// locks asset B on ChainB under the shorter. TimelockA and TimelockB are the absolute on-chain
// values derived from fasp.ComputeTimelocks + the unit converters, after Validate accepted the
// ladder.
type Swap struct {
	ChainA HTLC // the user's leg (asset A, timelock T_A, the longer one)
	ChainB HTLC // the solver's leg (asset B, timelock T_B, the shorter one)

	// A party has a different address on each chain, so identities are per-leg.
	// On A the user locks for the solver; on B the solver locks for the user.
	UserA, UserB     string
	SolverA, SolverB string

	TokenA string
	TokenB string

	AmountA *big.Int
	AmountB *big.Int

	TimelockA int64
	TimelockB int64

	Hashlock [32]byte
}

// SolverLockB is step 1: the solver locks asset B for the user. The solver locks first, so the
// user risks nothing until it has seen this lock.
func (s *Swap) SolverLockB() error {
	return s.ChainB.Lock(s.SolverB, s.UserB, s.TokenB, s.AmountB, s.Hashlock, s.TimelockB)
}

// UserVerifyAndLockA is step 2: the user verifies the solver's B lock on-chain, then locks asset A
// for the solver. If verification fails it returns an error and locks nothing, so the user's funds
// stay untouched. This is the single most important client decision after the ladder.
func (s *Swap) UserVerifyAndLockA() error {
	rec, ok := s.ChainB.Get(s.Hashlock)
	spec := LockSpec{Recipient: s.UserB, Token: s.TokenB, MinAmount: s.AmountB, Timelock: s.TimelockB}
	if err := VerifyLock(rec, ok, spec); err != nil {
		return fmt.Errorf("refusing to lock A: solver's B lock failed verification: %w", err)
	}
	return s.ChainA.Lock(s.UserA, s.SolverA, s.TokenA, s.AmountA, s.Hashlock, s.TimelockA)
}

// UserClaimB is step 3: the user claims asset B by revealing the secret, which publishes it on
// ChainB for the solver to read.
func (s *Swap) UserClaimB(secret [32]byte) error {
	return s.ChainB.Claim(s.UserB, s.Hashlock, secret)
}

// SolverClaimA is step 4: the solver reads the secret the user revealed on ChainB and claims asset
// A. It returns ErrNoSecret until the user has claimed B, and refuses a secret that does not open
// the hashlock.
func (s *Swap) SolverClaimA() error {
	secret, ok := s.ChainB.Preimage(s.Hashlock)
	if !ok {
		return ErrNoSecret
	}
	if !VerifyPreimage(secret, s.Hashlock) {
		return ErrBadSecret
	}
	return s.ChainA.Claim(s.SolverA, s.Hashlock, secret)
}

// SolverRefundB refunds the solver's B lock after T_B (the user never locked, or never claimed).
func (s *Swap) SolverRefundB() error {
	return s.ChainB.Refund(s.SolverB, s.Hashlock)
}

// UserRefundA refunds the user's A lock after T_A (the swap aborted after the user locked).
func (s *Swap) UserRefundA() error {
	return s.ChainA.Refund(s.UserA, s.Hashlock)
}
