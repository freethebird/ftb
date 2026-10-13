// Reference neotest suite for the Registry seam. Covers bootstrap gating, the two-step
// timelock (propose, wait, apply), apply-by-anyone, and delay-can-only-increase.
//
// Hash160 result assertions below use BytesBE; if your neo-go serializes them the other
// way, flip to BytesLE (test-only, does not affect contract correctness).
//
// Run: go test ./contracts/registry/
package registry_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func deployRegistry(t *testing.T) (*neotest.Executor, *neotest.ContractInvoker, neotest.Signer) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "registry.go", "registry.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	return e, e.NewInvoker(c.Hash, owner), owner
}

func TestBootstrapAndTimelock(t *testing.T) {
	e, reg, _ := deployRegistry(t)
	other := e.NewAccount(t)
	otherReg := e.NewInvoker(reg.Hash, other)

	v1 := e.NewAccount(t).ScriptHash()
	v2 := e.NewAccount(t).ScriptHash()

	// only owner can bootstrap
	otherReg.InvokeFail(t, "only owner", "bootstrap", "token", v1.BytesBE())

	// owner sets a small timelock, bootstraps a value, finishes bootstrap
	reg.Invoke(t, stackitem.Null{}, "setInitialDelay", 2)
	reg.Invoke(t, stackitem.Null{}, "bootstrap", "token", v1.BytesBE())
	// newer VM returns Buffer for stored byte values; NewByteArray would mismatch
	reg.Invoke(t, stackitem.NewBuffer(v1.BytesBE()), "get", "token")
	reg.Invoke(t, stackitem.Null{}, "finishBootstrap")

	// bootstrap is closed now
	reg.InvokeFail(t, "bootstrap already finished", "bootstrap", "token", v2.BytesBE())

	// propose a change; applying before the timelock elapses must fail
	reg.Invoke(t, stackitem.Null{}, "propose", "token", v2.BytesBE())
	reg.InvokeFail(t, "timelock not elapsed", "apply", "token")

	// mine past the timelock, then anyone can apply
	e.AddNewBlock(t)
	e.AddNewBlock(t)
	otherReg.Invoke(t, stackitem.Null{}, "apply", "token")
	reg.Invoke(t, stackitem.NewBuffer(v2.BytesBE()), "get", "token")
}

func TestDelayOnlyIncreases(t *testing.T) {
	_, reg, _ := deployRegistry(t)
	reg.Invoke(t, stackitem.Null{}, "setInitialDelay", 3)
	reg.InvokeFail(t, "delay can only increase", "increaseDelay", 2)
	reg.Invoke(t, stackitem.Null{}, "increaseDelay", 10)
}

func TestCannotFinishWithZeroTimelock(t *testing.T) {
	_, reg, _ := deployRegistry(t)
	// delay defaults to 0; finishing must be refused
	reg.InvokeFail(t, "non-zero timelock", "finishBootstrap")
}
