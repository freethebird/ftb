// Neotest suite for the ballot. Uses pkg/merkle — the same builder as the migration — so a
// pass proves ballots accept genuine migration proofs and reject everything else.
//
// Run: go test ./contracts/ballot/
package ballot_test

import (
	"math/big"
	"testing"

	"github.com/freethebird/ftb/pkg/merkle"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func proofArg(p [][]byte) []any {
	a := make([]any, len(p))
	for i, s := range p {
		a[i] = s
	}
	return a
}

func TestWeightedVoteFlow(t *testing.T) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)

	a1 := e.NewAccount(t) // weight 1000
	a2 := e.NewAccount(t) // weight 2500
	leaves := [][]byte{
		merkle.Leaf(a1.ScriptHash(), big.NewInt(1000)),
		merkle.Leaf(a2.ScriptHash(), big.NewInt(2500)),
	}
	root, proofs := merkle.Build(leaves)

	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "ballot.go", "ballot.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	b := e.NewInvoker(c.Hash, owner)

	// option bounds are enforced before configuration
	e.NewInvoker(c.Hash, a1).InvokeFail(t, "not configured", "vote", a1.ScriptHash(), 1000, 1, proofArg(proofs[0]))

	b.Invoke(t, stackitem.Null{}, "configure", root, 2, 1_000_000)
	b.Invoke(t, stackitem.Make(2), "numOptions")

	// a1 votes option 1 with its exact snapshot weight
	e.NewInvoker(c.Hash, a1).Invoke(t, stackitem.Null{}, "vote", a1.ScriptHash(), 1000, 1, proofArg(proofs[0]))
	b.Invoke(t, stackitem.Make(1000), "tally", 1)
	b.Invoke(t, stackitem.Make(1), "voted", a1.ScriptHash().BytesBE())

	// no double vote
	e.NewInvoker(c.Hash, a1).InvokeFail(t, "already voted", "vote", a1.ScriptHash(), 1000, 2, proofArg(proofs[0]))

	// inflated weight does not verify against the root
	e.NewInvoker(c.Hash, a2).InvokeFail(t, "bad proof", "vote", a2.ScriptHash(), 9999, 2, proofArg(proofs[1]))

	// out-of-range option rejected
	e.NewInvoker(c.Hash, a2).InvokeFail(t, "no such option", "vote", a2.ScriptHash(), 2500, 3, proofArg(proofs[1]))

	// a2 votes option 2 correctly; tallies accumulate independently
	e.NewInvoker(c.Hash, a2).Invoke(t, stackitem.Null{}, "vote", a2.ScriptHash(), 2500, 2, proofArg(proofs[1]))
	b.Invoke(t, stackitem.Make(1000), "tally", 1)
	b.Invoke(t, stackitem.Make(2500), "tally", 2)

	// a non-holder has no leaf, so no valid proof
	a3 := e.NewAccount(t)
	e.NewInvoker(c.Hash, a3).InvokeFail(t, "bad proof", "vote", a3.ScriptHash(), 1, 1, proofArg(proofs[0]))

	// the bound root is publicly readable and matches what was published
	b.Invoke(t, stackitem.NewBuffer(root), "root")

	// tally rejects out-of-range options instead of silently aliasing onto another
	// option's byte-keyed slot (FTB-31, gosec G115)
	b.InvokeFail(t, "tally: no such option", "tally", 0)
	b.InvokeFail(t, "tally: no such option", "tally", 17)
	b.InvokeFail(t, "tally: no such option", "tally", 257) // would alias option 1
}

func freshBallot(t *testing.T) (*neotest.Executor, *neotest.ContractInvoker, neotest.Signer) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "ballot.go", "ballot.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	return e, e.NewInvoker(c.Hash, owner), owner
}

func TestConfigureIsWriteOnce(t *testing.T) {
	_, b, _ := freshBallot(t)

	root := make([]byte, 32)
	b.Invoke(t, stackitem.Null{}, "configure", root, 2, 1_000_000)
	b.InvokeFail(t, "already configured", "configure", root, 2, 1_000_000)

	// option bounds are checked on a fresh contract (write-once fires first otherwise)
	_, b1, _ := freshBallot(t)
	b1.InvokeFail(t, "2 to 16 options", "configure", root, 1, 1_000_000)
}

func TestOnlyOwnerConfigures(t *testing.T) {
	e, b, _ := freshBallot(t)
	other := e.NewAccount(t)
	root := make([]byte, 32)
	e.NewInvoker(b.Hash, other).InvokeFail(t, "only owner", "configure", root, 2, 1_000_000)
}

// TestVotingWindowCloses proves the on-chain deadline: a window already in the past is
// rejected at configure time, votes inside the window succeed, and once the chain passes
// endBlock a valid, un-voted holder is turned away with "voting closed".
func TestVotingWindowCloses(t *testing.T) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	a1 := e.NewAccount(t)
	a2 := e.NewAccount(t)
	leaves := [][]byte{
		merkle.Leaf(a1.ScriptHash(), big.NewInt(1000)),
		merkle.Leaf(a2.ScriptHash(), big.NewInt(500)),
	}
	root, proofs := merkle.Build(leaves)

	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "ballot.go", "ballot.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	b := e.NewInvoker(c.Hash, owner)

	// a window already in the past is rejected at configure time
	b.InvokeFail(t, "endBlock must be in the future", "configure", root, 2, 1)

	// configure a short window
	end := int(bc.BlockHeight()) + 3
	b.Invoke(t, stackitem.Null{}, "configure", root, 2, end)
	b.Invoke(t, stackitem.Make(end), "endBlock")

	// a1 votes while the window is open
	e.NewInvoker(c.Hash, a1).Invoke(t, stackitem.Null{}, "vote", a1.ScriptHash(), 1000, 1, proofArg(proofs[0]))

	// advance well past endBlock; a2 is a valid, un-voted holder but the window has closed
	for i := 0; i < 10; i++ {
		e.AddNewBlock(t)
	}
	e.NewInvoker(c.Hash, a2).InvokeFail(t, "voting closed", "vote", a2.ScriptHash(), 500, 2, proofArg(proofs[1]))
}
