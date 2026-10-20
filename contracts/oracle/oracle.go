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
// Price unit: FTB base units per 1 WHOLE asset. NEO is 0-decimal (whole units), so the
// lend market values NEO collateral as wholeNEO * Price(NEO); GAS is 8-decimal, valued as
// gasBaseUnits * Price(GAS) / 1e8. Reporters MUST price per 1 whole asset, never per base
// unit — a per-base-unit (÷1e8) NEO report mis-scales collateral by 1e8 (O3 remediation).
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

const (
	minQuorum = 3 // O1: the minimum fresh-report quorum (never 1)
	// maxDevFactor is the O2 sanity gate. Once a quorum price exists, a single new report may
	// not be more than this factor above, or below, the current fresh median. Its job is to
	// reject a decimal / fat-finger mis-scale at entry — the NEO-decimals vector, where a
	// price mis-scaled by 1e8 would otherwise poison the feed. It is deliberately GENEROUS
	// (an order of magnitude), not a tight band: the real manipulation defense is the median
	// of a quorum of independent reporters — one compromised reporter cannot move the median —
	// and a tight per-report cap would freeze the feed during a genuine crash and block the
	// very liquidations the lend market needs, which is itself a lending-oracle anti-pattern.
	maxDevFactor = 10
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
	storage.Put(ctx, keyFreshnessMs, 600000)   // 10 minutes default
	storage.Put(ctx, keyMinReports, minQuorum) // O1: a real quorum by default, not 1
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
	if freshnessMs <= 0 || minReports < minQuorum {
		panic("bad params: minReports below the quorum floor")
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
	// O2 remediation: a sanity gate (not a tight circuit-breaker). Once a quorum price exists,
	// a single report may not be more than maxDevFactor× above or below the current fresh
	// median — this rejects a decimal / fat-finger mis-scale (a price off by ~1e8) at entry.
	// It is intentionally generous so it cannot freeze the feed during a real market move; the
	// median of the quorum, not this bound, is what stops a compromised reporter. During
	// bootstrap (no quorum yet) reports are unconstrained so the feed can establish itself.
	if cur, ok := medianFresh(ctx, asset); ok {
		if price > cur*maxDevFactor || price*maxDevFactor < cur {
			panic("report: price outside the sanity band (likely a decimal or fat-finger error)")
		}
	}
	storage.Put(ctx, priceKey(asset, reporter), price)
	storage.Put(ctx, timeKey(asset, reporter), runtime.GetTime())
	runtime.Notify("Reported", reporter, asset, price)
}

// ---- price (safe read) ----

// Price returns the median of fresh reporter prices for an asset, or reverts if too few
// are fresh (which pauses dependent borrowing, the safe failure mode).
func Price(asset interop.Hash160) int {
	m, ok := medianFresh(storage.GetContext(), asset)
	if !ok {
		panic("oracle: not enough fresh prices")
	}
	return m
}

// medianFresh returns the lower median of the fresh reporter prices for an asset and
// whether the fresh count met the quorum. Shared by Price (which reverts when short) and
// the Report circuit-breaker (which uses it as the reference to bound a new report).
func medianFresh(ctx storage.Context, asset interop.Hash160) (int, bool) {
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
		return 0, false
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
	return fresh[(len(fresh)-1)/2], true // lower median
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
