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

func deployBallot(t *testing.T, leaves [][]byte, numOpts int) (*neotest.Executor, *neotest.ContractInvoker, [][][]byte) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "ballot.go", "ballot.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	root, proofs := merkle.Build(leaves)
	b := e.NewInvoker(c.Hash, owner)
	b.Invoke(t, stackitem.Null{}, "configure", root, numOpts)
	return e, b, proofs
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

	b.Invoke(t, stackitem.Null{}, "configure", root, 2)
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
	b.Invoke(t, stackitem.Null{}, "configure", root, 2)
	b.InvokeFail(t, "already configured", "configure", root, 2)

	// option bounds are checked on a fresh contract (write-once fires first otherwise)
	_, b1, _ := freshBallot(t)
	b1.InvokeFail(t, "2 to 16 options", "configure", root, 1)
}

func TestOnlyOwnerConfigures(t *testing.T) {
	e, b, _ := freshBallot(t)
	other := e.NewAccount(t)
	root := make([]byte, 32)
	e.NewInvoker(b.Hash, other).InvokeFail(t, "only owner", "configure", root, 2)
}
