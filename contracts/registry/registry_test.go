// Reference neotest suite for the Registry seam. Covers bootstrap gating, the two-step
// timelock (propose, wait, apply), apply-by-anyone, delay-can-only-increase, and the
// H4/M3 timelock floor and ceiling.
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

// must mirror the floor/ceiling constants in registry.go (H4/M3 remediation)
const (
	minDelay = 5760
	maxDelay = 2102400
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

	// owner sets the timelock at the floor, bootstraps a value, finishes bootstrap.
	// H4 remediation: finishBootstrap refuses any delay below minDelay.
	reg.Invoke(t, stackitem.Null{}, "setInitialDelay", minDelay)
	reg.Invoke(t, stackitem.Null{}, "bootstrap", "token", v1.BytesBE())
	// newer VM returns Buffer for stored byte values; NewByteArray would mismatch
	reg.Invoke(t, stackitem.NewBuffer(v1.BytesBE()), "get", "token")
	reg.Invoke(t, stackitem.Null{}, "finishBootstrap")

	// bootstrap is closed now
	reg.InvokeFail(t, "bootstrap already finished", "bootstrap", "token", v2.BytesBE())

	// propose a change; applying before the timelock elapses must fail
	reg.Invoke(t, stackitem.Null{}, "propose", "token", v2.BytesBE())
	reg.InvokeFail(t, "timelock not elapsed", "apply", "token")

	// mine past the (one-day) timelock, then anyone can apply
	e.GenerateNewBlocks(t, minDelay+2)
	otherReg.Invoke(t, stackitem.Null{}, "apply", "token")
	reg.Invoke(t, stackitem.NewBuffer(v2.BytesBE()), "get", "token")
}

func TestDelayOnlyIncreasesWithinBounds(t *testing.T) {
	_, reg, _ := deployRegistry(t)
	reg.Invoke(t, stackitem.Null{}, "setInitialDelay", minDelay)
	// cannot shorten the timelock
	reg.InvokeFail(t, "delay can only increase", "increaseDelay", minDelay-1)
	// M3 remediation: cannot ratchet past the ceiling (no governance-freeze dressed as safety)
	reg.InvokeFail(t, "delay above the maximum", "increaseDelay", maxDelay+1)
	// a legitimate increase within bounds is accepted
	reg.Invoke(t, stackitem.Null{}, "increaseDelay", minDelay*2)
}

func TestCannotFinishBelowFloor(t *testing.T) {
	_, reg, _ := deployRegistry(t)
	// delay defaults to 0; finishing must be refused by the floor
	reg.InvokeFail(t, "below the minimum floor", "finishBootstrap")
	// a non-zero but sub-floor delay is still refused: a 1-block "timelock" is not a timelock
	reg.Invoke(t, stackitem.Null{}, "setInitialDelay", minDelay-1)
	reg.InvokeFail(t, "below the minimum floor", "finishBootstrap")
	// exactly at the floor, bootstrap can finish
	reg.Invoke(t, stackitem.Null{}, "setInitialDelay", minDelay)
	reg.Invoke(t, stackitem.Null{}, "finishBootstrap")
}
