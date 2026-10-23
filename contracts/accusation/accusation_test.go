// neotest integration suite for the FTB Discord-bond dispute ballot (accusation) wired to
// the real bond vault (dbondvault). Exercises the full case life cycle — file, stake-weighted
// vote, resolve — and the four remediations that hardened it:
//
//	C1  a voter's own bond is locked from refund until the case deadline (no vote recycling)
//	M1  the full voting window must close before any resolution (no early liquidation)
//	M2  a dismissed bond enters a re-file cooldown (no lock-grief by immediate re-filing)
//	L5  a case cannot open when the snapshotted threshold rounds to zero
//
// plus the happy liquidate/dismiss paths (condemn burns the bond; dismiss unlocks it) and the
// file/vote access rules. The ballot drives dbondvault.condemn / lockForCase / lockVoter over
// a real contract.Call, so the cross-contract caller gate is exercised for real here.
//
// Run: go test ./contracts/accusation/
package accusation_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const dec = 100_000_000

const (
	bondAmt = 100 * dec
	banBps  = 5000 // 50%
	window  = 20   // voting window / cooldown, in blocks (kept small so tests advance cheaply)
)

type sys struct {
	e                       *neotest.Executor
	owner                   neotest.Signer
	gov                     neotest.Signer
	burn                    util.Uint160
	ftb, caller, vault, acc util.Uint160
}

func setup(t *testing.T) *sys { return setupWith(t, bondAmt, banBps, window) }

func setupWith(t *testing.T, bAmt, bBps, win int) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	gov := e.NewAccount(t)
	burn := e.NewAccount(t)
	s := &sys{e: e, owner: owner, gov: gov, burn: burn.ScriptHash()}

	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	s.ftb = tokC.Hash

	callC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/testdata/caller/caller.go", "../ftb_token/testdata/caller/caller.yml")
	e.DeployContract(t, callC, nil)
	s.caller = callC.Hash
	e.NewInvoker(s.ftb, owner).Invoke(t, stackitem.Null{}, "setMigrator", s.caller.BytesBE())

	vaultC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../dbondvault/dbondvault.go", "../dbondvault/dbondvault.yml")
	e.DeployContract(t, vaultC, owner.ScriptHash().BytesBE())
	s.vault = vaultC.Hash

	accC := neotest.CompileFile(t, e.Validator.ScriptHash(), "accusation.go", "accusation.yml")
	e.DeployContract(t, accC, owner.ScriptHash().BytesBE())
	s.acc = accC.Hash

	// configure the ballot with the vault, then the vault with the real ballot as its accusation
	e.NewInvoker(s.acc, owner).Invoke(t, stackitem.Null{}, "configure", s.vault.BytesBE())
	e.NewInvoker(s.vault, owner).Invoke(t, stackitem.Null{}, "configure",
		s.ftb.BytesBE(), s.acc.BytesBE(), gov.ScriptHash().BytesBE(), s.burn.BytesBE(),
		bAmt, bBps, win)
	return s
}

func (s *sys) mintFTB(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.caller, s.owner).Invoke(t, stackitem.Null{}, "mint", s.ftb.BytesBE(), to.BytesBE(), amt)
}

func (s *sys) bond(t *testing.T, member neotest.Signer, discordID string, amt int) {
	s.e.NewInvoker(s.ftb, member).Invoke(t, stackitem.Make(true), "transfer",
		member.ScriptHash().BytesBE(), s.vault.BytesBE(), amt, []byte(discordID))
}

func (s *sys) ftbBal(t *testing.T, who util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", who.BytesBE())
}

func (s *sys) ab(signer neotest.Signer) *neotest.ContractInvoker {
	return s.e.NewInvoker(s.acc, signer)
}
func (s *sys) vb(signer neotest.Signer) *neotest.ContractInvoker {
	return s.e.NewInvoker(s.vault, signer)
}

func evid() []byte { return make([]byte, 32) }

// scenario bonds an accused (bond id 1, Discord "ACC") and two voters (ids 2 and 3), each at
// the standard bond. With banBps 50% the snapshot threshold is 100 FTB — two "for" votes pass.
func (s *sys) scenario(t *testing.T) (accused, v1, v2 neotest.Signer) {
	accused = s.e.NewAccount(t)
	v1 = s.e.NewAccount(t)
	v2 = s.e.NewAccount(t)
	for _, m := range []neotest.Signer{accused, v1, v2} {
		s.mintFTB(t, m.ScriptHash(), bondAmt)
	}
	s.bond(t, accused, "ACC", bondAmt)
	s.bond(t, v1, "V1", bondAmt)
	s.bond(t, v2, "V2", bondAmt)
	return
}

// Full liquidate path: file, two "for" votes, resolve after the window -> the vault condemns
// the accused bond and the FTB lands in the burn sink. Also pins M1 (no early resolution).
func TestFileVoteLiquidate(t *testing.T) {
	s := setup(t)
	accused, v1, v2 := s.scenario(t)

	s.ab(v1).Invoke(t, stackitem.Make(1), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
	s.ab(v1).Invoke(t, stackitem.Null{}, "vote", v1.ScriptHash().BytesBE(), 1, true)
	s.ab(v2).Invoke(t, stackitem.Null{}, "vote", v2.ScriptHash().BytesBE(), 1, true)

	// M1: the full window must close before any resolution
	s.ab(v1).InvokeFail(t, "voting window still open", "resolve", 1)
	s.e.GenerateNewBlocks(t, window+1)
	s.e.NewInvoker(s.acc, s.owner).Invoke(t, stackitem.Null{}, "resolve", 1) // permissionless

	vo := s.vb(s.owner)
	vo.Invoke(t, stackitem.Make(false), "isBondedVoter", accused.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(0), "stakeOf", accused.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(2*bondAmt), "totalActiveBondedStake")
	s.ftbBal(t, s.burn, bondAmt)
}

// Split vote dismisses (for not strictly greater than against); the bond survives and enters a
// re-file cooldown (M2) that blocks an immediate second filing but clears after the window.
func TestFileVoteDismissThenCooldown(t *testing.T) {
	s := setup(t)
	accused, v1, v2 := s.scenario(t)

	s.ab(v1).Invoke(t, stackitem.Make(1), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
	s.ab(v1).Invoke(t, stackitem.Null{}, "vote", v1.ScriptHash().BytesBE(), 1, true)  // 100 for
	s.ab(v2).Invoke(t, stackitem.Null{}, "vote", v2.ScriptHash().BytesBE(), 1, false) // 100 against
	s.e.GenerateNewBlocks(t, window+1)
	s.e.NewInvoker(s.acc, s.owner).Invoke(t, stackitem.Null{}, "resolve", 1)

	vo := s.vb(s.owner)
	vo.Invoke(t, stackitem.Make(true), "isBondedVoter", accused.ScriptHash().BytesBE())
	vo.Invoke(t, stackitem.Make(3*bondAmt), "totalActiveBondedStake")
	s.ftbBal(t, s.burn, 0)

	// M2: immediate re-file is blocked by the post-dismissal cooldown; it clears after the window
	s.ab(v1).InvokeFail(t, "post-dismissal cooldown", "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
	s.e.GenerateNewBlocks(t, window+1)
	s.ab(v1).Invoke(t, stackitem.Make(2), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
}

// C1: casting a vote locks the voter's OWN bond from refund until the case deadline, so stake
// cannot be pulled and recycled into another wallet mid-case. The lock expires with the window.
func TestVoterRefundLockedC1(t *testing.T) {
	s := setup(t)
	_, v1, _ := s.scenario(t)

	s.ab(v1).Invoke(t, stackitem.Make(1), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
	s.ab(v1).Invoke(t, stackitem.Null{}, "vote", v1.ScriptHash().BytesBE(), 1, true)

	// v1's own bond is id 2; no case targets it, yet the vote lock blocks the refund
	s.vb(v1).InvokeFail(t, "an open vote is pending", "refund", 2)
	s.e.GenerateNewBlocks(t, window+1)
	s.vb(v1).Invoke(t, stackitem.Null{}, "refund", 2)
}

// The accused bond is guarded from refund for as long as a case is open; a dismissal unlocks it.
func TestRefundBlockedWhileCaseOpen(t *testing.T) {
	s := setup(t)
	accused, v1, _ := s.scenario(t)

	s.ab(v1).Invoke(t, stackitem.Make(1), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
	s.vb(accused).InvokeFail(t, "an accusation case is open", "refund", 1)

	// no votes -> dismiss on resolution -> unlock; the owner can then refund
	s.e.GenerateNewBlocks(t, window+1)
	s.e.NewInvoker(s.acc, s.owner).Invoke(t, stackitem.Null{}, "resolve", 1)
	s.vb(accused).Invoke(t, stackitem.Null{}, "refund", 1)
}

// Filing rules: filer must be bonded, evidence must be 32 bytes, no self-accusation, and one
// open case per bond.
func TestFileRules(t *testing.T) {
	s := setup(t)
	accused, v1, _ := s.scenario(t)
	outsider := s.e.NewAccount(t)

	s.ab(outsider).InvokeFail(t, "only a bonded member may file", "fileCase", outsider.ScriptHash().BytesBE(), 1, 7, evid())
	s.ab(v1).InvokeFail(t, "evidenceHash must be 32 bytes", "fileCase", v1.ScriptHash().BytesBE(), 1, 7, make([]byte, 31))
	s.ab(accused).InvokeFail(t, "cannot file against your own bond", "fileCase", accused.ScriptHash().BytesBE(), 1, 7, evid())
	s.ab(v1).Invoke(t, stackitem.Make(1), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
	s.ab(v1).InvokeFail(t, "a case is already open for this bond", "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())
}

// Voting rules: the accused may not vote, non-bonded wallets have no weight, one vote per
// member, and no votes after the window closes.
func TestVoteRules(t *testing.T) {
	s := setup(t)
	accused, v1, v2 := s.scenario(t)
	outsider := s.e.NewAccount(t)
	s.ab(v1).Invoke(t, stackitem.Make(1), "fileCase", v1.ScriptHash().BytesBE(), 1, 7, evid())

	s.ab(accused).InvokeFail(t, "the accused may not vote on their own case", "vote", accused.ScriptHash().BytesBE(), 1, true)
	s.ab(outsider).InvokeFail(t, "only a bonded member may vote", "vote", outsider.ScriptHash().BytesBE(), 1, true)
	s.ab(v1).Invoke(t, stackitem.Null{}, "vote", v1.ScriptHash().BytesBE(), 1, true)
	s.ab(v1).InvokeFail(t, "already voted", "vote", v1.ScriptHash().BytesBE(), 1, true)

	s.e.GenerateNewBlocks(t, window+1)
	s.ab(v2).InvokeFail(t, "voting window has closed", "vote", v2.ScriptHash().BytesBE(), 1, true)
}

// L5: a case cannot open when the snapshotted threshold rounds down to zero (a tiny-stake /
// tiny-banBps misconfiguration), which would otherwise allow a trivially-passing ballot.
func TestThresholdGuardL5(t *testing.T) {
	s := setupWith(t, 1, 1, window) // bondAmount 1 base unit, banBps 1 -> threshold = 1*1/10000 = 0
	accused := s.e.NewAccount(t)
	filer := s.e.NewAccount(t)
	s.mintFTB(t, accused.ScriptHash(), 1)
	s.mintFTB(t, filer.ScriptHash(), 1)
	s.bond(t, accused, "ACC", 1)
	s.bond(t, filer, "FIL", 1)
	s.ab(filer).InvokeFail(t, "insufficient bonded stake", "fileCase", filer.ScriptHash().BytesBE(), 1, 7, evid())
}
