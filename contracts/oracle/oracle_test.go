// Reference neotest suite for the oracle: median of fresh reports, reporter gating, the
// minimum-reporters floor, and staleness causing a safe revert.
//
// Run: go test ./contracts/oracle/
package oracle_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func deployOracle(t *testing.T) (*neotest.Executor, *neotest.ContractInvoker, util.Uint160) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "oracle.go", "oracle.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	return e, e.NewInvoker(c.Hash, owner), c.Hash
}

func TestMedianAndGating(t *testing.T) {
	e, oracleOwner, oracleHash := deployOracle(t)
	asset := e.NewAccount(t).ScriptHash() // stand-in asset hash

	// quorum of 3, generous freshness so reports stay fresh across blocks
	oracleOwner.Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 3)

	r1 := e.NewAccount(t)
	r2 := e.NewAccount(t)
	r3 := e.NewAccount(t)
	rogue := e.NewAccount(t)
	for _, r := range []neotest.Signer{r1, r2, r3} {
		oracleOwner.Invoke(t, stackitem.Null{}, "setReporter", r.ScriptHash().BytesBE(), true)
	}

	// each reporter submits; non-reporter is rejected
	e.NewInvoker(oracleHash, r1).Invoke(t, stackitem.Null{}, "report", r1.ScriptHash().BytesBE(), asset.BytesBE(), 100)
	e.NewInvoker(oracleHash, r2).Invoke(t, stackitem.Null{}, "report", r2.ScriptHash().BytesBE(), asset.BytesBE(), 300)
	e.NewInvoker(oracleHash, r3).Invoke(t, stackitem.Null{}, "report", r3.ScriptHash().BytesBE(), asset.BytesBE(), 200)
	e.NewInvoker(oracleHash, rogue).InvokeFail(t, "not a reporter", "report", rogue.ScriptHash().BytesBE(), asset.BytesBE(), 999)

	// median of {100,300,200} is 200
	oracleOwner.Invoke(t, stackitem.Make(200), "price", asset.BytesBE())

	// drop a reporter below quorum -> price reverts (safe pause)
	oracleOwner.Invoke(t, stackitem.Null{}, "setReporter", r3.ScriptHash().BytesBE(), false)
	oracleOwner.InvokeFail(t, "not enough fresh prices", "price", asset.BytesBE())
}

func TestStalenessReverts(t *testing.T) {
	e, oracleOwner, oracleHash := deployOracle(t)
	asset := e.NewAccount(t).ScriptHash()

	// freshness of 1ms with a single reporter: 2ms of block advancement makes it stale
	oracleOwner.Invoke(t, stackitem.Null{}, "setParams", 1, 1)
	r1 := e.NewAccount(t)
	oracleOwner.Invoke(t, stackitem.Null{}, "setReporter", r1.ScriptHash().BytesBE(), true)
	e.NewInvoker(oracleHash, r1).Invoke(t, stackitem.Null{}, "report", r1.ScriptHash().BytesBE(), asset.BytesBE(), 500)

	// neotest advances block timestamps by 1ms; one empty block puts now - reportTime = 2ms,
	// past the 1ms freshness bound, so no fresh prices remain
	e.AddNewBlock(t)
	oracleOwner.InvokeFail(t, "not enough fresh prices", "price", asset.BytesBE())
}
