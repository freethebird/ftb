// Neotest suite for the DE-KEYED bond vault. Register/slash/delist are each authorised by a
// passed FtbGov proposal (staked-FTB vote) and executed by ANYONE -- no committee key
// (FTB-B1). The BV-01/BV-02 hardening is intact: the refund target and slash sink are bound
// at registration (from the authorising vote), the sink may not be the project, the first
// bond cannot open a listing, and closed listings are final.
//
// Run: go test ./contracts/bondvault/
package bondvault_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/stretchr/testify/require"
)

const minVotingPeriod = 5760

type sys struct {
	e                                    *neotest.Executor
	owner, gov1, project, comp, stranger neotest.Signer
	ftb, div, gov, vault, listed         util.Uint160
	govCount                             int
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.gov1 = e.NewAccount(t)
	s.project = e.NewAccount(t)
	s.comp = e.NewAccount(t)
	s.stranger = e.NewAccount(t)
	s.listed = e.NewAccount(t).ScriptHash()

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

	vC := neotest.CompileFile(t, e.Validator.ScriptHash(), "bondvault.go", "bondvault.yml")
	e.DeployContract(t, vC, s.owner.ScriptHash().BytesBE())
	s.vault = vC.Hash
	e.NewInvoker(s.vault, s.owner).Invoke(t, stackitem.Null{}, "configure", s.ftb.BytesBE(), s.gov.BytesBE())

	// the governor stakes FTB -> becomes the voting electorate
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.gov1.ScriptHash().BytesBE(), 1_000_000)
	e.NewInvoker(s.ftb, s.gov1).Invoke(t, stackitem.Make(true), "transfer",
		s.gov1.ScriptHash().BytesBE(), s.div.BytesBE(), 1_000_000, []any{"stake"})

	// the project has FTB to bond
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.project.ScriptHash().BytesBE(), 1000)
	return s
}

func (s *sys) subjectOf(t *testing.T, method string, args ...any) []byte {
	var out []byte
	s.e.NewInvoker(s.vault, s.e.Validator).InvokeAndCheck(t, func(t testing.TB, st []stackitem.Item) {
		out = st[0].Value().([]byte)
	}, method, args...)
	return out
}
func (s *sys) govPass(t *testing.T, subject []byte) int {
	s.govCount++
	id := s.govCount
	s.e.NewInvoker(s.gov, s.owner).Invoke(t, stackitem.Make(id), "propose", subject, minVotingPeriod)
	s.e.NewInvoker(s.gov, s.gov1).Invoke(t, stackitem.Null{}, "vote", id, s.gov1.ScriptHash().BytesBE(), true)
	s.e.GenerateNewBlocks(t, minVotingPeriod+1)
	return id
}
func (s *sys) register(t *testing.T) {
	sub := s.subjectOf(t, "subjectForRegisterListing", s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
	id := s.govPass(t, sub)
	s.e.NewInvoker(s.vault, s.stranger).Invoke(t, stackitem.Null{}, "registerListing",
		id, s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
}
func (s *sys) slash(t *testing.T) {
	id := s.govPass(t, s.subjectOf(t, "subjectForSlash", s.listed.BytesBE()))
	s.e.NewInvoker(s.vault, s.stranger).Invoke(t, stackitem.Null{}, "slash", id, s.listed.BytesBE())
}
func (s *sys) delist(t *testing.T) {
	id := s.govPass(t, s.subjectOf(t, "subjectForDelist", s.listed.BytesBE()))
	s.e.NewInvoker(s.vault, s.stranger).Invoke(t, stackitem.Null{}, "delist", id, s.listed.BytesBE())
}
func (s *sys) bond(t *testing.T, amount int) {
	s.e.NewInvoker(s.ftb, s.project).Invoke(t, stackitem.NewBool(true), "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), amount, s.listed.BytesBE())
}
func (s *sys) checkBalance(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.e.Validator).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) checkView(t *testing.T, method string, want int) {
	s.e.NewInvoker(s.vault, s.e.Validator).Invoke(t, stackitem.Make(want), method, s.listed.BytesBE())
}

func TestBondSplitsAndTopsUp(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 101)
	s.checkView(t, "state", 2)
	s.checkView(t, "vaultOf", 51)
	s.checkView(t, "liquidityOf", 50)
	s.bond(t, 50)
	s.checkView(t, "vaultOf", 76)
	s.checkView(t, "liquidityOf", 75)
	s.checkBalance(t, s.project.ScriptHash(), 849)
	// refund target is the vote-registered project, not whoever paid (BV-01)
	s.e.NewInvoker(s.vault, s.e.Validator).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		require.Equal(t, s.project.ScriptHash().BytesBE(), stack[0].Value().([]byte))
	}, "project", s.listed.BytesBE())
}

func TestBondRequiresRegistration(t *testing.T) {
	s := setup(t)
	s.e.NewInvoker(s.ftb, s.project).InvokeFail(t, "listing not open", "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), 100, s.listed.BytesBE())
	s.checkView(t, "state", 0)
}

func TestRegisterGating(t *testing.T) {
	s := setup(t)
	tok := s.listed.BytesBE()
	proj := s.project.ScriptHash().BytesBE()
	comp := s.comp.ScriptHash().BytesBE()
	in := s.e.NewInvoker(s.vault, s.stranger)
	// compensation == project faults before the vote is even consulted
	in.InvokeFail(t, "compensation must not be the project", "registerListing", 1, tok, proj, proj)
	// valid params but no passed proposal
	in.InvokeFail(t, "proposal has not passed", "registerListing", 999, tok, proj, comp)
	// a passed proposal for (token, project, comp) cannot be redirected to a different sink
	sub := s.subjectOf(t, "subjectForRegisterListing", tok, proj, comp)
	id := s.govPass(t, sub)
	in.InvokeFail(t, "subject does not match", "registerListing", id, tok, proj, s.stranger.ScriptHash().BytesBE())
	// the matching execution registers it; the sink is public
	in.Invoke(t, stackitem.Null{}, "registerListing", id, tok, proj, comp)
	s.checkView(t, "state", 1)
	s.e.NewInvoker(s.vault, s.e.Validator).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		require.Equal(t, comp, stack[0].Value().([]byte))
	}, "compensationOf", tok)
	// re-registering (fresh passing proposal) hits the already-exists guard
	id2 := s.govPass(t, sub)
	in.InvokeFail(t, "listing already exists", "registerListing", id2, tok, proj, comp)
}

func TestSlashDelistRequireVote(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 100)
	in := s.e.NewInvoker(s.vault, s.stranger)
	in.InvokeFail(t, "proposal has not passed", "slash", 999, s.listed.BytesBE())
	in.InvokeFail(t, "proposal has not passed", "delist", 999, s.listed.BytesBE())
	s.checkView(t, "state", 2)
}

func TestSlashThenDelist(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 100)
	s.slash(t)
	s.checkView(t, "state", 3)
	s.checkView(t, "vaultOf", 0)
	s.checkView(t, "liquidityOf", 50)
	s.checkBalance(t, s.comp.ScriptHash(), 50)
	s.delist(t)
	s.checkView(t, "state", 4)
	s.checkBalance(t, s.project.ScriptHash(), 950)
	s.checkBalance(t, s.vault, 0)
}

func TestCleanDelistReturnsEverything(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 200)
	s.delist(t)
	s.checkBalance(t, s.project.ScriptHash(), 1000)
	s.checkBalance(t, s.vault, 0)
}

func TestNoRebondAfterDelist(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 100)
	s.delist(t)
	s.e.NewInvoker(s.ftb, s.project).InvokeFail(t, "listing not open", "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), 10, s.listed.BytesBE())
	id := s.govPass(t, s.subjectOf(t, "subjectForRegisterListing", s.listed.BytesBE(),
		s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE()))
	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "listing already exists", "registerListing",
		id, s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
}

func TestRejectsForeignToken(t *testing.T) {
	s := setup(t)
	gasHash := s.e.NativeHash(t, nativenames.Gas)
	s.e.NewInvoker(gasHash, s.project).InvokeFail(t, "only FTB accepted", "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), 1_0000_0000, s.listed.BytesBE())
	s.checkView(t, "state", 0)
}

func TestGuards(t *testing.T) {
	s := setup(t)
	// a passed slash on an unbonded token reaches the state check
	id := s.govPass(t, s.subjectOf(t, "subjectForSlash", s.listed.BytesBE()))
	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "not a bonded listing", "slash", id, s.listed.BytesBE())
	id2 := s.govPass(t, s.subjectOf(t, "subjectForDelist", s.listed.BytesBE()))
	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "no live listing", "delist", id2, s.listed.BytesBE())
	s.e.NewInvoker(s.vault, s.owner).InvokeFail(t, "write-once", "configure", s.ftb.BytesBE(), s.gov.BytesBE())
	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "only owner", "configure", s.ftb.BytesBE(), s.gov.BytesBE())
}
