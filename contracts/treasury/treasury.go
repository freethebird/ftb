// Package treasury is the FTB development treasury (ECONOMICS.md): a contract, not a
// wallet. It holds the treasury slice of protocol revenue — 1/9 of interest and swap fees,
// carved from the holders' third, LPs untouched — and pays it out only through the
// governance pipeline:
//
//  1. a proposal passes the 2/3 ballot with a budget, a dev team, and an auditor;
//  2. the operator committee (the same trusted-window multisig as registry/oracle) escrows
//     the budget to that proposal — funds never leave the contract until earned;
//  3. the named auditor posts an on-chain attestation binding the EXACT audited NEF hash
//     (the bytes recorded in NEF_HASHES.sha256);
//  4. the dev team self-serves tranche A after attestation and tranche B after the
//     testnet soak elapses — no committee push, no keys held by a person.
//
// A failed audit or a cancelled escrow returns the budget to the free balance. Excess
// accrual can only be returned to the registry-bound dividends module — fail-closed, never
// to an arbitrary address.
//
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package treasury

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

// escrow states
const (
	stateNone      = 0
	stateOpen      = 1 // funded, awaiting audit
	stateAttested  = 2 // audit passed, tranche A claimable, soak clock running
	statePaidA     = 3 // tranche A claimed, awaiting soak
	stateDone      = 4 // both tranches paid
	stateCancelled = 5 // committee cancelled before attestation; budget returned
	stateFailed    = 6 // audit failed; budget returned
)

var (
	keyOwner     = []byte{0x01} // deploy owner; may Configure once
	keyFTB       = []byte{0x02} // the FTB token hash
	keyCommittee = []byte{0x03} // operator multisig executing passed ballots
	keyRegistry  = []byte{0x04} // the registry (resolves "dividends" for excess returns)
	keyEscrowed  = []byte{0x05} // total FTB locked in live escrows

	prefixState  = []byte{0x10} // + id -> state
	prefixAmount = []byte{0x11} // + id -> remaining escrowed amount
	prefixDev    = []byte{0x12} // + id -> dev team address
	prefixAud    = []byte{0x13} // + id -> auditor address
	prefixHash   = []byte{0x14} // + id -> audited NEF hash (32 bytes)
	prefixHeight = []byte{0x15} // + id -> attestation block height
	prefixBps    = []byte{0x16} // + id -> tranche A share of the escrow, bps
	prefixSoak   = []byte{0x17} // + id -> soak blocks between attestation and tranche B
)

//lint:ignore U1000 VM entry point, invoked by the chain via the manifest, not from Go
func _deploy(data any, isUpdate bool) {
	if isUpdate {
		return
	}
	owner := data.(interop.Hash160)
	if len(owner) != 20 {
		panic("deploy: owner must be a 20-byte address")
	}
	storage.Put(storage.GetContext(), keyOwner, owner)
}

// Configure binds the FTB token, the operator committee, and the registry. Owner only,
// write-once: who holds the money and who turns the key must never change after publication.
func Configure(ftb, committee, registry interop.Hash160) {
	requireOwner()
	if len(ftb) != 20 || len(committee) != 20 || len(registry) != 20 {
		panic("configure: bad address")
	}
	ctx := storage.GetContext()
	if storage.Get(ctx, keyFTB) != nil {
		panic("configure: already configured (write-once)")
	}
	storage.Put(ctx, keyFTB, ftb)
	storage.Put(ctx, keyCommittee, committee)
	storage.Put(ctx, keyRegistry, registry)
	runtime.Notify("Configured", ftb, committee, registry)
}

// OnNEP17Payment receives income: the lend reserve split, future swap fees, donations.
// Only FTB is accepted; anything else faults and the transfer never happens.
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	if amount <= 0 {
		panic("treasury: amount must be positive")
	}
	if !eq(runtime.GetCallingScriptHash(), ftbHash()) {
		panic("treasury: only FTB accepted")
	}
	runtime.Notify("Received", from, amount)
}

// maxEscrowID bounds the id space: escKey packs ids into 4 big-endian bytes. NOTE: spelled
// as a literal, never as 1<<32 — shift expressions in this position make neo-go v0.123.0
// emit non-deterministic NEFs (found by the NEF-hash gate, FTB-33).
const maxEscrowID = 4294967296 // 2^32

// CreateEscrow locks a proposal budget after its ballot passes. Committee only. The dev
// team and the auditor must be different addresses — the one who writes the code is never
// the one who approves it.
func CreateEscrow(id int, dev, auditor interop.Hash160, amount, trancheABps, soakBlocks int) {
	requireCommittee()
	if id < 0 || id >= maxEscrowID {
		panic("escrow: bad id") // escKey packs the id into 4 bytes; the bound is proven, not hoped
	}
	if len(dev) != 20 || len(auditor) != 20 || eq(dev, auditor) {
		panic("escrow: bad dev/auditor")
	}
	if amount <= 0 || trancheABps <= 0 || trancheABps >= 10000 || soakBlocks < 0 {
		panic("escrow: bad terms")
	}
	ctx := storage.GetContext()
	sk := escKey(prefixState, id)
	if getIntCtx(ctx, sk) != stateNone {
		panic("escrow: id already used")
	}
	if amount > free(ctx) {
		panic("escrow: over free balance")
	}
	storage.Put(ctx, sk, stateOpen)
	storage.Put(ctx, escKey(prefixAmount, id), amount)
	storage.Put(ctx, escKey(prefixDev, id), dev)
	storage.Put(ctx, escKey(prefixAud, id), auditor)
	storage.Put(ctx, escKey(prefixBps, id), trancheABps)
	storage.Put(ctx, escKey(prefixSoak, id), soakBlocks)
	storage.Put(ctx, keyEscrowed, getIntCtx(ctx, keyEscrowed)+amount)
	runtime.Notify("EscrowCreated", id, dev, auditor, amount, trancheABps, soakBlocks)
}

// Attest records a passed audit. Auditor only, and the attestation binds the exact audited
// NEF hash — payment is tied to the bytes that were reviewed, not to a promise.
func Attest(id int, nefHash []byte) {
	ctx := storage.GetContext()
	aud := auditorOf(ctx, id)
	if aud == nil || !runtime.CheckWitness(aud) {
		panic("attest: only the escrow auditor")
	}
	if len(nefHash) != 32 {
		panic("attest: NEF hash must be 32 bytes")
	}
	sk := escKey(prefixState, id)
	if getIntCtx(ctx, sk) != stateOpen {
		panic("attest: not an open escrow")
	}
	storage.Put(ctx, sk, stateAttested)
	storage.Put(ctx, escKey(prefixHash, id), nefHash)
	storage.Put(ctx, escKey(prefixHeight, id), ledger.CurrentIndex())
	runtime.Notify("Attested", id, aud, nefHash)
}

// FailAudit records a failed audit; the budget returns to the free balance and the
// proposal must start over with fixed code. Auditor only.
func FailAudit(id int) {
	ctx := storage.GetContext()
	aud := auditorOf(ctx, id)
	if aud == nil || !runtime.CheckWitness(aud) {
		panic("fail: only the escrow auditor")
	}
	sk := escKey(prefixState, id)
	if getIntCtx(ctx, sk) != stateOpen {
		panic("fail: not an open escrow")
	}
	release(ctx, id, stateFailed)
	runtime.Notify("AuditFailed", id, aud)
}

// ClaimA pays tranche A to the dev team once the audit is attested. Self-service: the
// committee cannot delay, reroute, or withhold it.
func ClaimA(id int) {
	ctx := storage.GetContext()
	dev := devOf(ctx, id)
	if dev == nil || !runtime.CheckWitness(dev) {
		panic("claim: only the dev team")
	}
	sk := escKey(prefixState, id)
	if getIntCtx(ctx, sk) != stateAttested {
		panic("claim: audit not attested")
	}
	amount := getIntCtx(ctx, escKey(prefixAmount, id))
	tranche := amount * getIntCtx(ctx, escKey(prefixBps, id)) / 10000
	if tranche <= 0 || tranche >= amount {
		panic("claim: degenerate tranche") // bounded at create; defense in depth
	}
	storage.Put(ctx, sk, statePaidA)
	storage.Put(ctx, escKey(prefixAmount, id), amount-tranche)
	storage.Put(ctx, keyEscrowed, getIntCtx(ctx, keyEscrowed)-tranche)
	send(ftbHash(), dev, tranche)
	runtime.Notify("TranchePaid", id, dev, 1, tranche)
}

// ClaimB pays the remainder after the testnet soak elapses without a critical finding.
func ClaimB(id int) {
	ctx := storage.GetContext()
	dev := devOf(ctx, id)
	if dev == nil || !runtime.CheckWitness(dev) {
		panic("claim: only the dev team")
	}
	sk := escKey(prefixState, id)
	if getIntCtx(ctx, sk) != statePaidA {
		panic("claim: tranche A not claimed")
	}
	ready := getIntCtx(ctx, escKey(prefixHeight, id)) + getIntCtx(ctx, escKey(prefixSoak, id))
	if ledger.CurrentIndex() < ready {
		panic("claim: soak period still running")
	}
	amount := getIntCtx(ctx, escKey(prefixAmount, id))
	storage.Put(ctx, sk, stateDone)
	storage.Put(ctx, escKey(prefixAmount, id), 0)
	storage.Put(ctx, keyEscrowed, getIntCtx(ctx, keyEscrowed)-amount)
	send(ftbHash(), dev, amount)
	runtime.Notify("TranchePaid", id, dev, 2, amount)
}

// Cancel returns an un-attested escrow to the free balance. Once an audit is attested the
// funds are earned; there is no clawback of reviewed work.
func Cancel(id int) {
	requireCommittee()
	ctx := storage.GetContext()
	sk := escKey(prefixState, id)
	if getIntCtx(ctx, sk) != stateOpen {
		panic("cancel: not an open escrow")
	}
	release(ctx, id, stateCancelled)
	runtime.Notify("EscrowCancelled", id)
}

// ReturnExcess moves surplus accrual back to the FTB holders' dividends module — the
// registry-bound address, never an arbitrary one. Fail-closed while unbound. This is the
// accrual-cap lever: when the treasury holds more than near-term proposals need, the
// committee returns the excess instead of hoarding it.
func ReturnExcess(amount int) {
	requireCommittee()
	if amount <= 0 {
		panic("excess: amount must be positive")
	}
	ctx := storage.GetContext()
	if amount > free(ctx) {
		panic("excess: over free balance")
	}
	div := contract.Call(regHash(), "get", contract.ReadOnly, "dividends")
	if div == nil {
		panic("excess: dividends module not bound")
	}
	send(ftbHash(), div.(interop.Hash160), amount)
	runtime.Notify("ExcessReturned", div.(interop.Hash160), amount)
}

// ---- views (safe) ----

// State returns the escrow state: 0 none, 1 open, 2 attested, 3 tranche A paid, 4 done,
// 5 cancelled, 6 audit failed.
func State(id int) int { return getInt(escKey(prefixState, id)) }

// AmountOf returns the FTB still locked in the escrow (tranche A deducts from it).
func AmountOf(id int) int { return getInt(escKey(prefixAmount, id)) }

func DevOf(id int) interop.Hash160     { return devOf(storage.GetContext(), id) }
func AuditorOf(id int) interop.Hash160 { return auditorOf(storage.GetContext(), id) }

// AuditedHash is the exact NEF hash the auditor attested; nil before attestation.
func AuditedHash(id int) []byte {
	v := storage.Get(storage.GetContext(), escKey(prefixHash, id))
	if v == nil {
		return nil
	}
	return v.([]byte)
}

// AttestedAt is the attestation block height; tranche B unlocks at AttestedAt + soak.
func AttestedAt(id int) int { return getInt(escKey(prefixHeight, id)) }

// Escrowed is the total FTB locked in live escrows.
func Escrowed() int { return getInt(keyEscrowed) }

// Free is the un-escrowed balance: what new proposals can draw on.
func Free() int { return free(storage.GetContext()) }

func Committee() interop.Hash160 {
	return storage.Get(storage.GetContext(), keyCommittee).(interop.Hash160)
}
func Token() interop.Hash160 { return ftbHash() }

// ---- helpers ----

func release(ctx storage.Context, id, toState int) {
	amount := getIntCtx(ctx, escKey(prefixAmount, id))
	storage.Put(ctx, escKey(prefixState, id), toState)
	storage.Put(ctx, escKey(prefixAmount, id), 0)
	storage.Put(ctx, keyEscrowed, getIntCtx(ctx, keyEscrowed)-amount)
}

// escKey prefixes a proposal id as 4 big-endian bytes: fixed-width, collision-free. The id
// space is therefore bounded — enforced here, at the one place keys are built, so no id can
// ever alias onto another id's slot (Go canon Book II rule 5: a ceiling, not hope).
// CreateEscrow rejects out-of-range ids with its own message first; this guard is the
// fail-closed backstop on the view paths.
//
// NOTE: the bytes are appended one per statement, never as one variadic append — a
// multi-argument append of computed values makes neo-go v0.123.0 emit the SETITEM sequence
// in map order, producing different (semantically equal) NEFs per build and breaking the
// reproducibility gate (FTB-33, upstream compiler bug).
func escKey(prefix []byte, id int) []byte {
	if id < 0 || id >= maxEscrowID {
		panic("treasury: id out of range")
	}
	k := append(prefix, byte(id>>24)) // #nosec G115 -- id bounded by the guard above
	k = append(k, byte(id>>16))       // #nosec G115 -- id bounded by the guard above
	k = append(k, byte(id>>8))        // #nosec G115 -- id bounded by the guard above
	k = append(k, byte(id))           // #nosec G115 -- id bounded by the guard above
	return k
}

func free(ctx storage.Context) int {
	return cash() - getIntCtx(ctx, keyEscrowed)
}

func cash() int {
	return contract.Call(ftbHash(), "balanceOf", contract.ReadOnly, runtime.GetExecutingScriptHash()).(int)
}

func devOf(ctx storage.Context, id int) interop.Hash160 {
	v := storage.Get(ctx, escKey(prefixDev, id))
	if v == nil {
		return nil
	}
	return v.(interop.Hash160)
}

func auditorOf(ctx storage.Context, id int) interop.Hash160 {
	v := storage.Get(ctx, escKey(prefixAud, id))
	if v == nil {
		return nil
	}
	return v.(interop.Hash160)
}

func send(token, to interop.Hash160, amount int) {
	if amount == 0 {
		return
	}
	ok := contract.Call(token, "transfer", contract.All,
		runtime.GetExecutingScriptHash(), to, amount, nil).(bool)
	if !ok {
		panic("transfer failed")
	}
}

func ftbHash() interop.Hash160 {
	h := storage.Get(storage.GetContext(), keyFTB)
	if h == nil {
		panic("not configured")
	}
	return h.(interop.Hash160)
}

func regHash() interop.Hash160 {
	h := storage.Get(storage.GetContext(), keyRegistry)
	if h == nil {
		panic("not configured")
	}
	return h.(interop.Hash160)
}

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil {
		panic("owner not set")
	}
	if !runtime.CheckWitness(owner.(interop.Hash160)) {
		panic("only owner")
	}
}

func requireCommittee() {
	c := storage.Get(storage.GetContext(), keyCommittee)
	if c == nil {
		panic("not configured")
	}
	if !runtime.CheckWitness(c.(interop.Hash160)) {
		panic("only committee")
	}
}

func eq(a, b interop.Hash160) bool { return string(a) == string(b) }
func getInt(key []byte) int        { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
