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

	// freshness of 1ms with the quorum of 3: block advancement ages the reports out
	oracleOwner.Invoke(t, stackitem.Null{}, "setParams", 1, 3)
	r1, r2, r3 := e.NewAccount(t), e.NewAccount(t), e.NewAccount(t)
	for _, r := range []neotest.Signer{r1, r2, r3} {
		oracleOwner.Invoke(t, stackitem.Null{}, "setReporter", r.ScriptHash().BytesBE(), true)
		e.NewInvoker(oracleHash, r).Invoke(t, stackitem.Null{}, "report", r.ScriptHash().BytesBE(), asset.BytesBE(), 500)
	}
	// advance a block so the reports age past the 1ms freshness bound -> no fresh quorum
	e.AddNewBlock(t)
	oracleOwner.InvokeFail(t, "not enough fresh prices", "price", asset.BytesBE())
}

// O1 remediation: minReports can never be set below the quorum floor (no single-reporter feed).
func TestMinReportsFloorEnforced(t *testing.T) {
	_, oracleOwner, _ := deployOracle(t)
	oracleOwner.InvokeFail(t, "below the quorum floor", "setParams", 100_000_000, 1)
	oracleOwner.InvokeFail(t, "below the quorum floor", "setParams", 100_000_000, 2)
	oracleOwner.Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 3) // 3 is allowed
}

// O2 remediation: once a quorum price exists, a single report outside the deviation band
// is rejected (circuit-breaker); a report within the band is accepted.
func TestCircuitBreakerBoundsReports(t *testing.T) {
	e, oracleOwner, oracleHash := deployOracle(t)
	asset := e.NewAccount(t).ScriptHash()
	oracleOwner.Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 3)
	r1, r2, r3 := e.NewAccount(t), e.NewAccount(t), e.NewAccount(t)
	for _, r := range []neotest.Signer{r1, r2, r3} {
		oracleOwner.Invoke(t, stackitem.Null{}, "setReporter", r.ScriptHash().BytesBE(), true)
		e.NewInvoker(oracleHash, r).Invoke(t, stackitem.Null{}, "report", r.ScriptHash().BytesBE(), asset.BytesBE(), 1000)
	}
	// quorum established at median 1000; a re-report 2x over (beyond the 20% band) is rejected
	e.NewInvoker(oracleHash, r1).InvokeFail(t, "circuit-breaker", "report", r1.ScriptHash().BytesBE(), asset.BytesBE(), 2000)
	// a report within the band (+10%) is accepted
	e.NewInvoker(oracleHash, r1).Invoke(t, stackitem.Null{}, "report", r1.ScriptHash().BytesBE(), asset.BytesBE(), 1100)
}
