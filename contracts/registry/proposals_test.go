// Neotest suite for the registry v2 proposals board. A mock NEP-17 stands in for FTB
// (bound as the "token" module, exactly as the deploy tool will bootstrap it). Covers
// bonded submission, the exact-bond rule, kind/meta validation, foreign-token rejection,
// refund vs spam-keep on close, escrow accounting, and the no-double-close guard.
//
// Run: go test ./contracts/registry/
package registry_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
)

const boardBond = 25

type board struct {
	e            *neotest.Executor
	owner, alice neotest.Signer
	ftb, reg     util.Uint160
}

func setupBoard(t *testing.T) *board {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	b := &board{e: e}
	b.owner = e.NewAccount(t)
	b.alice = e.NewAccount(t)

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	b.ftb = ftbC.Hash

	regC := neotest.CompileFile(t, e.Validator.ScriptHash(), "registry.go", "registry.yml")
	e.DeployContract(t, regC, b.owner.ScriptHash().BytesBE())
	b.reg = regC.Hash

	// bind the token module and set the bond, as the deploy flow will
	e.NewInvoker(b.reg, b.owner).Invoke(t, stackitem.Null{}, "bootstrap", "token", b.ftb.BytesBE())
	e.NewInvoker(b.reg, b.owner).Invoke(t, stackitem.Null{}, "setProposalBond", boardBond)
	e.NewInvoker(b.ftb, b.owner).Invoke(t, stackitem.Null{}, "mint", b.alice.ScriptHash().BytesBE(), 500)
	return b
}

// propose moves the bond from alice into the registry with the proposal as data.
func (b *board) propose(t *testing.T, kind string, ref util.Uint160, meta string) {
	b.e.NewInvoker(b.ftb, b.alice).Invoke(t, stackitem.NewBool(true), "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond,
		[]any{kind, ref.BytesBE(), meta})
}

func (b *board) regBalance(t *testing.T, want int) {
	b.e.NewInvoker(b.ftb, b.e.Validator).Invoke(t, stackitem.Make(want), "balanceOf", b.reg.BytesBE())
}

func TestSubmitAndCloseWithRefund(t *testing.T) {
	b := setupBoard(t)
	ballotHash := b.e.NewAccount(t).ScriptHash()

	b.propose(t, "ballot", ballotHash, "ratios: r1 = 1 FLM : 1 FTB, r2 = 4 FLOCKS : 1 FTB — adopt?")
	b.e.NewInvoker(b.reg, b.e.Validator).Invoke(t, stackitem.Make(1), "proposalCount")
	b.e.NewInvoker(b.reg, b.e.Validator).Invoke(t, stackitem.Make(boardBond), "escrow")
	b.regBalance(t, boardBond)

	// the record reads back verbatim
	b.e.NewInvoker(b.reg, b.e.Validator).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		p, err := stack[0].Convert(stackitem.ArrayT)
		require.NoError(t, err)
		items := p.Value().([]stackitem.Item)
		require.Equal(t, "ballot", string(items[0].Value().([]byte)))
		require.Equal(t, ballotHash.BytesBE(), items[1].Value().([]byte))
		require.Equal(t, b.alice.ScriptHash().BytesBE(), items[3].Value().([]byte))
		require.Equal(t, true, items[5].Value().(bool))
	}, "proposal", 1)

	// legitimate close refunds the bond to the proposer
	b.e.NewInvoker(b.reg, b.owner).Invoke(t, stackitem.Null{}, "closeProposal", 1, true)
	b.regBalance(t, 0)
	b.e.NewInvoker(b.ftb, b.e.Validator).Invoke(t, stackitem.Make(500), "balanceOf", b.alice.ScriptHash().BytesBE())
	b.e.NewInvoker(b.reg, b.e.Validator).Invoke(t, stackitem.Make(0), "escrow")
}

func TestSpamCloseKeepsTheBond(t *testing.T) {
	b := setupBoard(t)
	b.propose(t, "listing", b.e.NewAccount(t).ScriptHash(), "list DOGE2 at 2% bond")

	b.e.NewInvoker(b.reg, b.owner).Invoke(t, stackitem.Null{}, "closeProposal", 1, false)
	b.regBalance(t, 0)
	// the committee keeps the spam bond
	b.e.NewInvoker(b.ftb, b.e.Validator).Invoke(t, stackitem.Make(boardBond), "balanceOf",
		b.owner.ScriptHash().BytesBE())
	b.e.NewInvoker(b.ftb, b.e.Validator).Invoke(t, stackitem.Make(475), "balanceOf",
		b.alice.ScriptHash().BytesBE())
}

func TestBoardGuards(t *testing.T) {
	b := setupBoard(t)
	ref := b.e.NewAccount(t).ScriptHash()
	stranger := b.e.NewAccount(t)

	// exact bond: underpay and overpay both revert
	b.e.NewInvoker(b.ftb, b.alice).InvokeFail(t, "exact bond", "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond-1, []any{"ballot", ref.BytesBE(), "x"})
	b.e.NewInvoker(b.ftb, b.alice).InvokeFail(t, "exact bond", "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond+1, []any{"ballot", ref.BytesBE(), "x"})

	// kind whitelist
	b.e.NewInvoker(b.ftb, b.alice).InvokeFail(t, "kind must be", "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond, []any{"coup", ref.BytesBE(), "x"})

	// meta bounds: empty and oversized both revert
	b.e.NewInvoker(b.ftb, b.alice).InvokeFail(t, "meta", "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond, []any{"ballot", ref.BytesBE(), ""})
	big := make([]byte, maxMetaForTest+1)
	for i := range big {
		big[i] = 'x'
	}
	b.e.NewInvoker(b.ftb, b.alice).InvokeFail(t, "meta", "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond, []any{"ballot", ref.BytesBE(), string(big)})

	// foreign token (native GAS) is not a bond
	gasHash := b.e.NativeHash(t, nativenames.Gas)
	b.e.NewInvoker(gasHash, b.alice).InvokeFail(t, "only FTB", "transfer",
		b.alice.ScriptHash().BytesBE(), b.reg.BytesBE(), boardBond, []any{"ballot", ref.BytesBE(), "x"})

	// only the owner closes; double close panics
	b.propose(t, "ballot", ref, "a real question")
	b.e.NewInvoker(b.reg, stranger).InvokeFail(t, "only owner", "closeProposal", 1, true)
	b.e.NewInvoker(b.reg, b.owner).Invoke(t, stackitem.Null{}, "closeProposal", 1, true)
	b.e.NewInvoker(b.reg, b.owner).InvokeFail(t, "not open", "closeProposal", 1, true)

	// unknown ids panic on read
	b.e.NewInvoker(b.reg, b.e.Validator).InvokeFail(t, "no such id", "proposal", 99)
}

// maxMetaForTest mirrors the contract's maxMetaLen (tests cannot import contract code).
const maxMetaForTest = 1024
