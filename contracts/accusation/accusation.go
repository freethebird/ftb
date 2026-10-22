// Package accusation is the FTB Discord-bond dispute ballot (Neo N3 / neo-go).
//
// A bonded member files a case against another member's bond; bonded members
// vote, weighted by their own bonded stake; if "liquidate" reaches banBps of the
// bonded stake snapshotted at filing (and outweighs "dismiss"), the bond is
// burned via dbondvault.Condemn. Separate from protocol governance; holds no
// funds; keyless (no Update) after the write-once Configure.
//
// The bot never calls fileCase/vote — a bonded member signs those with their own
// wallet on the signing page. This contract only ever reads the vault and, on a
// passed vote, calls condemn/unlock on it.
package accusation

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/std"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

// Case status values.
const (
	caseOpen       = 0
	caseLiquidated = 1
	caseDismissed  = 2
)

// Case field order IS the interface getCase returns to the bots:
// [accusedBondId, reasonCode, evidenceHash, forStake, againstStake, status,
// deadline, threshold]. forStake/againstStake are stake totals; threshold is the
// absolute stake required to liquidate (banBps * denomSnapshot / 10000).
type Case struct {
	AccusedBondID int
	ReasonCode    int
	EvidenceHash  []byte
	ForStake      int
	AgainstStake  int
	Status        int
	Deadline      int
	Threshold     int
}

const (
	prefixCase     = "c" // c<id>            -> serialized Case
	prefixVoted    = "v" // v<id>|<voter>    -> 1
	prefixOpenBond = "o" // o<bondId>        -> case id (guards one open case per bond)
)

const (
	keyOwner      = "O"
	keyVault      = "V"
	keyConfigured = "C"
	keyNextCase   = "N"
	keyOpenCases  = "L"
)

func _deploy(data any, isUpdate bool) {
	if isUpdate {
		return
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyOwner, data.(interop.Hash160))
}

// Configure wires the vault hash once. After this the owner has no powers.
func Configure(vault interop.Hash160) {
	ctx := storage.GetContext()
	if storage.Get(ctx, keyConfigured) != nil {
		panic("already configured")
	}
	owner := storage.Get(ctx, keyOwner).(interop.Hash160)
	if !runtime.CheckWitness(owner) {
		panic("configure: owner witness required")
	}
	storage.Put(ctx, keyVault, vault)
	storage.Put(ctx, keyNextCase, 1)
	storage.Put(ctx, keyOpenCases, std.Serialize([]int{}))
	storage.Put(ctx, keyConfigured, 1)
	runtime.Notify("Configured", vault)
}

// FileCase opens a case against a bond. Only a bonded member may file, against
// an active bond with no case already open. Snapshots the denominator (bonded
// stake minus the accused's own) and locks the accused bond from refund.
func FileCase(filer interop.Hash160, accusedBondID, reasonCode int, evidenceHash []byte) int {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	if !runtime.CheckWitness(filer) {
		panic("filer witness required")
	}
	if len(evidenceHash) != 32 {
		panic("evidenceHash must be 32 bytes")
	}
	vault := vaultHash(ctx)
	if !contract.Call(vault, "isBondedVoter", contract.ReadOnly, filer).(bool) {
		panic("only a bonded member may file")
	}
	if storage.Get(ctx, openBondKey(accusedBondID)) != nil {
		panic("a case is already open for this bond")
	}
	owner, amount, status := accusedFields(vault, accusedBondID)
	if status != 1 { // dbondvault statusActive
		panic("accused bond is not active")
	}
	if filer.Equals(owner) {
		panic("cannot file against your own bond")
	}

	total := contract.Call(vault, "totalActiveBondedStake", contract.ReadOnly).(int)
	banBps := contract.Call(vault, "banBps", contract.ReadOnly).(int)
	window := contract.Call(vault, "votingWindow", contract.ReadOnly).(int)
	denom := total - amount // exclude the accused's own stake
	if denom < 0 {
		denom = 0
	}
	threshold := banBps * denom / 10000

	id := storageInt(ctx, keyNextCase)
	c := Case{
		AccusedBondID: accusedBondID,
		ReasonCode:    reasonCode,
		EvidenceHash:  evidenceHash,
		ForStake:      0,
		AgainstStake:  0,
		Status:        caseOpen,
		Deadline:      ledger.CurrentIndex() + window,
		Threshold:     threshold,
	}
	// State before the external lock call (CEI).
	storage.Put(ctx, caseKey(id), std.Serialize(c))
	storage.Put(ctx, openBondKey(accusedBondID), id)
	storage.Put(ctx, keyNextCase, id+1)
	addOpenCase(ctx, id)

	contract.Call(vault, "lockForCase", contract.All, accusedBondID)
	runtime.Notify("CaseFiled", id, accusedBondID, reasonCode, evidenceHash, filer)
	return id
}

// Vote casts one stake-weighted vote. One vote per bonded member per case; the
// accused owner may not vote on their own case.
func Vote(voter interop.Hash160, caseID int, liquidate bool) {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	if !runtime.CheckWitness(voter) {
		panic("voter witness required")
	}
	c := loadCase(ctx, caseID)
	if c.Status != caseOpen {
		panic("case is closed")
	}
	if ledger.CurrentIndex() > c.Deadline {
		panic("voting window has closed")
	}
	vault := vaultHash(ctx)
	owner, _, _ := accusedFields(vault, c.AccusedBondID)
	if voter.Equals(owner) {
		panic("the accused may not vote on their own case")
	}
	weight := contract.Call(vault, "stakeOf", contract.ReadOnly, voter).(int)
	if weight <= 0 {
		panic("only a bonded member may vote")
	}
	vk := votedKey(caseID, voter)
	if storage.Get(ctx, vk) != nil {
		panic("already voted")
	}
	// CEI: record the vote before tallying/storing.
	storage.Put(ctx, vk, 1)
	if liquidate {
		c.ForStake += weight
	} else {
		c.AgainstStake += weight
	}
	storage.Put(ctx, caseKey(caseID), std.Serialize(c))
	runtime.Notify("Voted", caseID, voter, liquidate, weight)
}

// Resolve closes a case. Permissionless. Liquidates when "for" reaches the
// threshold and outweighs "against"; otherwise dismisses. Either way it unwinds
// the open-case state and tells the vault to condemn or unlock.
func Resolve(caseID int) {
	ctx := storage.GetContext()
	requireConfigured(ctx)
	c := loadCase(ctx, caseID)
	if c.Status != caseOpen {
		panic("case already resolved")
	}
	reached := c.ForStake >= c.Threshold
	if !reached && ledger.CurrentIndex() <= c.Deadline {
		panic("voting still open")
	}
	pass := reached && c.ForStake > c.AgainstStake
	if pass {
		c.Status = caseLiquidated
	} else {
		c.Status = caseDismissed
	}
	// State before the external call (CEI).
	storage.Put(ctx, caseKey(caseID), std.Serialize(c))
	storage.Delete(ctx, openBondKey(c.AccusedBondID))
	removeOpenCase(ctx, caseID)

	vault := vaultHash(ctx)
	if pass {
		contract.Call(vault, "condemn", contract.All, c.AccusedBondID)
	} else {
		contract.Call(vault, "unlockForCase", contract.All, c.AccusedBondID)
	}
	runtime.Notify("CaseResolved", caseID, c.AccusedBondID, c.Status)
}

// ---- read methods (safe) ----

// GetCase returns the case array, or Null if absent.
func GetCase(caseID int) any {
	ctx := storage.GetReadOnlyContext()
	data := storage.Get(ctx, caseKey(caseID))
	if data == nil {
		return nil
	}
	return std.Deserialize(data.([]byte))
}

// ListOpenCases returns the ids of all open cases.
func ListOpenCases() []int {
	ctx := storage.GetReadOnlyContext()
	return std.Deserialize(storage.Get(ctx, keyOpenCases).([]byte)).([]int)
}

// IsBondedVoter delegates to the vault (voter eligibility).
func IsBondedVoter(wallet interop.Hash160) bool {
	ctx := storage.GetReadOnlyContext()
	return contract.Call(vaultHash(ctx), "isBondedVoter", contract.ReadOnly, wallet).(bool)
}

// ---- internals ----

func requireConfigured(ctx storage.Context) {
	if storage.Get(ctx, keyConfigured) == nil {
		panic("not configured")
	}
}

func vaultHash(ctx storage.Context) interop.Hash160 {
	return storage.Get(ctx, keyVault).(interop.Hash160)
}

// accusedFields reads (owner, amount, status) from the vault's getBond array.
func accusedFields(vault interop.Hash160, bondID int) (interop.Hash160, int, int) {
	res := contract.Call(vault, "getBond", contract.ReadOnly, bondID)
	if res == nil {
		panic("accused bond does not exist")
	}
	b := res.([]any)
	return b[0].(interop.Hash160), b[1].(int), b[3].(int)
}

func loadCase(ctx storage.Context, id int) Case {
	data := storage.Get(ctx, caseKey(id))
	if data == nil {
		panic("no such case")
	}
	return std.Deserialize(data.([]byte)).(Case)
}

func addOpenCase(ctx storage.Context, id int) {
	list := std.Deserialize(storage.Get(ctx, keyOpenCases).([]byte)).([]int)
	list = append(list, id)
	storage.Put(ctx, keyOpenCases, std.Serialize(list))
}

func removeOpenCase(ctx storage.Context, id int) {
	list := std.Deserialize(storage.Get(ctx, keyOpenCases).([]byte)).([]int)
	out := []int{}
	for i := 0; i < len(list); i++ {
		if list[i] != id {
			out = append(out, list[i])
		}
	}
	storage.Put(ctx, keyOpenCases, std.Serialize(out))
}

func storageInt(ctx storage.Context, key string) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}

func caseKey(id int) []byte     { return append([]byte(prefixCase), []byte(std.Itoa(id, 10))...) }
func openBondKey(id int) []byte { return append([]byte(prefixOpenBond), []byte(std.Itoa(id, 10))...) }

func votedKey(id int, voter interop.Hash160) []byte {
	k := append([]byte(prefixVoted), []byte(std.Itoa(id, 10))...)
	k = append(k, '|')
	return append(k, voter...)
}
