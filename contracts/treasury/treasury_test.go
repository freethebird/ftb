// Neotest suite for the DE-KEYED treasury. Every fund action is authorised by a passed FtbGov
// proposal (staked-FTB vote), not a committee key (FTB-B1): a proposer reads the action's
// subject from a view, FtbGov.propose + vote decides it, and ANYONE executes it. The audited
// hardening is unchanged -- dev!=auditor, vetted auditor set, soak floor, bound NEF hash
// (T1/T2) -- only the authorisation source moved from a key to a vote. Self-service claims
// (attest/claimA/claimB/failAudit) are untouched.
//
// Run: go test ./contracts/treasury/
package treasury_test

import (
	"bytes"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const soak = 5760
const minVotingPeriod = 5760

var testNefHash = bytes.Repeat([]byte{0xab}, 32)

type sys struct {
	e                            *neotest.Executor
	owner, gov1, dev, auditor    neotest.Signer
	other                        neotest.Signer
	ftb, div, gov, treasury, reg util.Uint160
	govCount                     int
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.gov1 = e.NewAccount(t) // the governor: stakes FTB, so it IS the electorate
	s.dev = e.NewAccount(t)
	s.auditor = e.NewAccount(t)
	s.other = e.NewAccount(t)

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	s.ftb = ftbC.Hash

	divC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../dividends/dividends.go", "../dividends/dividends.yml")
	e.DeployContract(t, divC, s.owner.ScriptHash().BytesBE())
	s.div = divC.Hash
	e.NewInvoker(s.div, s.owner).Invoke(t, stackitem.Null{}, "configure", s.ftb.BytesBE())

	govC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_gov/ftb_gov.go", "../ftb_gov/ftb_gov.yml")
	e.DeployContract(t, govC, s.owner.ScriptHash().BytesBE())
	s.gov = govC.Hash
	e.NewInvoker(s.gov, s.owner).Invoke(t, stackitem.Null{}, "configure", s.div.BytesBE())

	regC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../registry/registry.go", "../registry/registry.yml")
	e.DeployContract(t, regC, s.owner.ScriptHash().BytesBE())
	s.reg = regC.Hash
	e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "setInitialDelay", 1)

	trC := neotest.CompileFile(t, e.Validator.ScriptHash(), "treasury.go", "treasury.yml")
	e.DeployContract(t, trC, s.owner.ScriptHash().BytesBE())
	s.treasury = trC.Hash
	e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Null{},
		"configure", s.ftb.BytesBE(), s.gov.BytesBE(), s.reg.BytesBE())

	// the governor stakes FTB -> becomes the voting electorate
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.gov1.ScriptHash().BytesBE(), 1_000_000)
	e.NewInvoker(s.ftb, s.gov1).Invoke(t, stackitem.Make(true), "transfer",
		s.gov1.ScriptHash().BytesBE(), s.div.BytesBE(), 1_000_000, []any{"stake"})

	// enroll the auditor through governance (setAuditor is now vote-bound)
	sub := s.subjectOf(t, "subjectForSetAuditor", s.auditor.ScriptHash().BytesBE(), true)
	id := s.govPass(t, sub)
	e.NewInvoker(s.treasury, s.other).Invoke(t, stackitem.Null{},
		"setAuditor", id, s.auditor.ScriptHash().BytesBE(), true)

	// the treasury starts with income: 1000 mock-FTB
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.owner.ScriptHash().BytesBE(), 1000)
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(true), "transfer",
		s.owner.ScriptHash().BytesBE(), s.treasury.BytesBE(), 1000, nil)
	return s
}

// subjectOf reads an action's subject from the treasury's on-chain view -- no off-chain hashing.
func (s *sys) subjectOf(t *testing.T, method string, args ...any) []byte {
	var out []byte
	s.e.NewInvoker(s.treasury, s.owner).InvokeAndCheck(t, func(t testing.TB, st []stackitem.Item) {
		out = st[0].Value().([]byte)
	}, method, args...)
	return out
}

// govPass proposes a subject, has the governor vote YES (the whole electorate), and closes the
// window -- so the proposal passes. Returns the proposal id (= the escrow id for createEscrow).
func (s *sys) govPass(t *testing.T, subject []byte) int {
	s.govCount++
	id := s.govCount
	s.e.NewInvoker(s.gov, s.owner).Invoke(t, stackitem.Make(id), "propose", subject, minVotingPeriod)
	s.e.NewInvoker(s.gov, s.gov1).Invoke(t, stackitem.Null{}, "vote", id, s.gov1.ScriptHash().BytesBE(), true)
	s.e.GenerateNewBlocks(t, minVotingPeriod+1)
	return id
}

func (s *sys) bal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}

// create runs a governed createEscrow and returns the escrow id (= the proposal id).
func (s *sys) create(t *testing.T, amount, bps, soakBlocks int) int {
	dev := s.dev.ScriptHash().BytesBE()
	aud := s.auditor.ScriptHash().BytesBE()
	sub := s.subjectOf(t, "subjectForCreateEscrow", dev, aud, amount, bps, soakBlocks, testNefHash)
	id := s.govPass(t, sub)
	s.e.NewInvoker(s.treasury, s.other).Invoke(t, stackitem.Null{}, "createEscrow",
		id, dev, aud, amount, bps, soakBlocks, testNefHash) // ANYONE may execute a passed proposal
	return id
}

func TestGovLifecycle(t *testing.T) {
	s := setup(t)
	tr := s.treasury
	id := s.create(t, 600, 3000, soak)
	s.bal(t, tr, 1000)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(400), "free")
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(1), "state", id)

	// self-service claims are unchanged
	s.e.NewInvoker(tr, s.auditor).Invoke(t, stackitem.Null{}, "attest", id, testNefHash)
	s.e.NewInvoker(tr, s.dev).Invoke(t, stackitem.Null{}, "claimA", id)
	s.bal(t, s.dev.ScriptHash(), 180) // 30% of 600
	s.e.NewInvoker(tr, s.dev).InvokeFail(t, "soak period still running", "claimB", id)
	s.e.GenerateNewBlocks(t, soak+2)
	s.e.NewInvoker(tr, s.dev).Invoke(t, stackitem.Null{}, "claimB", id)
	s.bal(t, s.dev.ScriptHash(), 600)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(4), "state", id)
}

func TestCreateRequiresPassedVote(t *testing.T) {
	s := setup(t)
	dev := s.dev.ScriptHash().BytesBE()
	aud := s.auditor.ScriptHash().BytesBE()
	// valid params but no passed proposal with id 999 -> requireGov rejects
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "proposal has not passed", "createEscrow",
		999, dev, aud, 100, 3000, soak, testNefHash)
}

func TestCreateSubjectMustMatch(t *testing.T) {
	s := setup(t)
	dev := s.dev.ScriptHash().BytesBE()
	aud := s.auditor.ScriptHash().BytesBE()
	// vote authorises amount 100 ...
	sub := s.subjectOf(t, "subjectForCreateEscrow", dev, aud, 100, 3000, soak, testNefHash)
	id := s.govPass(t, sub)
	// ... but execution tries amount 200 -> subject mismatch
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "subject does not match", "createEscrow",
		id, dev, aud, 200, 3000, soak, testNefHash)
}

func TestConsumedGuard(t *testing.T) {
	s := setup(t)
	id := s.create(t, 100, 3000, soak)
	// the same passed proposal cannot be executed twice
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "already executed", "createEscrow",
		id, s.dev.ScriptHash().BytesBE(), s.auditor.ScriptHash().BytesBE(), 100, 3000, soak, testNefHash)
}

func TestParamValidationBeforeVote(t *testing.T) {
	s := setup(t)
	dev := s.dev.ScriptHash().BytesBE()
	aud := s.auditor.ScriptHash().BytesBE()
	// these fault on the parameter check, before requireGov -- no proposal needed
	in := s.e.NewInvoker(s.treasury, s.other)
	in.InvokeFail(t, "bad dev/auditor", "createEscrow", 1, dev, dev, 100, 3000, soak, testNefHash)
	in.InvokeFail(t, "expected NEF hash must be 32 bytes", "createEscrow", 1, dev, aud, 100, 3000, soak, []byte{1})
	in.InvokeFail(t, "bad terms", "createEscrow", 1, dev, aud, 100, 0, soak, testNefHash)
	in.InvokeFail(t, "bad terms", "createEscrow", 1, dev, aud, 100, 10000, soak, testNefHash)
	in.InvokeFail(t, "soak below the minimum floor", "createEscrow", 1, dev, aud, 100, 3000, soak-1, testNefHash)
}

func TestAuditorMustBeVetted(t *testing.T) {
	s := setup(t)
	dev := s.dev.ScriptHash().BytesBE()
	unvetted := s.other.ScriptHash().BytesBE()
	// a passed proposal for an escrow naming an UN-enrolled auditor still fails the vetted check
	sub := s.subjectOf(t, "subjectForCreateEscrow", dev, unvetted, 100, 3000, soak, testNefHash)
	id := s.govPass(t, sub)
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "auditor not on the vetted set", "createEscrow",
		id, dev, unvetted, 100, 3000, soak, testNefHash)
}

func TestCancelViaGov(t *testing.T) {
	s := setup(t)
	id := s.create(t, 500, 3000, soak)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(500), "free") // 1000 - 500 escrowed
	// governed cancel returns the budget
	sub := s.subjectOf(t, "subjectForCancel", id)
	cid := s.govPass(t, sub)
	s.e.NewInvoker(s.treasury, s.other).Invoke(t, stackitem.Null{}, "cancel", cid, id)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(5), "state", id)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(1000), "free")
}

func TestReturnExcessViaGov(t *testing.T) {
	s := setup(t)
	dividends := s.e.NewAccount(t)
	s.e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "bootstrap", "dividends",
		dividends.ScriptHash().BytesBE())
	sub := s.subjectOf(t, "subjectForReturnExcess", 400)
	id := s.govPass(t, sub)
	s.e.NewInvoker(s.treasury, s.other).Invoke(t, stackitem.Null{}, "returnExcess", id, 400)
	s.bal(t, dividends.ScriptHash(), 400)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(600), "free")
}

func TestFailAuditReturnsBudget(t *testing.T) {
	s := setup(t)
	id := s.create(t, 500, 3000, soak)
	s.e.NewInvoker(s.treasury, s.auditor).Invoke(t, stackitem.Null{}, "failAudit", id)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(6), "state", id)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(1000), "free")
}

func TestRejectsForeignToken(t *testing.T) {
	s := setup(t)
	gas := s.e.NativeHash(t, nativenames.Gas)
	s.e.NewInvoker(gas, s.owner).InvokeFail(t, "only FTB accepted", "transfer",
		s.owner.ScriptHash().BytesBE(), s.treasury.BytesBE(), 1, nil)
}

func TestConfigureWriteOnce(t *testing.T) {
	s := setup(t)
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only owner", "configure",
		s.ftb.BytesBE(), s.gov.BytesBE(), s.reg.BytesBE())
	s.e.NewInvoker(s.treasury, s.owner).InvokeFail(t, "write-once", "configure",
		s.ftb.BytesBE(), s.gov.BytesBE(), s.reg.BytesBE())
}
