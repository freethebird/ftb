// Neotest suite for FtbGov: the staked-FTB governance executor. Exercises propose/vote, the
// 2/3 supermajority and 1/6 quorum gates, the voting window, one-vote-per-address, stake-
// weighting, write-once configure, and the one-way renounce. Uses the real dividends module
// (the vote-weight source) over a mock FTB token.
package ftbgov_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const minVotingPeriod = 5760

type sys struct {
	e                        *neotest.Executor
	owner, alice, bob, carol neotest.Signer
	ftb, div, gov            util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.alice = e.NewAccount(t)
	s.bob = e.NewAccount(t)
	s.carol = e.NewAccount(t)

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	s.ftb = ftbC.Hash

	divC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../dividends/dividends.go", "../dividends/dividends.yml")
	e.DeployContract(t, divC, s.owner.ScriptHash().BytesBE())
	s.div = divC.Hash
	e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Null{}, "configure", s.ftb.BytesBE())

	govC := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_gov.go", "ftb_gov.yml")
	e.DeployContract(t, govC, s.owner.ScriptHash().BytesBE())
	s.gov = govC.Hash
	e.NewInvoker(s.gov, s.owner).Invoke(t, stackitem.Null{}, "configure", s.div.BytesBE())

	for _, who := range []neotest.Signer{s.alice, s.bob, s.carol} {
		e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", who.ScriptHash().BytesBE(), 1_000_000)
	}
	return s
}

func (s *sys) stake(t *testing.T, who neotest.Signer, amt int) {
	s.e.NewInvoker(s.ftb, who).Invoke(t, stackitem.Make(true), "transfer",
		who.ScriptHash().BytesBE(), s.div.BytesBE(), amt, []any{"stake"})
}
func (s *sys) govAs(who neotest.Signer) *neotest.ContractInvoker { return s.e.NewInvoker(s.gov, who) }
func (s *sys) vote(t *testing.T, who neotest.Signer, id int, yes bool) {
	s.govAs(who).Invoke(t, stackitem.Null{}, "vote", id, who.ScriptHash().BytesBE(), yes)
}
func (s *sys) passed(t *testing.T, id int, want bool) {
	s.govAs(s.owner).Invoke(t, stackitem.Make(want), "passed", id)
}

func subj() []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i + 7)
	}
	return b
}

func TestProposeVotePasses(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 4000)
	s.stake(t, s.bob, 2000) // total staked 6000 -> quorum floor 1000
	s.govAs(s.owner).Invoke(t, stackitem.Make(1), "propose", subj(), minVotingPeriod)
	s.vote(t, s.alice, 1, true)
	s.vote(t, s.bob, 1, true) // 6000 YES, 0 NO
	s.passed(t, 1, false)     // window still open
	s.e.GenerateNewBlocks(t, minVotingPeriod+1)
	s.passed(t, 1, true) // closed, quorum 6000>=1000, YES 100% >= 2/3
}

func TestFailsQuorum(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 6000)
	s.stake(t, s.bob, 100) // total 6100 -> need turnout*6 >= 6100 (>= ~1017)
	s.govAs(s.owner).Invoke(t, stackitem.Make(1), "propose", subj(), minVotingPeriod)
	s.vote(t, s.bob, 1, true) // only 100 votes; alice abstains
	s.e.GenerateNewBlocks(t, minVotingPeriod+1)
	s.passed(t, 1, false) // 100*6=600 < 6100 -> quorum not met
}

func TestFailsSupermajority(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 4000)
	s.stake(t, s.bob, 2000) // total 6000, quorum 1000
	s.govAs(s.owner).Invoke(t, stackitem.Make(1), "propose", subj(), minVotingPeriod)
	s.vote(t, s.alice, 1, false) // 4000 NO
	s.vote(t, s.bob, 1, true)    // 2000 YES; cast 6000, YES=1/3
	s.e.GenerateNewBlocks(t, minVotingPeriod+1)
	s.passed(t, 1, false) // 2000*3=6000 < 6000*2=12000 -> below 2/3
}

func TestOneVotePerAddress(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 4000)
	s.govAs(s.owner).Invoke(t, stackitem.Make(1), "propose", subj(), minVotingPeriod)
	s.vote(t, s.alice, 1, true)
	s.govAs(s.alice).InvokeFail(t, "already voted", "vote", 1, s.alice.ScriptHash().BytesBE(), false)
}

func TestNoStakeNoVote(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 4000) // alice gives the proposal a quorum base
	s.govAs(s.owner).Invoke(t, stackitem.Make(1), "propose", subj(), minVotingPeriod)
	s.govAs(s.carol).InvokeFail(t, "no staked weight", "vote", 1, s.carol.ScriptHash().BytesBE(), true)
}

func TestWindowClosedAndValidation(t *testing.T) {
	s := setup(t)
	// propose with no stake at all -> no quorum base
	s.govAs(s.owner).InvokeFail(t, "no staked supply", "propose", subj(), minVotingPeriod)
	s.stake(t, s.alice, 4000)
	s.govAs(s.owner).InvokeFail(t, "subject must be a 32-byte hash", "propose", []byte{1, 2, 3}, minVotingPeriod)
	s.govAs(s.owner).InvokeFail(t, "voting period out of range", "propose", subj(), 10)
	s.govAs(s.owner).Invoke(t, stackitem.Make(1), "propose", subj(), minVotingPeriod)
	s.e.GenerateNewBlocks(t, minVotingPeriod+1)
	s.govAs(s.alice).InvokeFail(t, "window closed", "vote", 1, s.alice.ScriptHash().BytesBE(), true)
}

func TestConfigureAndRenounce(t *testing.T) {
	s := setup(t)
	// already configured in setup
	s.govAs(s.owner).InvokeFail(t, "already configured", "configure", s.div.BytesBE())
	s.govAs(s.alice).InvokeFail(t, "only owner", "renounceOwnership")
	s.govAs(s.owner).Invoke(t, stackitem.Null{}, "renounceOwnership")
	s.govAs(s.owner).InvokeFail(t, "only owner", "renounceOwnership")
}

// TestPassedUnknownLargeId guards the idKey byte-mask fix: passed() on an unknown id whose
// packed bytes exceed 255 must return false, not fault with SETITEM "invalid value".
func TestPassedUnknownLargeId(t *testing.T) {
	s := setup(t)
	s.stake(t, s.alice, 4000)
	s.govAs(s.owner).Invoke(t, stackitem.Make(false), "passed", 999)
	s.govAs(s.owner).Invoke(t, stackitem.Make(false), "passed", 70000)
}
