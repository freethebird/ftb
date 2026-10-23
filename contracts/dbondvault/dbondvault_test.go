// neotest suite for the FTB Discord member-bond vault (dbondvault). Wires the FTB token,
// its mint-caller and the vault, then exercises bond posting, the one-bond-per-wallet and
// per-Discord-ID rules, owner refund, the governance-only SetParams, write-once Configure
// (including the 20-byte-hash validation, M3/L2), and the access control on the
// accusation-only entry points (condemn / lockForCase / unlockForCase / lockVoter).
//
// The accusation-gated *positive* paths (condemn burns, the refund guard, the C1 vote lock)
// are driven end-to-end through the real ballot in accusation_test.go; here the accusation
// is a placeholder address, so only the negative (rejection) side of those gates is asserted.
//
// Run: go test ./contracts/dbondvault/
package dbondvault_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const dec = 100_000_000

const (
	bondAmt = 100 * dec
	banBps  = 5000 // 50%
	window  = 100  // voting window / cooldown, in blocks
)

type sys struct {
	e                  *neotest.Executor
	owner              neotest.Signer
	gov                neotest.Signer
	acc                neotest.Signer // placeholder "accusation" address (negative gate tests only)
	burn               util.Uint160
	ftb, caller, vault util.Uint160
	gas                util.Uint160
}

func setup(t *testing.T) *sys {
	s := deploy(t)
	s.e.NewInvoker(s.vault, s.owner).Invoke(t, stackitem.Null{}, "configure",
		s.ftb.BytesBE(), s.acc.ScriptHash().BytesBE(), s.gov.ScriptHash().BytesBE(), s.burn.BytesBE(),
		bondAmt, banBps, window)
	return s
}

// deploy wires token + mint-caller + vault on a fresh chain but leaves the vault UNCONFIGURED,
// so Configure and its validation can be exercised directly.
func deploy(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	gov := e.NewAccount(t)
	acc := e.NewAccount(t)
	burn := e.NewAccount(t)
	s := &sys{e: e, owner: owner, gov: gov, acc: acc, burn: burn.ScriptHash(), gas: e.NativeHash(t, nativenames.Gas)}

	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	s.ftb = tokC.Hash

	callC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/testdata/caller/caller.go", "../ftb_token/testdata/caller/caller.yml")
	e.DeployContract(t, callC, nil)
	s.caller = callC.Hash
	e.NewInvoker(s.ftb, owner).Invoke(t, stackitem.Null{}, "setMigrator", s.caller.BytesBE())

	vaultC := neotest.CompileFile(t, e.Validator.ScriptHash(), "dbondvault.go", "dbondvault.yml")
	e.DeployContract(t, vaultC, owner.ScriptHash().BytesBE())
	s.vault = vaultC.Hash
	return s
}

func (s *sys) mintFTB(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.caller, s.owner).Invoke(t, stackitem.Null{}, "mint", s.ftb.BytesBE(), to.BytesBE(), amt)
}

// bond posts a bond by transferring exactly bondAmt FTB with the Discord ID as transfer data.
func (s *sys) bond(t *testing.T, member neotest.Signer, discordID string, amt int) {
	s.e.NewInvoker(s.ftb, member).Invoke(t, stackitem.Make(true), "transfer",
		member.ScriptHash().BytesBE(), s.vault.BytesBE(), amt, []byte(discordID))
}

func (s *sys) ftbBal(t *testing.T, who util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", who.BytesBE())
}

func (s *sys) vw(signer neotest.Signer) *neotest.ContractInvoker {
	return s.e.NewInvoker(s.vault, signer)
}

// A clean bond posts, reads back through the views, and refunds to its owner.
func TestBondAndRefund(t *testing.T) {
	s := setup(t)
	m := s.e.NewAccount(t)
	s.mintFTB(t, m.ScriptHash(), bondAmt)
	s.bond(t, m, "1001", bondAmt)

	vo := s.vw(s.owner)
	vo.Invoke(t, stackitem.Make(true), "isBondedVoter", m.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(bondAmt), "stakeOf", m.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(bondAmt), "totalActiveBondedStake")
	vo.Invoke(t, stackitem.Make(1), "getBondByDiscord", []byte("1001"))

	// only the owner may refund, and the bond amount returns to them
	s.vw(m).Invoke(t, stackitem.Null{}, "refund", 1)
	s.ftbBal(t, m.ScriptHash(), bondAmt)
	vo.Invoke(t, stackitem.Make(false), "isBondedVoter", m.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(0), "stakeOf", m.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(0), "totalActiveBondedStake")
	vo.Invoke(t, stackitem.Make(0), "getBondByDiscord", []byte("1001"))
}

// The bond must be EXACTLY the current bondAmount; any other amount is rejected and the
// transfer reverts (no partial or over-sized bond is ever recorded).
func TestBondRejectsWrongAmount(t *testing.T) {
	s := setup(t)
	m := s.e.NewAccount(t)
	s.mintFTB(t, m.ScriptHash(), bondAmt)
	s.e.NewInvoker(s.ftb, m).InvokeFail(t, "exactly bondAmount", "transfer",
		m.ScriptHash().BytesBE(), s.vault.BytesBE(), bondAmt/2, []byte("1001"))
}

// Only FTB is accepted; a GAS transfer into the vault reverts (the token check fires on the
// calling hash before any amount check, so a small, affordable GAS amount is enough to prove it).
func TestBondRejectsWrongToken(t *testing.T) {
	s := setup(t)
	m := s.e.NewAccount(t)
	s.e.NewInvoker(s.gas, m).InvokeFail(t, "only FTB is accepted", "transfer",
		m.ScriptHash().BytesBE(), s.vault.BytesBE(), 1*dec, []byte("1001"))
}

// The Discord ID is mandatory: an empty data payload is rejected.
func TestBondRejectsMissingDiscordID(t *testing.T) {
	s := setup(t)
	m := s.e.NewAccount(t)
	s.mintFTB(t, m.ScriptHash(), bondAmt)
	s.e.NewInvoker(s.ftb, m).InvokeFail(t, "missing discord id", "transfer",
		m.ScriptHash().BytesBE(), s.vault.BytesBE(), bondAmt, []byte{})
}

// One active bond per wallet: a second bond from the same wallet (different Discord ID) is rejected.
func TestOneBondPerWallet(t *testing.T) {
	s := setup(t)
	m := s.e.NewAccount(t)
	s.mintFTB(t, m.ScriptHash(), 2*bondAmt)
	s.bond(t, m, "1001", bondAmt)
	s.e.NewInvoker(s.ftb, m).InvokeFail(t, "wallet already holds an active bond", "transfer",
		m.ScriptHash().BytesBE(), s.vault.BytesBE(), bondAmt, []byte("1002"))
}

// One active bond per Discord ID: a second wallet cannot bond the same Discord ID.
func TestOneBondPerDiscordID(t *testing.T) {
	s := setup(t)
	m1 := s.e.NewAccount(t)
	m2 := s.e.NewAccount(t)
	s.mintFTB(t, m1.ScriptHash(), bondAmt)
	s.mintFTB(t, m2.ScriptHash(), bondAmt)
	s.bond(t, m1, "1001", bondAmt)
	s.e.NewInvoker(s.ftb, m2).InvokeFail(t, "member already holds an active bond", "transfer",
		m2.ScriptHash().BytesBE(), s.vault.BytesBE(), bondAmt, []byte("1001"))
}

// Refund is owner-only and one-shot: a stranger cannot refund, and a refunded bond cannot refund again.
func TestRefundAccessAndIdempotence(t *testing.T) {
	s := setup(t)
	m := s.e.NewAccount(t)
	stranger := s.e.NewAccount(t)
	s.mintFTB(t, m.ScriptHash(), bondAmt)
	s.bond(t, m, "1001", bondAmt)

	s.vw(stranger).InvokeFail(t, "only the bond owner may refund", "refund", 1)
	s.vw(m).Invoke(t, stackitem.Null{}, "refund", 1)
	s.vw(m).InvokeFail(t, "bond is not active", "refund", 1)
}

// SetParams is governance-only and validates its inputs; existing bonds keep their amount.
func TestSetParamsGovernanceOnly(t *testing.T) {
	s := setup(t)
	stranger := s.e.NewAccount(t)
	s.vw(stranger).InvokeFail(t, "only governance may set params", "setParams", 200*dec, 6000, 200)

	gv := s.vw(s.gov)
	gv.Invoke(t, stackitem.Null{}, "setParams", 200*dec, 6000, 200)
	vo := s.vw(s.owner)
	vo.Invoke(t, stackitem.Make(200*dec), "bondAmount")
	vo.Invoke(t, stackitem.Make(6000), "banBps")
	vo.Invoke(t, stackitem.Make(200), "votingWindow")

	gv.InvokeFail(t, "bondAmount must be positive", "setParams", 0, 6000, 200)
	gv.InvokeFail(t, "banBps must be in 1..10000", "setParams", 200*dec, 0, 200)
	gv.InvokeFail(t, "banBps must be in 1..10000", "setParams", 200*dec, 10001, 200)
	gv.InvokeFail(t, "window must be positive", "setParams", 200*dec, 6000, 0)
}

// Configure is write-once and validates every wired hash is 20 bytes (M3/L2).
func TestConfigureValidation(t *testing.T) {
	s := deploy(t) // unconfigured vault
	v := s.vw(s.owner)
	// a short (non-20-byte) hash is rejected
	v.InvokeFail(t, "must be 20-byte hashes", "configure",
		s.ftb.BytesBE(), []byte{}, s.gov.ScriptHash().BytesBE(), s.burn.BytesBE(),
		bondAmt, banBps, window)
	// a valid Configure succeeds, and a second one is rejected (write-once)
	v.Invoke(t, stackitem.Null{}, "configure",
		s.ftb.BytesBE(), s.acc.ScriptHash().BytesBE(), s.gov.ScriptHash().BytesBE(), s.burn.BytesBE(),
		bondAmt, banBps, window)
	v.InvokeFail(t, "already configured", "configure",
		s.ftb.BytesBE(), s.acc.ScriptHash().BytesBE(), s.gov.ScriptHash().BytesBE(), s.burn.BytesBE(),
		bondAmt, banBps, window)
}

// The accusation-only entry points reject any caller that is not the configured ballot. Here
// the configured accusation is a placeholder address, so a direct call from any account fails.
func TestAccusationGateRejectsOutsiders(t *testing.T) {
	s := setup(t)
	caller := s.vw(s.owner)
	caller.InvokeFail(t, "only the accusation ballot", "condemn", 1)
	caller.InvokeFail(t, "only the accusation ballot", "lockForCase", 1)
	caller.InvokeFail(t, "only the accusation ballot", "unlockForCase", 1)
	caller.InvokeFail(t, "only the accusation ballot", "lockVoter", s.owner.ScriptHash().BytesBE(), 100)
}
