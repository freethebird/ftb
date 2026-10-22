// Package dbondvault is the FTB Discord member-bond vault (Neo N3 / neo-go).
//
// Custody of fixed-size FTB bonds tagged to a Discord user. A bond is posted by
// sending exactly bondAmount FTB with the member's Discord ID as the transfer
// data. The owner may refund a clean bond; only the accusation ballot contract
// may condemn (burn) a bond, and only governance (the timelock) may change the
// parameters. No admin key moves, refunds or burns a bond.
//
// Ownerless: no Update method, no ContractManagement permission (immutable
// bytecode); the deploy owner's only power is the one-time write-once Configure.
// No oracle, no price anywhere.
package dbondvault

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/std"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

// Bond status values.
const (
	statusActive     = 1
	statusRefunded   = 2
	statusLiquidated = 3
)

// Bond is one posted bond. Field order IS the interface getBond returns to the
// bots: [owner, amount, discordId, status, depositTx, createdAt].
type Bond struct {
	Owner     interop.Hash160
	Amount    int
	DiscordID []byte
	Status    int
	DepositTx interop.Hash256
	CreatedAt int
}

// per-id / per-key storage prefixes.
const (
	prefixBond    = "b" // b<id>        -> serialized Bond
	prefixDiscord = "d" // d<discordId> -> bond id
	prefixWallet  = "w" // w<wallet>    -> bond id (one active bond per wallet)
	prefixGuard   = "g" // g<id>        -> 1 while an accusation case is open
)

// singleton storage keys.
const (
	keyOwner      = "O"
	keyFTB        = "F"
	keyAccusation = "A"
	keyGov        = "G" // governance timelock (the only caller allowed to setParams)
	keyBurn       = "K" // burn sink (provably unspendable)
	keyConfigured = "C"
	keyBondAmount = "pA"
	keyBanBps     = "pB"
	keyWindow     = "pW"
	keyNextID     = "N"
	keyTotalStake = "T"
)

// _deploy records the deploy owner (passed as deploy data). The owner's only
// power is the one-time Configure; it holds no authority afterwards.
func _deploy(data any, isUpdate bool) {
	if isUpdate {
		return
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyOwner, data.(interop.Hash160))
}

// Configure wires the contract once. After this the owner has no powers.
func Configure(ftb, accusation, governance, burn interop.Hash160, bondAmount, banBps, window int) {
	ctx := storage.GetContext()
	if storage.Get(ctx, keyConfigured) != nil {
		panic("already configured")
	}
	owner := storage.Get(ctx, keyOwner).(interop.Hash160)
	if !runtime.CheckWitness(owner) {
		panic("configure: owner witness required")
	}
	validateParams(bondAmount, banBps, window)
	storage.Put(ctx, keyFTB, ftb)
	storage.Put(ctx, keyAccusation, accusation)
	storage.Put(ctx, keyGov, governance)
	storage.Put(ctx, keyBurn, burn)
	storage.Put(ctx, keyBondAmount, bondAmount)
	storage.Put(ctx, keyBanBps, banBps)
	storage.Put(ctx, keyWindow, window)
	storage.Put(ctx, keyNextID, 1)
	storage.Put(ctx, keyTotalStake, 0)
	storage.Put(ctx, keyConfigured, 1)
	runtime.Notify("Configured", accusation, governance, bondAmount, banBps, window)
}

// OnNEP17Payment posts a bond. It accepts only the FTB token, only the exact
// current bondAmount, and only one active bond per member (by Discord ID and by
// wallet). data must be the member's Discord user ID (bytes).
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	if !runtime.GetCallingScriptHash().Equals(storageHash(ctx, keyFTB)) {
		panic("only FTB is accepted")
	}
	if amount != storageInt(ctx, keyBondAmount) {
		panic("bond must be exactly bondAmount FTB")
	}
	discordID := data.([]byte)
	if len(discordID) == 0 {
		panic("missing discord id in payment data")
	}
	if storage.Get(ctx, discordKey(discordID)) != nil {
		panic("member already holds an active bond")
	}
	if storage.Get(ctx, walletKey(from)) != nil {
		panic("wallet already holds an active bond")
	}
	id := storageInt(ctx, keyNextID)
	b := Bond{
		Owner:     from,
		Amount:    amount,
		DiscordID: discordID,
		Status:    statusActive,
		DepositTx: runtime.GetScriptContainer().Hash,
		CreatedAt: runtime.GetTime(),
	}
	storage.Put(ctx, bondKey(id), std.Serialize(b))
	storage.Put(ctx, discordKey(discordID), id)
	storage.Put(ctx, walletKey(from), id)
	storage.Put(ctx, keyNextID, id+1)
	storage.Put(ctx, keyTotalStake, storageInt(ctx, keyTotalStake)+amount)
	runtime.Notify("Bonded", id, from, discordID, amount)
}

// Refund returns a clean bond to its owner. Only the owner, only while active
// and not locked by an open case.
func Refund(bondID int) {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	b := loadBond(ctx, bondID)
	if b.Status != statusActive {
		panic("bond is not active")
	}
	if !runtime.CheckWitness(b.Owner) {
		panic("only the bond owner may refund")
	}
	if storage.Get(ctx, guardKey(bondID)) != nil {
		panic("bond locked: an accusation case is open")
	}
	// CEI: write state before the external transfer.
	b.Status = statusRefunded
	storage.Put(ctx, bondKey(bondID), std.Serialize(b))
	storage.Delete(ctx, discordKey(b.DiscordID))
	storage.Delete(ctx, walletKey(b.Owner))
	storage.Put(ctx, keyTotalStake, storageInt(ctx, keyTotalStake)-b.Amount)
	transferFTB(ctx, b.Owner, b.Amount)
	runtime.Notify("Refunded", bondID, b.Owner, b.Amount)
}

// Condemn burns a bond. ONLY the accusation contract may call it (set once at
// Configure); there is no key path. This is the on-chain half of a ban.
func Condemn(bondID int) {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	requireAccusation(ctx)
	b := loadBond(ctx, bondID)
	if b.Status != statusActive {
		panic("bond is not active")
	}
	b.Status = statusLiquidated
	storage.Put(ctx, bondKey(bondID), std.Serialize(b))
	storage.Delete(ctx, discordKey(b.DiscordID))
	storage.Delete(ctx, walletKey(b.Owner))
	storage.Delete(ctx, guardKey(bondID))
	storage.Put(ctx, keyTotalStake, storageInt(ctx, keyTotalStake)-b.Amount)
	transferFTB(ctx, storageHash(ctx, keyBurn), b.Amount)
	runtime.Notify("Liquidated", bondID, b.Owner, b.Amount)
}

// LockForCase / UnlockForCase toggle the refund lock while a case runs. Ballot
// only.
func LockForCase(bondID int) {
	ctx := storage.GetContext()
	requireAccusation(ctx)
	storage.Put(ctx, guardKey(bondID), 1)
}

func UnlockForCase(bondID int) {
	ctx := storage.GetContext()
	requireAccusation(ctx)
	storage.Delete(ctx, guardKey(bondID))
}

// SetParams updates the governed parameters. ONLY the governance timelock may
// call it. Existing bonds keep their recorded amount (grandfathered); only new
// deposits use the new bondAmount.
func SetParams(bondAmount, banBps, window int) {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	// CheckWitness lets governance be either a multisig account (it signs the
	// tx) or the timelock contract (it calls via contract.Call); both satisfy it.
	if !runtime.CheckWitness(storageHash(ctx, keyGov)) {
		panic("only governance may set params")
	}
	validateParams(bondAmount, banBps, window)
	storage.Put(ctx, keyBondAmount, bondAmount)
	storage.Put(ctx, keyBanBps, banBps)
	storage.Put(ctx, keyWindow, window)
	runtime.Notify("ParamsUpdated", bondAmount, banBps, window)
}

// ---- read methods (safe) ----

// GetBond returns the bond as [owner, amount, discordId, status, depositTx,
// createdAt], or Null if there is none.
func GetBond(bondID int) any {
	ctx := storage.GetReadOnlyContext()
	data := storage.Get(ctx, bondKey(bondID))
	if data == nil {
		return nil
	}
	return std.Deserialize(data.([]byte))
}

// GetBondByDiscord returns the active bond id for a Discord user, or 0.
func GetBondByDiscord(discordID []byte) int {
	ctx := storage.GetReadOnlyContext()
	v := storage.Get(ctx, discordKey(discordID))
	if v == nil {
		return 0
	}
	return v.(int)
}

// IsBondedVoter reports whether a wallet holds an active bond.
func IsBondedVoter(wallet interop.Hash160) bool {
	ctx := storage.GetReadOnlyContext()
	return storage.Get(ctx, walletKey(wallet)) != nil
}

// StakeOf returns a wallet's active bonded stake (0 if none).
func StakeOf(wallet interop.Hash160) int {
	ctx := storage.GetReadOnlyContext()
	v := storage.Get(ctx, walletKey(wallet))
	if v == nil {
		return 0
	}
	b := loadBond(ctx, v.(int))
	if b.Status != statusActive {
		return 0
	}
	return b.Amount
}

func BondAmount() int             { return storageInt(storage.GetReadOnlyContext(), keyBondAmount) }
func BanBps() int                 { return storageInt(storage.GetReadOnlyContext(), keyBanBps) }
func VotingWindow() int           { return storageInt(storage.GetReadOnlyContext(), keyWindow) }
func TotalActiveBondedStake() int { return storageInt(storage.GetReadOnlyContext(), keyTotalStake) }

// ---- internals ----

func validateParams(bondAmount, banBps, window int) {
	if bondAmount <= 0 {
		panic("bondAmount must be positive")
	}
	if banBps <= 0 || banBps > 10000 {
		panic("banBps must be in 1..10000")
	}
	if window <= 0 {
		panic("window must be positive")
	}
}

func requireConfigured(ctx storage.Context) {
	if storage.Get(ctx, keyConfigured) == nil {
		panic("not configured")
	}
}

func requireAccusation(ctx storage.Context) {
	if !runtime.GetCallingScriptHash().Equals(storageHash(ctx, keyAccusation)) {
		panic("only the accusation ballot may call this")
	}
}

func transferFTB(ctx storage.Context, to interop.Hash160, amount int) {
	self := runtime.GetExecutingScriptHash()
	ok := contract.Call(storageHash(ctx, keyFTB), "transfer", contract.All, self, to, amount, nil).(bool)
	if !ok {
		panic("FTB transfer failed")
	}
}

func loadBond(ctx storage.Context, id int) Bond {
	data := storage.Get(ctx, bondKey(id))
	if data == nil {
		panic("no such bond")
	}
	return std.Deserialize(data.([]byte)).(Bond)
}

func storageInt(ctx storage.Context, key string) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}

func storageHash(ctx storage.Context, key string) interop.Hash160 {
	return storage.Get(ctx, key).(interop.Hash160)
}

func bondKey(id int) []byte              { return append([]byte(prefixBond), []byte(std.Itoa(id, 10))...) }
func guardKey(id int) []byte             { return append([]byte(prefixGuard), []byte(std.Itoa(id, 10))...) }
func discordKey(d []byte) []byte         { return append([]byte(prefixDiscord), d...) }
func walletKey(w interop.Hash160) []byte { return append([]byte(prefixWallet), w...) }
