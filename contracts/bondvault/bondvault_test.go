// Neotest suite for the bond vault. A mock NEP-17 stands in for FTB (the vault only ever
// calls transfer/balanceOf on it); a single account stands in for the operator committee
// multisig. Covers committee registration (the BV-01/BV-02 remediation — refund target and
// slash sink bound up-front), the 50/50 split at the door, top-ups, foreign-token rejection,
// the no-bond-without-registration rule, committee-only execution, slash-to-the-registered-
// sink, slash-then-delist, clean delist, and the no-rebond rule.
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

type sys struct {
	e                                         *neotest.Executor
	owner, committee, project, comp, stranger neotest.Signer
	ftb, vault, listed                        util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.committee = e.NewAccount(t)
	s.project = e.NewAccount(t)
	s.comp = e.NewAccount(t) // compensation sink for harmed LPs (registered up-front)
	s.stranger = e.NewAccount(t)
	s.listed = e.NewAccount(t).ScriptHash() // identifier for the listed token

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	s.ftb = ftbC.Hash

	vC := neotest.CompileFile(t, e.Validator.ScriptHash(), "bondvault.go", "bondvault.yml")
	e.DeployContract(t, vC, s.owner.ScriptHash().BytesBE())
	s.vault = vC.Hash
	e.NewInvoker(s.vault, s.owner).Invoke(t, stackitem.Null{},
		"configure", s.ftb.BytesBE(), s.committee.ScriptHash().BytesBE())

	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{},
		"mint", s.project.ScriptHash().BytesBE(), 1000)
	return s
}

// register authorizes the default listed token: project is the refund target, comp is the
// slash sink (the committee acting on a passed listing ballot).
func (s *sys) register(t *testing.T) {
	s.e.NewInvoker(s.vault, s.committee).Invoke(t, stackitem.Null{}, "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
}

// bond moves `amount` mock-FTB from the project into the vault for the listed token.
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

	s.bond(t, 101) // odd amount: vault keeps the rounding dust
	s.checkView(t, "state", 2) // bonded
	s.checkView(t, "vaultOf", 51)
	s.checkView(t, "liquidityOf", 50)
	s.checkBalance(t, s.vault, 101)

	s.bond(t, 50) // top-up adds halves on top
	s.checkView(t, "vaultOf", 76)
	s.checkView(t, "liquidityOf", 75)
	s.checkBalance(t, s.vault, 151)
	s.checkBalance(t, s.project.ScriptHash(), 849)

	// the refund target is the committee-registered project, NOT whoever paid (BV-01)
	s.e.NewInvoker(s.vault, s.e.Validator).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		require.Equal(t, s.project.ScriptHash().BytesBE(), stack[0].Value().([]byte))
	}, "project", s.listed.BytesBE())
}

// BV-01: a permissionless first bond can no longer open a listing or set its refund target.
func TestBondRequiresRegistration(t *testing.T) {
	s := setup(t)
	s.e.NewInvoker(s.ftb, s.project).InvokeFail(t, "listing not open", "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), 100, s.listed.BytesBE())
	s.checkView(t, "state", 0) // still none; the front-run has nothing to capture
}

// BV-01/BV-02: registration is committee-only, one-time, and the slash sink may be neither
// the committee (no self-drain) nor the project (a slash must not refund the rugger).
func TestRegisterListingGating(t *testing.T) {
	s := setup(t)
	vcom := s.e.NewInvoker(s.vault, s.committee)

	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "only committee", "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
	vcom.InvokeFail(t, "compensation must not be the committee", "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.committee.ScriptHash().BytesBE())
	vcom.InvokeFail(t, "compensation must not be the project", "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.project.ScriptHash().BytesBE())

	vcom.Invoke(t, stackitem.Null{}, "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
	s.checkView(t, "state", 1) // registered
	// the registered sink is public from registration on
	s.e.NewInvoker(s.vault, s.e.Validator).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		require.Equal(t, s.comp.ScriptHash().BytesBE(), stack[0].Value().([]byte))
	}, "compensationOf", s.listed.BytesBE())
	vcom.InvokeFail(t, "listing already exists", "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
}

func TestRejectsForeignToken(t *testing.T) {
	s := setup(t)

	// native GAS is a well-behaved NEP-17 that is not FTB: the vault must refuse it
	gasHash := s.e.NativeHash(t, nativenames.Gas)
	s.e.NewInvoker(gasHash, s.project).InvokeFail(t, "only FTB accepted", "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), 1_0000_0000, s.listed.BytesBE())
	s.checkView(t, "state", 0)
}

func TestOnlyCommitteeExecutes(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 100)

	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "only committee", "slash", s.listed.BytesBE())
	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "only committee", "delist", s.listed.BytesBE())
	s.checkView(t, "state", 2) // untouched (bonded)
}

func TestSlashThenDelist(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 100) // vault 50, liquidity 50

	// slash takes only the token: the destination is the pre-registered sink (BV-02)
	s.e.NewInvoker(s.vault, s.committee).Invoke(t, stackitem.Null{}, "slash", s.listed.BytesBE())
	s.checkView(t, "state", 3)
	s.checkView(t, "vaultOf", 0)
	s.checkView(t, "liquidityOf", 50)      // liquidity half stays locked
	s.checkBalance(t, s.comp.ScriptHash(), 50) // paid to the registered sink, not an arbitrary addr

	// no double slash
	s.e.NewInvoker(s.vault, s.committee).InvokeFail(t, "not a bonded listing", "slash", s.listed.BytesBE())

	// delisting after a slash returns only the liquidity half to the project
	s.e.NewInvoker(s.vault, s.committee).Invoke(t, stackitem.Null{}, "delist", s.listed.BytesBE())
	s.checkView(t, "state", 4)
	s.checkBalance(t, s.project.ScriptHash(), 950)
	s.checkBalance(t, s.vault, 0)
}

func TestCleanDelistReturnsEverything(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 200)

	s.e.NewInvoker(s.vault, s.committee).Invoke(t, stackitem.Null{}, "delist", s.listed.BytesBE())
	s.checkView(t, "state", 4)
	s.checkBalance(t, s.project.ScriptHash(), 1000)
	s.checkBalance(t, s.vault, 0)
}

func TestNoRebondAfterDelist(t *testing.T) {
	s := setup(t)
	s.register(t)
	s.bond(t, 100)
	s.e.NewInvoker(s.vault, s.committee).Invoke(t, stackitem.Null{}, "delist", s.listed.BytesBE())

	// the token record is terminal: neither a new bond nor a re-registration reopens it
	s.e.NewInvoker(s.ftb, s.project).InvokeFail(t, "listing not open", "transfer",
		s.project.ScriptHash().BytesBE(), s.vault.BytesBE(), 10, s.listed.BytesBE())
	s.e.NewInvoker(s.vault, s.committee).InvokeFail(t, "listing already exists", "registerListing",
		s.listed.BytesBE(), s.project.ScriptHash().BytesBE(), s.comp.ScriptHash().BytesBE())
}

func TestGuards(t *testing.T) {
	s := setup(t)

	// slash with no bonded listing
	s.e.NewInvoker(s.vault, s.committee).InvokeFail(t, "not a bonded listing", "slash", s.listed.BytesBE())
	// delist with no listing
	s.e.NewInvoker(s.vault, s.committee).InvokeFail(t, "no live listing", "delist", s.listed.BytesBE())
	// configure is write-once
	s.e.NewInvoker(s.vault, s.owner).InvokeFail(t, "write-once", "configure",
		s.ftb.BytesBE(), s.committee.ScriptHash().BytesBE())
	// configure is owner-only (checked before the write-once guard)
	s.e.NewInvoker(s.vault, s.stranger).InvokeFail(t, "only owner", "configure",
		s.ftb.BytesBE(), s.committee.ScriptHash().BytesBE())
}
