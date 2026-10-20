// Neotest suite for the dividends module. A mock NEP-17 stands in for FTB. Covers the
// pro-rata accumulator (two stakers, 1:3), queueing of revenue that arrives before any
// stake, settlement at the old balance on re-stake, unstake semantics, claim access
// control, foreign-token rejection, and write-once configure.
//
// Run: go test ./contracts/dividends/
package dividends_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

// must mirror minStakeBlocks in dividends.go (the D-2 unstake lock)
const minStakeBlocks = 5760

type sys struct {
	e                        *neotest.Executor
	owner, alice, bob, other neotest.Signer
	ftb, div                 util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.alice = e.NewAccount(t)
	s.bob = e.NewAccount(t)
	s.other = e.NewAccount(t)

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	s.ftb = ftbC.Hash

	divC := neotest.CompileFile(t, e.Validator.ScriptHash(), "dividends.go", "dividends.yml")
	e.DeployContract(t, divC, s.owner.ScriptHash().BytesBE())
	s.div = divC.Hash
	e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Null{}, "configure", s.ftb.BytesBE())

	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.alice.ScriptHash().BytesBE(), 10000)
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.bob.ScriptHash().BytesBE(), 10000)
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.owner.ScriptHash().BytesBE(), 10000)
	return s
}

// stake moves FTB into the module as a stake; revenue sends it as a distribution.
func (s *sys) stake(t *testing.T, who neotest.Signer, amount int) {
	s.e.NewInvoker(s.ftb, who).Invoke(t, stackitem.Make(true), "transfer",
		who.ScriptHash().BytesBE(), s.div.BytesBE(), amount, []any{"stake"})
}
func (s *sys) revenue(t *testing.T, amount int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(true), "transfer",
		s.owner.ScriptHash().BytesBE(), s.div.BytesBE(), amount, nil)
}
func (s *sys) bal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) pending(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Make(want), "pendingOf", h.BytesBE())
}

func TestProRataClaim(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 100)
	s.stake(t, s.bob, 300)
	s.revenue(t, 400)

	s.pending(t, s.alice.ScriptHash(), 100) // 1/4 of 400
	s.pending(t, s.bob.ScriptHash(), 300)   // 3/4

	// only the earner claims, exactly once
	s.e.NewInvoker(s.div, s.other).InvokeFail(t, "no witness", "claim", s.alice.ScriptHash().BytesBE())
	s.e.NewInvoker(s.div, s.alice).Invoke(t, stackitem.Null{}, "claim", s.alice.ScriptHash().BytesBE())
	s.bal(t, s.alice.ScriptHash(), 10000-100+100) // stake left, dividend returned
	s.e.NewInvoker(s.div, s.alice).InvokeFail(t, "nothing to claim", "claim", s.alice.ScriptHash().BytesBE())

	// new revenue accrues on top for the remaining staker
	s.revenue(t, 200)
	s.pending(t, s.alice.ScriptHash(), 50) // already-claimed portion is not re-payable
	s.pending(t, s.bob.ScriptHash(), 300+150)
}

func TestRevenueBeforeStakersQueues(t *testing.T) {
	s := setup(t)
	s.revenue(t, 500) // nobody staked: queued, not lost
	s.e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Make(500), "queued")
	s.e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Make(0), "accPerShare")

	s.stake(t, s.alice, 100)
	s.pending(t, s.alice.ScriptHash(), 0) // queued revenue is NOT retroactive to the first staker

	s.revenue(t, 100) // folds the queue: 500 + 100 over 100 staked
	s.e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Make(0), "queued")
	s.pending(t, s.alice.ScriptHash(), 600)
	s.e.NewInvoker(s.div, s.alice).Invoke(t, stackitem.Null{}, "claim", s.alice.ScriptHash().BytesBE())
	s.bal(t, s.alice.ScriptHash(), 10000-100+600)
}

// The monthly ledger counts revenue when RECEIVED (stakes are not revenue),
// so the dividends module can show this month vs last month without an indexer.
func TestMonthlyRevenueLedger(t *testing.T) {
	s := setup(t)
	dv := s.e.NewInvoker(s.div, s.owner)

	s.stake(t, s.alice, 100)
	dv.Invoke(t, stackitem.Make(0), "totalReceived") // stakes are not revenue

	s.revenue(t, 500)
	dv.Invoke(t, stackitem.Make(0), "monthIndex") // a fresh test chain is in month 0
	dv.Invoke(t, stackitem.Make(500), "monthTotal", 0)
	dv.Invoke(t, stackitem.Make(500), "totalReceived")

	s.revenue(t, 100)
	dv.Invoke(t, stackitem.Make(600), "monthTotal", 0)
	dv.Invoke(t, stackitem.Make(600), "totalReceived")

	dv.InvokeFail(t, "bad month", "monthTotal", -1)
}

func TestRestakeSettlesAtOldBalance(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 100)
	s.revenue(t, 100) // 100% to alice: acc = 1 FTB per share
	s.stake(t, s.alice, 100)
	s.pending(t, s.alice.ScriptHash(), 100) // settled from the old 100-share balance

	s.revenue(t, 100) // now over 200 staked: +0.5 per share
	s.pending(t, s.alice.ScriptHash(), 100+100)
}

func TestUnstakeStopsAccrual(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 100)
	s.revenue(t, 100)
	s.e.GenerateNewBlocks(t, minStakeBlocks) // D-2: wait out the commitment window before unstaking
	s.e.NewInvoker(s.div, s.alice).Invoke(t, stackitem.Null{}, "unstake", s.alice.ScriptHash().BytesBE(), 100)
	s.bal(t, s.alice.ScriptHash(), 10000) // principal back in full
	s.pending(t, s.alice.ScriptHash(), 100)

	s.revenue(t, 50) // nothing staked: queues, does not accrue to alice
	s.pending(t, s.alice.ScriptHash(), 100)
	s.e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Make(50), "queued")

	// earned dividends survive a full unstake
	s.e.NewInvoker(s.div, s.alice).Invoke(t, stackitem.Null{}, "claim", s.alice.ScriptHash().BytesBE())
	s.bal(t, s.alice.ScriptHash(), 10000+100)
}

func TestUnstakeValidation(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 100)
	// witness and amount bounds are checked before the D-2 lock, so they fire without maturing
	s.e.NewInvoker(s.div, s.other).InvokeFail(t, "no witness", "unstake", s.alice.ScriptHash().BytesBE(), 50)
	s.e.NewInvoker(s.div, s.alice).InvokeFail(t, "bad amount", "unstake", s.alice.ScriptHash().BytesBE(), 0)
	s.e.NewInvoker(s.div, s.alice).InvokeFail(t, "bad amount", "unstake", s.alice.ScriptHash().BytesBE(), 101)
}

// D-2: principal is locked for minStakeBlocks after staking, but earned dividends remain
// claimable immediately — a same-block / flash JIT staker cannot capture a distribution and
// exit, which was the reserve-dilution vector.
func TestMinStakeLock(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 100)
	s.revenue(t, 100)

	divAlice := s.e.NewInvoker(s.div, s.alice)
	// unstake is locked right after staking (no same-block exit)
	divAlice.InvokeFail(t, "minimum stake period not elapsed", "unstake", s.alice.ScriptHash().BytesBE(), 100)
	// the earned dividend is claimable immediately — Claim is not gated
	divAlice.Invoke(t, stackitem.Null{}, "claim", s.alice.ScriptHash().BytesBE())
	s.bal(t, s.alice.ScriptHash(), 10000) // -100 stake, +100 dividend

	// once the commitment window elapses, principal unstakes
	s.e.GenerateNewBlocks(t, minStakeBlocks)
	divAlice.Invoke(t, stackitem.Null{}, "unstake", s.alice.ScriptHash().BytesBE(), 100)
	s.bal(t, s.alice.ScriptHash(), 10100)
}

func TestRejectsForeignToken(t *testing.T) {
	s := setup(t)
	gas := s.e.NativeHash(t, nativenames.Gas)
	s.e.NewInvoker(gas, s.owner).InvokeFail(t, "only FTB accepted", "transfer",
		s.owner.ScriptHash().BytesBE(), s.div.BytesBE(), 1, nil)
}

func TestConfigureWriteOnce(t *testing.T) {
	s := setup(t)
	s.e.NewInvoker(s.div, s.other).InvokeFail(t, "only owner", "configure", s.ftb.BytesBE())
	s.e.NewInvoker(s.div, s.owner).InvokeFail(t, "write-once", "configure", s.ftb.BytesBE())
}

// C3/D-1 remediation: the bytecode upgrade over all staked FTB is two-step and self-timelocked.
// Exercises the gating without landing a real NEF (a bogus NEF would fault inside management.update).
func TestUpgradeTimelock(t *testing.T) {
	s := setup(t)
	dv := s.e.NewInvoker(s.div, s.owner)
	nef, man := []byte{0x01}, []byte{0x02}

	s.e.NewInvoker(s.div, s.other).InvokeFail(t, "only owner", "proposeUpgrade", nef, man)
	dv.InvokeFail(t, "nothing pending", "applyUpgrade")
	dv.Invoke(t, stackitem.Null{}, "proposeUpgrade", nef, man)
	dv.InvokeFail(t, "timelock not elapsed", "applyUpgrade")
	dv.Invoke(t, stackitem.Null{}, "cancelUpgrade")
	dv.Invoke(t, stackitem.Make(0), "pendingUpgradeEta")
	dv.InvokeFail(t, "nothing pending", "applyUpgrade")
}
