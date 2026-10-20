// Neotest suite for the treasury. A mock NEP-17 stands in for FTB (the treasury only ever
// calls transfer/balanceOf on it); single accounts stand in for the operator committee
// multisig. Covers the full escrow lifecycle (create -> attest -> tranche A -> soak ->
// tranche B), the hardened separation of duties (T1: dev/auditor independence, auditor ∉
// {committee}, registered-auditor set, non-zero soak floor), the bound-hash attestation
// (T2), audit failure and cancellation returning funds, foreign-token rejection, and the
// fail-closed excess return to the dividends module.
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

type sys struct {
	e                                     *neotest.Executor
	owner, committee, dev, auditor, other neotest.Signer
	ftb, treasury, reg                    util.Uint160
}

// soak is the per-escrow testnet soak used in tests; it must meet the contract's minSoak
// floor (5760, the T1 remediation that forbids a zero-soak same-block drain).
const soak = 5760

var testNefHash = bytes.Repeat([]byte{0xab}, 32)

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.committee = e.NewAccount(t)
	s.dev = e.NewAccount(t)
	s.auditor = e.NewAccount(t)
	s.other = e.NewAccount(t)

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	s.ftb = ftbC.Hash

	regC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../registry/registry.go", "../registry/registry.yml")
	e.DeployContract(t, regC, s.owner.ScriptHash().BytesBE())
	s.reg = regC.Hash
	e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "setInitialDelay", 1)

	trC := neotest.CompileFile(t, e.Validator.ScriptHash(), "treasury.go", "treasury.yml")
	e.DeployContract(t, trC, s.owner.ScriptHash().BytesBE())
	s.treasury = trC.Hash
	e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Null{},
		"configure", s.ftb.BytesBE(), s.committee.ScriptHash().BytesBE(), s.reg.BytesBE())

	// T1: the auditor must be enrolled on the registered set before it can be named
	e.NewInvoker(s.treasury, s.committee).Invoke(t, stackitem.Null{},
		"setAuditor", s.auditor.ScriptHash().BytesBE(), true)

	// the treasury starts with income: 1000 mock-FTB
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", s.owner.ScriptHash().BytesBE(), 1000)
	e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(true), "transfer",
		s.owner.ScriptHash().BytesBE(), s.treasury.BytesBE(), 1000, nil)
	return s
}

func (s *sys) bal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}

// create opens an escrow binding testNefHash as the expected audited hash (T2).
func (s *sys) create(t *testing.T, id, amount, bps, soakBlocks int) {
	s.e.NewInvoker(s.treasury, s.committee).Invoke(t, stackitem.Null{}, "createEscrow", id,
		s.dev.ScriptHash().BytesBE(), s.auditor.ScriptHash().BytesBE(), amount, bps, soakBlocks, testNefHash)
}

func TestLifecycle(t *testing.T) {
	s := setup(t)
	tr := s.treasury

	s.create(t, 7, 600, 3000, soak) // 600 escrowed, 30% tranche A, floor soak
	s.bal(t, tr, 1000)              // nothing left the contract
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(400), "free")
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(600), "escrowed")
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(1), "state", 7)

	devInv := s.e.NewInvoker(tr, s.dev)
	devInv.InvokeFail(t, "audit not attested", "claimA", 7)

	// only the escrow's auditor may attest, only to a 32-byte hash, and only the BOUND hash
	s.e.NewInvoker(tr, s.other).InvokeFail(t, "only the escrow auditor", "attest", 7, testNefHash)
	s.e.NewInvoker(tr, s.auditor).InvokeFail(t, "32 bytes", "attest", 7, []byte{0x01})
	s.e.NewInvoker(tr, s.auditor).InvokeFail(t, "does not match the escrow's bound hash",
		"attest", 7, bytes.Repeat([]byte{0xcd}, 32))
	s.e.NewInvoker(tr, s.auditor).Invoke(t, stackitem.Null{}, "attest", 7, testNefHash)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(2), "state", 7)
	s.e.NewInvoker(tr, s.owner).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		if !bytes.Equal(stack[0].Value().([]byte), testNefHash) {
			t.Fatalf("audited hash mismatch")
		}
	}, "auditedHash", 7)

	// tranche A is self-service by the dev team, exactly 30% of 600
	s.e.NewInvoker(tr, s.other).InvokeFail(t, "only the dev team", "claimA", 7)
	devInv.Invoke(t, stackitem.Null{}, "claimA", 7)
	s.bal(t, s.dev.ScriptHash(), 180)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(3), "state", 7)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(420), "amountOf", 7)

	// tranche B waits out the soak, then pays the remainder
	devInv.InvokeFail(t, "soak period still running", "claimB", 7)
	s.e.GenerateNewBlocks(t, soak+2)
	devInv.Invoke(t, stackitem.Null{}, "claimB", 7)
	s.bal(t, s.dev.ScriptHash(), 600)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(4), "state", 7)
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(0), "escrowed")
	s.e.NewInvoker(tr, s.owner).Invoke(t, stackitem.Make(400), "free")

	// nothing left to claim, ever
	devInv.InvokeFail(t, "tranche A not claimed", "claimB", 7)
}

func TestCreateEscrowValidation(t *testing.T) {
	s := setup(t)
	trC := s.e.NewInvoker(s.treasury, s.committee)
	dev := s.dev.ScriptHash().BytesBE()
	aud := s.auditor.ScriptHash().BytesBE()
	com := s.committee.ScriptHash().BytesBE()

	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only committee", "createEscrow", 1,
		dev, aud, 100, 3000, soak, testNefHash)
	// dev == auditor
	trC.InvokeFail(t, "bad dev/auditor", "createEscrow", 1, dev, dev, 100, 3000, soak, testNefHash)
	// T1: dev or auditor equal to the committee
	trC.InvokeFail(t, "must not be the committee", "createEscrow", 1, com, aud, 100, 3000, soak, testNefHash)
	trC.InvokeFail(t, "must not be the committee", "createEscrow", 1, dev, com, 100, 3000, soak, testNefHash)
	// T1: auditor not on the registered set (s.other is a valid, non-committee, un-enrolled addr)
	trC.InvokeFail(t, "auditor not on the registered set", "createEscrow", 1,
		dev, s.other.ScriptHash().BytesBE(), 100, 3000, soak, testNefHash)
	// bad bps
	trC.InvokeFail(t, "bad terms", "createEscrow", 1, dev, aud, 100, 0, soak, testNefHash)
	trC.InvokeFail(t, "bad terms", "createEscrow", 1, dev, aud, 100, 10000, soak, testNefHash)
	// T1: soak below the floor
	trC.InvokeFail(t, "soak below the minimum floor", "createEscrow", 1, dev, aud, 100, 3000, soak-1, testNefHash)
	// T2: expected hash must be 32 bytes
	trC.InvokeFail(t, "expected NEF hash must be 32 bytes", "createEscrow", 1, dev, aud, 100, 3000, soak, []byte{0x01})
	// bad id (negative, and the 4-byte escKey ceiling)
	trC.InvokeFail(t, "bad id", "createEscrow", -1, dev, aud, 100, 3000, soak, testNefHash)
	trC.InvokeFail(t, "bad id", "createEscrow", 1<<32, dev, aud, 100, 3000, soak, testNefHash)
	// the same bound is fail-closed on the view path
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "id out of range", "state", 1<<32)

	s.create(t, 1, 100, 3000, soak)
	trC.InvokeFail(t, "id already used", "createEscrow", 1, dev, aud, 100, 3000, soak, testNefHash)
	// the escrowed amount is not available to a second proposal (100 locked -> 900 free)
	trC.InvokeFail(t, "over free balance", "createEscrow", 2, dev, aud, 901, 3000, soak, testNefHash)
}

// T1: the committee cannot stand up a puppet auditor inside the funding call — enrollment is
// a separate, evented act, and a disenrolled auditor can no longer be named.
func TestAuditorSetGating(t *testing.T) {
	s := setup(t)
	trCom := s.e.NewInvoker(s.treasury, s.committee)

	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only committee", "setAuditor",
		s.other.ScriptHash().BytesBE(), true)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(true), "auditorEnabled",
		s.auditor.ScriptHash().BytesBE())
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(false), "auditorEnabled",
		s.other.ScriptHash().BytesBE())

	// disenroll the auditor -> it can no longer be named on a new escrow
	trCom.Invoke(t, stackitem.Null{}, "setAuditor", s.auditor.ScriptHash().BytesBE(), false)
	trCom.InvokeFail(t, "auditor not on the registered set", "createEscrow", 1,
		s.dev.ScriptHash().BytesBE(), s.auditor.ScriptHash().BytesBE(), 100, 3000, soak, testNefHash)
}

func TestCancelReturnsBudget(t *testing.T) {
	s := setup(t)
	s.create(t, 3, 500, 3000, soak)

	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only committee", "cancel", 3)
	s.e.NewInvoker(s.treasury, s.committee).Invoke(t, stackitem.Null{}, "cancel", 3)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(5), "state", 3)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(1000), "free")
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(0), "escrowed")

	// a cancelled escrow is terminal: no attestation, no claims
	s.e.NewInvoker(s.treasury, s.auditor).InvokeFail(t, "not an open escrow", "attest", 3, testNefHash)
	s.e.NewInvoker(s.treasury, s.dev).InvokeFail(t, "audit not attested", "claimA", 3)

	// and once an audit is attested there is no clawback of reviewed work
	s.create(t, 4, 100, 3000, soak)
	s.e.NewInvoker(s.treasury, s.auditor).Invoke(t, stackitem.Null{}, "attest", 4, testNefHash)
	s.e.NewInvoker(s.treasury, s.committee).InvokeFail(t, "not an open escrow", "cancel", 4)
}

func TestFailAuditReturnsBudget(t *testing.T) {
	s := setup(t)
	s.create(t, 5, 500, 3000, soak)

	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only the escrow auditor", "failAudit", 5)
	s.e.NewInvoker(s.treasury, s.committee).InvokeFail(t, "only the escrow auditor", "failAudit", 5)
	s.e.NewInvoker(s.treasury, s.auditor).Invoke(t, stackitem.Null{}, "failAudit", 5)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(6), "state", 5)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(1000), "free")
	s.e.NewInvoker(s.treasury, s.dev).InvokeFail(t, "audit not attested", "claimA", 5)
}

func TestRejectsForeignToken(t *testing.T) {
	s := setup(t)
	gas := s.e.NativeHash(t, nativenames.Gas)
	s.e.NewInvoker(gas, s.owner).InvokeFail(t, "only FTB accepted", "transfer",
		s.owner.ScriptHash().BytesBE(), s.treasury.BytesBE(), 1, nil)
}

func TestReturnExcess(t *testing.T) {
	s := setup(t)
	trC := s.e.NewInvoker(s.treasury, s.committee)
	dividends := s.e.NewAccount(t)

	// fail-closed while the dividends module is unbound
	trC.InvokeFail(t, "dividends module not bound", "returnExcess", 100)

	s.e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "bootstrap", "dividends",
		dividends.ScriptHash().BytesBE())
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only committee", "returnExcess", 100)
	trC.InvokeFail(t, "amount must be positive", "returnExcess", 0)
	trC.InvokeFail(t, "over free balance", "returnExcess", 1001)

	// escrowed funds are not excess: with 600 locked, only 400 can leave
	s.create(t, 9, 600, 3000, soak)
	trC.InvokeFail(t, "over free balance", "returnExcess", 401)
	trC.Invoke(t, stackitem.Null{}, "returnExcess", 400)
	s.bal(t, dividends.ScriptHash(), 400)
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(0), "free")
	s.e.NewInvoker(s.treasury, s.owner).Invoke(t, stackitem.Make(600), "escrowed")
}

func TestConfigureWriteOnce(t *testing.T) {
	s := setup(t)
	s.e.NewInvoker(s.treasury, s.other).InvokeFail(t, "only owner", "configure",
		s.ftb.BytesBE(), s.committee.ScriptHash().BytesBE(), s.reg.BytesBE())
	s.e.NewInvoker(s.treasury, s.owner).InvokeFail(t, "write-once", "configure",
		s.ftb.BytesBE(), s.committee.ScriptHash().BytesBE(), s.reg.BytesBE())
}
