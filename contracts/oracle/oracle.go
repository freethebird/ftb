// Package oracle is a hardened price feed for the lend market. Oracle manipulation is the
// number one killer of lending protocols, so this does NOT read a single spot AMM price.
// Instead a committee of whitelisted reporters each submit a price, and Price returns the
// MEDIAN of the reports that are still fresh, requiring a minimum number of them. A single
// compromised or stale reporter cannot move the price, and if too few fresh reports exist,
// Price reverts, which safely pauses borrowing rather than trading on a bad number.
//
// Reporters are a trust assumption. Use several independent operators, each aggregating
// multiple venues off-chain. The oracle module is swappable behind the registry timelock.
//
// Price unit: FTB base units per 1 whole asset (bNEO or GAS, both 8 decimals). So
// collateralValueFTB = assetBaseUnits * Price(asset) / 1e8.
//
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package oracle

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/iterator"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	keyOwner       = []byte{0x01}
	keyFreshnessMs = []byte{0x02}
	keyMinReports  = []byte{0x03}
	prefixReporter = []byte{0x10} // prefixReporter + reporter -> 1
	prefixPrice    = []byte{0x11} // prefixPrice + asset + reporter -> price
	prefixTime     = []byte{0x12} // prefixTime  + asset + reporter -> timestamp(ms)
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
	ctx := storage.GetContext()
	storage.Put(ctx, keyOwner, owner)
	storage.Put(ctx, keyFreshnessMs, 600000) // 10 minutes default
	storage.Put(ctx, keyMinReports, 1)       // >>> set to a real quorum (e.g. 3) before launch
}

// ---- admin (owner = multisig) ----

func SetReporter(reporter interop.Hash160, enabled bool) {
	requireOwner()
	if len(reporter) != 20 {
		panic("bad reporter")
	}
	ctx := storage.GetContext()
	if enabled {
		storage.Put(ctx, append(prefixReporter, reporter...), 1)
	} else {
		storage.Delete(ctx, append(prefixReporter, reporter...))
	}
	runtime.Notify("ReporterSet", reporter, enabled)
}

func SetParams(freshnessMs int, minReports int) {
	requireOwner()
	if freshnessMs <= 0 || minReports <= 0 {
		panic("bad params")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyFreshnessMs, freshnessMs)
	storage.Put(ctx, keyMinReports, minReports)
	runtime.Notify("ParamsSet", freshnessMs, minReports)
}

// ---- reporting ----

func Report(reporter interop.Hash160, asset interop.Hash160, price int) {
	if !Enabled(reporter) {
		panic("report: not a reporter")
	}
	if !runtime.CheckWitness(reporter) {
		panic("report: no witness")
	}
	if len(asset) != 20 || price <= 0 {
		panic("report: bad asset or price")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, priceKey(asset, reporter), price)
	storage.Put(ctx, timeKey(asset, reporter), runtime.GetTime())
	runtime.Notify("Reported", reporter, asset, price)
}

// ---- price (safe read) ----

// Price returns the median of fresh reporter prices for an asset, or reverts if too few
// are fresh (which pauses dependent borrowing, the safe failure mode).
func Price(asset interop.Hash160) int {
	ctx := storage.GetContext()
	now := runtime.GetTime()
	freshnessMs := getIntCtx(ctx, keyFreshnessMs)

	fresh := []int{}
	it := storage.Find(ctx, prefixReporter, storage.KeysOnly|storage.RemovePrefix)
	for iterator.Next(it) {
		reporter := iterator.Value(it).(interop.Hash160)
		ts := getIntCtx(ctx, timeKey(asset, reporter))
		if ts > 0 && now-ts <= freshnessMs {
			fresh = append(fresh, getIntCtx(ctx, priceKey(asset, reporter)))
		}
	}

	if len(fresh) < getIntCtx(ctx, keyMinReports) {
		panic("oracle: not enough fresh prices")
	}
	// insertion sort (committees are small)
	for i := 1; i < len(fresh); i++ {
		v := fresh[i]
		j := i - 1
		for j >= 0 && fresh[j] > v {
			fresh[j+1] = fresh[j]
			j--
		}
		fresh[j+1] = v
	}
	return fresh[(len(fresh)-1)/2] // lower median
}

func Enabled(reporter interop.Hash160) bool {
	return storage.Get(storage.GetContext(), append(prefixReporter, reporter...)) != nil
}
func Freshness() int  { return getInt(keyFreshnessMs) }
func MinReports() int { return getInt(keyMinReports) }

// ---- helpers ----

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil || !runtime.CheckWitness(owner.(interop.Hash160)) {
		panic("only owner")
	}
}
func priceKey(asset, reporter interop.Hash160) []byte {
	return append(append(prefixPrice, asset...), reporter...)
}
func timeKey(asset, reporter interop.Hash160) []byte {
	return append(append(prefixTime, asset...), reporter...)
}
func getInt(key []byte) int { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
