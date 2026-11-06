// Package ftbloanbook is the FTB peer-to-peer term-loan book: oracleless, keyless, and
// immutable. It replaces the pooled oracle-priced lend market (FtbLend) and removes the last
// price dependency -- and therefore the last standing key -- from the FTB stack.
//
// There is no oracle, no LTV, no liquidation threshold, no margin call, no risk parameter,
// no owner, and no upgrade path. The contract never asks what collateral is worth; it
// enforces one fact: was the loan repaid in full before maturity? The lender decides the
// collateral's worth when they post an offer. The willing lender is the oracle.
//
// Lifecycle. A lender escrows `principal` FTB into an OPEN offer naming a collateral asset
// and amount, a flat term interest rate (bps of principal), and a term (ms). A borrower
// takes the offer by transferring exactly that collateral; the contract stamps a wall-clock
// maturity (GetTime()+termMs, immune to Council block-time changes) and releases the
// principal. Before maturity the borrower repays principal+interest and gets the collateral
// back; the lender's proceeds are credited to a pull ledger. At/after maturity an unrepaid
// loan is closed by ANYONE -- the collateral is credited to the lender -- a clean swap with
// no fire-sale, no bad debt, and no price.
//
// Amounts: FTB and GAS are base units (8 decimals); NEO is whole units (0 decimals).
//
// Security: checks-effects-interactions on every path (state is written before any token is
// sent), the one permissionless path (Close) performs no outbound call at all, the lender
// side is pull not push (a contract-lender can never brick a borrower's repay), and
// collateral is restricted to the two native assets by an immutable allowlist.
//
// Reference implementation. Internal audit lineage only -- verify before trusting value.
package ftbloanbook

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/gas"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/neo"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const (
	bps        = 10000
	maxOfferID = 4294967296 // 2^32; an offer id is packed as four masked bytes

	// Immutable sanity bounds -- foot-gun guards, not economic policy. A single offer cannot
	// name an absurd rate or term; everything inside these bounds is the parties' own choice.
	maxRateBps = 1000000      // <= 10,000% flat over the term
	minTermMs  = 60000        // >= 1 minute, so a term spans several blocks and is repayable
	maxTermMs  = 315360000000 // <= 10 years

	// Offer states. stateNone(0) is the zero value, so a never-created id is distinguishable
	// from a live OPEN offer.
	stateNone      = 0
	stateOpen      = 1
	stateTaken     = 2
	stateRepaid    = 3
	stateDefaulted = 4
	stateCancelled = 5
)

var (
	keyFTB    = []byte{0x01} // the FTB token hash, injected at _deploy, read-only thereafter
	keyNextID = []byte{0x02} // next sequential offer id

	prefixFunder = []byte{0x10} // + id -> lender address
	prefixBorrow = []byte{0x11} // + id -> borrower address (set at take)
	prefixColTok = []byte{0x12} // + id -> collateral token hash
	prefixColAmt = []byte{0x13} // + id -> collateral amount
	prefixPrin   = []byte{0x14} // + id -> principal (FTB base units)
	prefixRate   = []byte{0x15} // + id -> flat term interest, bps of principal
	prefixTerm   = []byte{0x16} // + id -> term in ms
	prefixMatur  = []byte{0x17} // + id -> maturity timestamp in ms (set at take)
	prefixRepay  = []byte{0x18} // + id -> principal+interest due (set at take)
	prefixState  = []byte{0x19} // + id -> state
	prefixClaim  = []byte{0x20} // + funder + token -> withdrawable balance (pull ledger)
)

//lint:ignore U1000 VM entry point, invoked by the chain via the manifest, not from Go
func _deploy(data any, isUpdate bool) {
	if isUpdate {
		return
	}
	ftb := data.(interop.Hash160)
	if len(ftb) != 20 {
		panic("deploy: ftb token must be a 20-byte address")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyFTB, ftb)
	storage.Put(ctx, keyNextID, 0)
	runtime.Notify("Deployed", ftb)
}

// OnNEP17Payment is the only entry for value. FTB arrives to post an offer or to repay a
// loan; NEO or GAS arrives to take an offer. Every other token, and every malformed payload,
// faults -- which reverts the sender's own transfer, so nothing is ever half-locked.
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	token := runtime.GetCallingScriptHash()
	if from == nil {
		// A null sender is a mint. The only mint that can land here is the GAS the chain
		// distributes to this contract whenever its NEO balance changes (it holds NEO as
		// collateral). It belongs to no user; accept it silently. Rejecting it would fault
		// every NEO movement and brick takes, repays and withdrawals.
		if !eq(token, interop.Hash160(gas.Hash)) {
			panic("unexpected mint")
		}
		return
	}
	arr := data.([]any)
	action := arr[0].(string)
	if eq(token, ftbHash()) {
		if action == "offer" {
			postOffer(from, amount, arr)
		} else if action == "repay" {
			repayIn(from, amount, arr[1].(int))
		} else {
			panic("ftb: bad action")
		}
		return
	}
	if eq(token, interop.Hash160(neo.Hash)) || eq(token, interop.Hash160(gas.Hash)) {
		if action != "take" {
			panic("collateral: bad action")
		}
		takeOffer(from, token, amount, arr[1].(int))
		return
	}
	panic("token not accepted")
}

// postOffer escrows the lender's FTB and opens an offer. `principal` is the FTB received.
func postOffer(funder interop.Hash160, principal int, arr []any) {
	colTok := arr[1].(interop.Hash160)
	colAmt := arr[2].(int)
	rate := arr[3].(int)
	termMs := arr[4].(int)

	if principal <= 0 {
		panic("offer: principal must be positive")
	}
	if colAmt <= 0 {
		panic("offer: collateral amount must be positive")
	}
	if rate < 0 || rate > maxRateBps {
		panic("offer: rate out of range")
	}
	if termMs < minTermMs || termMs > maxTermMs {
		panic("offer: term out of range")
	}
	// Immutable v1 collateral allowlist: the two native assets only. A later version adds the
	// Flamingo LP by a fresh deploy, never by a key.
	if !eq(colTok, interop.Hash160(neo.Hash)) && !eq(colTok, interop.Hash160(gas.Hash)) {
		panic("offer: collateral token not allowed")
	}

	ctx := storage.GetContext()
	id := getIntCtx(ctx, keyNextID)
	storage.Put(ctx, keyNextID, id+1)

	storage.Put(ctx, okey(prefixFunder, id), funder)
	storage.Put(ctx, okey(prefixColTok, id), colTok)
	storage.Put(ctx, okey(prefixColAmt, id), colAmt)
	storage.Put(ctx, okey(prefixPrin, id), principal)
	storage.Put(ctx, okey(prefixRate, id), rate)
	storage.Put(ctx, okey(prefixTerm, id), termMs)
	storage.Put(ctx, okey(prefixState, id), stateOpen)

	runtime.Notify("OfferPosted", id, funder, colTok, colAmt, principal, rate, termMs)
}

// takeOffer opens a loan: the borrower has just transferred `colAmt` of `token`, which must
// match the offer exactly. The principal is released to the borrower.
func takeOffer(borrower, token interop.Hash160, colAmt, id int) {
	ctx := storage.GetContext()
	sk := okey(prefixState, id)
	if getIntCtx(ctx, sk) != stateOpen {
		panic("take: offer not open")
	}
	if !eq(token, storage.Get(ctx, okey(prefixColTok, id)).(interop.Hash160)) {
		panic("take: wrong collateral token")
	}
	if colAmt != getIntCtx(ctx, okey(prefixColAmt, id)) {
		panic("take: collateral amount mismatch")
	}

	principal := getIntCtx(ctx, okey(prefixPrin, id))
	rate := getIntCtx(ctx, okey(prefixRate, id))
	termMs := getIntCtx(ctx, okey(prefixTerm, id))
	maturity := runtime.GetTime() + termMs
	repayAmount := principal + principal*rate/bps

	// Effects before the interaction.
	storage.Put(ctx, sk, stateTaken)
	storage.Put(ctx, okey(prefixBorrow, id), borrower)
	storage.Put(ctx, okey(prefixMatur, id), maturity)
	storage.Put(ctx, okey(prefixRepay, id), repayAmount)

	send(ftbHash(), borrower, principal)

	runtime.Notify("LoanOpened", id, borrower, maturity, repayAmount)
}

// repayIn closes a loan in good standing. Anyone may pay (collateral always returns to the
// recorded borrower); the payer must cover the full amount due and any overpay is refunded.
func repayIn(payer interop.Hash160, amount, id int) {
	ctx := storage.GetContext()
	sk := okey(prefixState, id)
	if getIntCtx(ctx, sk) != stateTaken {
		panic("repay: loan not active")
	}
	if runtime.GetTime() >= getIntCtx(ctx, okey(prefixMatur, id)) {
		panic("repay: past maturity")
	}
	repayAmount := getIntCtx(ctx, okey(prefixRepay, id))
	if amount < repayAmount {
		panic("repay: insufficient amount")
	}

	funder := storage.Get(ctx, okey(prefixFunder, id)).(interop.Hash160)
	borrower := storage.Get(ctx, okey(prefixBorrow, id)).(interop.Hash160)
	colTok := storage.Get(ctx, okey(prefixColTok, id)).(interop.Hash160)
	colAmt := getIntCtx(ctx, okey(prefixColAmt, id))

	// Effects before interactions.
	storage.Put(ctx, sk, stateRepaid)
	credit(ctx, funder, ftbHash(), repayAmount)

	if amount > repayAmount {
		send(ftbHash(), payer, amount-repayAmount)
	}
	send(colTok, borrower, colAmt)

	runtime.Notify("LoanRepaid", id, borrower, repayAmount)
}

// Close settles a defaulted loan. Permissionless and callable by anyone once the term has
// elapsed: the collateral only ever goes to the lender, so the caller's identity is
// irrelevant. It performs NO outbound transfer -- the collateral is credited to the lender's
// pull ledger -- so it has no reentrancy surface.
func Close(id int) {
	ctx := storage.GetContext()
	sk := okey(prefixState, id)
	if getIntCtx(ctx, sk) != stateTaken {
		panic("close: loan not active")
	}
	if runtime.GetTime() < getIntCtx(ctx, okey(prefixMatur, id)) {
		panic("close: not yet matured")
	}
	funder := storage.Get(ctx, okey(prefixFunder, id)).(interop.Hash160)
	colTok := storage.Get(ctx, okey(prefixColTok, id)).(interop.Hash160)
	colAmt := getIntCtx(ctx, okey(prefixColAmt, id))

	storage.Put(ctx, sk, stateDefaulted)
	credit(ctx, funder, colTok, colAmt)

	runtime.Notify("LoanDefaulted", id, funder, colTok, colAmt)
}

// CancelOffer withdraws an un-taken offer and refunds the escrowed principal. Funder only.
func CancelOffer(id int) {
	ctx := storage.GetContext()
	sk := okey(prefixState, id)
	if getIntCtx(ctx, sk) != stateOpen {
		panic("cancel: offer not open")
	}
	funder := storage.Get(ctx, okey(prefixFunder, id)).(interop.Hash160)
	if !runtime.CheckWitness(funder) {
		panic("cancel: only funder")
	}
	principal := getIntCtx(ctx, okey(prefixPrin, id))

	storage.Put(ctx, sk, stateCancelled)
	send(ftbHash(), funder, principal)

	runtime.Notify("OfferCancelled", id, funder, principal)
}

// Withdraw pays out a lender's accumulated proceeds (repaid FTB and/or defaulted collateral)
// for one token. Funder only; the balance is zeroed before the send.
func Withdraw(funder, token interop.Hash160) {
	if !runtime.CheckWitness(funder) {
		panic("withdraw: no witness")
	}
	ctx := storage.GetContext()
	ck := claimKey(funder, token)
	bal := getIntCtx(ctx, ck)
	if bal <= 0 {
		panic("withdraw: nothing to claim")
	}
	storage.Put(ctx, ck, 0)
	send(token, funder, bal)

	runtime.Notify("Withdrawn", funder, token, bal)
}

// ---- views ----

func OfferCount() int { return getInt(keyNextID) }

func StatusOf(id int) int { return getInt(okey(prefixState, id)) }

func MaturityOf(id int) int { return getInt(okey(prefixMatur, id)) }

func RepayAmountOf(id int) int { return getInt(okey(prefixRepay, id)) }

func Claimable(funder, token interop.Hash160) int { return getInt(claimKey(funder, token)) }

func Ftb() interop.Hash160 { return ftbHash() }

// Offer returns the full record: funder, borrower, collateralToken, collateralAmount,
// principal, rateBps, termMs, maturity, repayAmount, state.
func Offer(id int) []any {
	ctx := storage.GetContext()
	return []any{
		storage.Get(ctx, okey(prefixFunder, id)),
		storage.Get(ctx, okey(prefixBorrow, id)),
		storage.Get(ctx, okey(prefixColTok, id)),
		getIntCtx(ctx, okey(prefixColAmt, id)),
		getIntCtx(ctx, okey(prefixPrin, id)),
		getIntCtx(ctx, okey(prefixRate, id)),
		getIntCtx(ctx, okey(prefixTerm, id)),
		getIntCtx(ctx, okey(prefixMatur, id)),
		getIntCtx(ctx, okey(prefixRepay, id)),
		getIntCtx(ctx, okey(prefixState, id)),
	}
}

// ---- helpers ----

func ftbHash() interop.Hash160 { return storage.Get(storage.GetContext(), keyFTB).(interop.Hash160) }

// send performs a NEP-17 transfer out and checks the boolean result (FTB-15): if a token ever
// returns false instead of faulting, our storage was already updated and the tokens must
// provably have moved.
func send(token, to interop.Hash160, amount int) {
	if !contract.Call(token, "transfer", contract.All, runtime.GetExecutingScriptHash(), to, amount, nil).(bool) {
		panic("send: token transfer returned false")
	}
}

func credit(ctx storage.Context, who, token interop.Hash160, amount int) {
	ck := claimKey(who, token)
	storage.Put(ctx, ck, getIntCtx(ctx, ck)+amount)
}

func eq(a, b interop.Hash160) bool { return string(a) == string(b) }

func claimKey(funder, token interop.Hash160) []byte {
	k := append(prefixClaim, funder...)
	return append(k, token...)
}

// okey packs an offer id into a four-byte suffix. Every byte is explicitly masked: neo-go
// does not auto-mask a byte() conversion, and an unmasked high byte faults on SETITEM into a
// storage buffer for any id >= 256 (the bug fixed in treasury/dividends/gov).
func okey(prefix []byte, id int) []byte {
	if id < 0 || id >= maxOfferID {
		panic("loanbook: id out of range")
	}
	k := append(prefix, byte((id>>24)&0xFF)) // #nosec G115 -- masked to one byte
	k = append(k, byte((id>>16)&0xFF))       // #nosec G115 -- masked to one byte
	k = append(k, byte((id>>8)&0xFF))        // #nosec G115 -- masked to one byte
	k = append(k, byte(id&0xFF))             // #nosec G115 -- masked to one byte
	return k
}

func getInt(key []byte) int { return getIntCtx(storage.GetContext(), key) }

func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
