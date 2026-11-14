// Neotest suite for the keyless, immutable FtbHtlc settlement contract. Exercises the full
// lifecycle -- lock / claim / refund -- plus the wrong-preimage, double-claim,
// refund-before/after-timelock, reused-hashlock, and validation paths, over both a NEP-17
// mock token and native NEO (the null-sender GAS mint path). No owner, no oracle: the only
// authorities are the preimage and the block height.
//
// Timelocks are block heights (ledger.CurrentIndex()); neotest advances height one per
// generated block, so a refund window is crossed by GenerateNewBlocks.
//
// Run: go test ./contracts/ftb_htlc/
package ftbhtlc_test

import (
	"crypto/sha256"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

type sys struct {
	e                                *neotest.Executor
	owner, alice, bob, other, solver neotest.Signer
	tok, htlc, neoH, gasH            util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t, 100000_0000_0000)
	s.alice = e.NewAccount(t, 100000_0000_0000)
	s.bob = e.NewAccount(t)
	s.other = e.NewAccount(t, 100000_0000_0000)
	s.solver = e.NewAccount(t, 100000_0000_0000)
	s.neoH = e.NativeHash(t, nativenames.Neo)
	s.gasH = e.NativeHash(t, nativenames.Gas)

	// A minimal openly-mintable NEP-17 stands in for an escrowed asset.
	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, tokC, nil)
	s.tok = tokC.Hash

	// The HTLC is asset-agnostic: _deploy stores only a version tag, no token is injected.
	htlcC := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_htlc.go", "ftb_htlc.yml")
	e.DeployContract(t, htlcC, nil)
	s.htlc = htlcC.Hash
	return s
}

// ---- helpers ----

func (s *sys) top(t *testing.T) int { return int(s.e.TopBlock(t).Index) } // #nosec G115 -- test height

func (s *sys) mintTok(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.tok, s.owner).Invoke(t, stackitem.Null{}, "mint", to.BytesBE(), amt)
}
func (s *sys) fundNeo(t *testing.T, to util.Uint160, whole int) {
	s.e.NewInvoker(s.neoH, s.e.Validator).Invoke(t, stackitem.Make(true), "transfer",
		s.e.Validator.ScriptHash().BytesBE(), to.BytesBE(), whole, nil)
}
func (s *sys) tokBal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.tok, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) neoBal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.neoH, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) stateOf(t *testing.T, hashlock []byte, want int) {
	s.e.NewInvoker(s.htlc, s.owner).Invoke(t, stackitem.Make(want), "stateOf", hashlock)
}
func (s *sys) lockTok(t *testing.T, sender neotest.Signer, amt int, hashlock []byte, recipient util.Uint160, timelock int) {
	s.e.NewInvoker(s.tok, sender).Invoke(t, stackitem.Make(true), "transfer",
		sender.ScriptHash().BytesBE(), s.htlc.BytesBE(), amt,
		[]any{"lock", hashlock, recipient.BytesBE(), timelock})
}
func (s *sys) lockTokFail(t *testing.T, sender neotest.Signer, amt int, data []any, msg string) {
	s.e.NewInvoker(s.tok, sender).InvokeFail(t, msg, "transfer",
		sender.ScriptHash().BytesBE(), s.htlc.BytesBE(), amt, data)
}
func (s *sys) lockNeo(t *testing.T, sender neotest.Signer, amt int, hashlock []byte, recipient util.Uint160, timelock int) {
	s.e.NewInvoker(s.neoH, sender).Invoke(t, stackitem.Make(true), "transfer",
		sender.ScriptHash().BytesBE(), s.htlc.BytesBE(), amt,
		[]any{"lock", hashlock, recipient.BytesBE(), timelock})
}
func (s *sys) claim(t *testing.T, who neotest.Signer, hashlock, preimage []byte) {
	s.e.NewInvoker(s.htlc, who).Invoke(t, stackitem.Null{}, "claim", hashlock, preimage)
}
func (s *sys) claimFail(t *testing.T, who neotest.Signer, hashlock, preimage []byte, msg string) {
	s.e.NewInvoker(s.htlc, who).InvokeFail(t, msg, "claim", hashlock, preimage)
}
func (s *sys) refund(t *testing.T, who neotest.Signer, hashlock []byte) {
	s.e.NewInvoker(s.htlc, who).Invoke(t, stackitem.Null{}, "refund", hashlock)
}
func (s *sys) refundFail(t *testing.T, who neotest.Signer, hashlock []byte, msg string) {
	s.e.NewInvoker(s.htlc, who).InvokeFail(t, msg, "refund", hashlock)
}

// mkSwap derives a 32-byte preimage from a tag and its SHA-256 hashlock, matching the
// contract's crypto.Sha256(preimage) == hashlock check.
func mkSwap(tag string) ([]byte, []byte) {
	p := sha256.Sum256([]byte(tag))
	h := sha256.Sum256(p[:])
	return p[:], h[:]
}

// ---- lifecycle ----

func TestLockClaim(t *testing.T) {
	s := setup(t)
	pre, hl := mkSwap("a")
	s.mintTok(t, s.alice.ScriptHash(), 1000)

	s.lockTok(t, s.alice, 1000, hl, s.bob.ScriptHash(), s.top(t)+50)
	s.stateOf(t, hl, 1) // LOCKED
	s.tokBal(t, s.htlc, 1000)
	s.tokBal(t, s.alice.ScriptHash(), 0)

	// Permissionless: a third party may submit the claim; funds go to the fixed recipient.
	s.claim(t, s.other, hl, pre)
	s.stateOf(t, hl, 2) // CLAIMED
	s.tokBal(t, s.bob.ScriptHash(), 1000)
	s.tokBal(t, s.htlc, 0)
}

func TestWrongPreimage(t *testing.T) {
	s := setup(t)
	pre, hl := mkSwap("b")
	_ = pre
	s.mintTok(t, s.alice.ScriptHash(), 1000)
	s.lockTok(t, s.alice, 1000, hl, s.bob.ScriptHash(), s.top(t)+50)

	wrong, _ := mkSwap("not-b")
	s.claimFail(t, s.other, hl, wrong, "claim: preimage mismatch")
	// A short preimage is rejected before the hash check.
	s.claimFail(t, s.other, hl, make([]byte, 16), "claim: preimage must be 32 bytes")
	s.stateOf(t, hl, 1) // still LOCKED
}

func TestDoubleClaim(t *testing.T) {
	s := setup(t)
	pre, hl := mkSwap("c")
	s.mintTok(t, s.alice.ScriptHash(), 1000)
	s.lockTok(t, s.alice, 1000, hl, s.bob.ScriptHash(), s.top(t)+50)

	s.claim(t, s.other, hl, pre)
	s.claimFail(t, s.other, hl, pre, "claim: not locked")
	s.tokBal(t, s.bob.ScriptHash(), 1000) // paid exactly once
}

func TestRefundBeforeTimelock(t *testing.T) {
	s := setup(t)
	_, hl := mkSwap("d")
	s.mintTok(t, s.alice.ScriptHash(), 1000)
	s.lockTok(t, s.alice, 1000, hl, s.bob.ScriptHash(), s.top(t)+50)

	s.refundFail(t, s.other, hl, "refund: timelock not reached")
	s.stateOf(t, hl, 1)
}

func TestRefundAfterTimelock(t *testing.T) {
	s := setup(t)
	_, hl := mkSwap("e")
	s.mintTok(t, s.alice.ScriptHash(), 1000)
	s.lockTok(t, s.alice, 1000, hl, s.bob.ScriptHash(), s.top(t)+10)

	s.e.GenerateNewBlocks(t, 12) // cross the timelock height
	s.refund(t, s.other, hl)     // permissionless; funds go to the fixed sender
	s.stateOf(t, hl, 3)          // REFUNDED
	s.tokBal(t, s.alice.ScriptHash(), 1000)
	s.tokBal(t, s.htlc, 0)
}

func TestReusedHashlock(t *testing.T) {
	s := setup(t)
	_, hl := mkSwap("f")
	s.mintTok(t, s.alice.ScriptHash(), 1000)
	s.lockTok(t, s.alice, 500, hl, s.bob.ScriptHash(), s.top(t)+50)

	// A reused hashlock faults, which reverts the locker's own transfer: nothing half-locks.
	s.lockTokFail(t, s.alice, 500,
		[]any{"lock", hl, s.bob.ScriptHash().BytesBE(), s.top(t) + 50}, "lock: hashlock already used")
	s.tokBal(t, s.alice.ScriptHash(), 500) // second transfer reverted, alice keeps the rest
	s.tokBal(t, s.htlc, 500)
}

func TestLockValidation(t *testing.T) {
	s := setup(t)
	_, hl := mkSwap("g")
	s.mintTok(t, s.alice.ScriptHash(), 1000)
	rec := s.bob.ScriptHash().BytesBE()

	s.lockTokFail(t, s.alice, 100, []any{"lock", make([]byte, 16), rec, s.top(t) + 50},
		"lock: hashlock must be 32 bytes")
	s.lockTokFail(t, s.alice, 100, []any{"lock", hl, make([]byte, 10), s.top(t) + 50},
		"lock: recipient must be a 20-byte address")
	s.lockTokFail(t, s.alice, 100, []any{"lock", hl, rec, 1},
		"lock: timelock must be in the future")
	s.lockTokFail(t, s.alice, 100, []any{"frobnicate"}, "htlc: bad action")

	s.stateOf(t, hl, 0)                     // nothing persisted
	s.tokBal(t, s.alice.ScriptHash(), 1000) // every bad transfer reverted
}

func TestNativeNeoAndNullSenderGas(t *testing.T) {
	s := setup(t)
	pre, hl := mkSwap("neo")
	s.fundNeo(t, s.alice.ScriptHash(), 10)

	s.lockNeo(t, s.alice, 10, hl, s.bob.ScriptHash(), s.top(t)+10)
	s.stateOf(t, hl, 1)
	s.neoBal(t, s.htlc, 10)

	// The contract now holds NEO; generated blocks mint GAS to it (a null-sender payment the
	// handler must accept silently, not fault).
	s.e.GenerateNewBlocks(t, 3)

	s.claim(t, s.other, hl, pre)
	s.stateOf(t, hl, 2)
	s.neoBal(t, s.bob.ScriptHash(), 10)
	s.neoBal(t, s.htlc, 0)
}
