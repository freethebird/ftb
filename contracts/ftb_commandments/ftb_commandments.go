// Package ftbcommandments is FTB's on-chain governance record -- the "stone commandments."
// A decided proposal is inked here once, forever: its question, the source ballot, the
// per-option tally, the winning option, and the ratified outcome in words. There is no
// Update and, after the genesis commandments are inked, the owner is renounced, so every
// record is immutable -- carved in stone.
//
// It holds no funds, mints nothing, and calls no other FTB contract. The source ballot hash
// lets anyone re-derive the tally from that ballot independently; the commandment is the
// canonical, human-readable ratification the rest of the system (the token's claim-window
// deadline, the front end) points back to.
//
// Reference implementation. Internal audit lineage only. Verify before trusting value.
package ftbcommandments

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/std"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	keyOwner  = []byte{0x01} // admin; renounceable, one-way
	keyCount  = []byte{0x02} // number of commandments inked (also the highest id)
	prefixCmd = []byte{0x10} // prefixCmd + id byte -> std.Serialize(record)
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

// Ink records one commandment, in order (number must be count+1), write-once. Owner only --
// and the owner is renounced once the genesis commandments are inked, after which Ink can
// never be called again. The tally is recorded as read from the source ballot; the ballot
// hash is stored so anyone can re-derive it.
func Ink(number int, question string, sourceBallot interop.Hash160, options []string, tallies []int, winner int, ratified string) {
	requireOwner()
	ctx := storage.GetContext()
	if number < 1 || number > 255 {
		panic("ink: number out of range")
	}
	if number != getInt(keyCount)+1 {
		panic("ink: commandments are sequential (next = count+1); write-once")
	}
	if len(question) == 0 {
		panic("ink: question required")
	}
	if len(sourceBallot) != 20 {
		panic("ink: source ballot must be a 20-byte hash")
	}
	n := len(options)
	if n < 2 || n != len(tallies) {
		panic("ink: need >= 2 options and one tally each")
	}
	if winner < 1 || winner > n {
		panic("ink: winner out of range")
	}
	if len(ratified) == 0 {
		panic("ink: ratified value required")
	}
	rec := []any{number, question, sourceBallot, options, tallies, winner, ratified, ledger.CurrentIndex(), runtime.GetTime()}
	storage.Put(ctx, append(prefixCmd, byte(number)), std.Serialize(rec)) // #nosec G115 -- number bounded to 1..255 above
	storage.Put(ctx, keyCount, number)
	runtime.Notify("Inked", number, winner, ratified)
}

// Commandment returns the inked record for a number:
// [number, question, sourceBallot, options, tallies, winner, ratified, inkedHeight, inkedTime].
func Commandment(number int) []any {
	if number < 1 || number > 255 {
		panic("no such commandment")
	}
	v := storage.Get(storage.GetContext(), append(prefixCmd, byte(number))) // #nosec G115 -- bounded 1..255 above
	if v == nil {
		panic("no such commandment")
	}
	return std.Deserialize(v.([]byte)).([]any)
}

// Count is the number of commandments inked (also the highest id, since they are sequential).
func Count() int { return getInt(keyCount) }

// RenounceOwnership freezes the record forever: no more Ink, no owner. One-way door.
func RenounceOwnership() {
	requireOwner()
	storage.Put(storage.GetContext(), keyOwner, []byte{})
	runtime.Notify("OwnershipRenounced")
}

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil {
		panic("owner not set")
	}
	o := owner.(interop.Hash160)
	if len(o) != 20 {
		panic("only owner") // renounced: owner cleared, no valid witness possible
	}
	if !runtime.CheckWitness(o) {
		panic("only owner")
	}
}

func getInt(key []byte) int {
	v := storage.Get(storage.GetContext(), key)
	if v == nil {
		return 0
	}
	return v.(int)
}
