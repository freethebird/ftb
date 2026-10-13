package main

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/util"
)

// A 20-byte account in the 0x big-endian form parseAccount accepts.
const (
	acctA = "0x639aaa7801f41c75d15c75aaad31aa6d6ede2b8b"
	acctB = "0x1111111111111111111111111111111111111111"
)

var head = []string{"address_or_scripthash", "flm_raw", "flocks_raw"}

func noWarn(string, ...any) {}

func rat(n, d int64) *big.Rat { return big.NewRat(n, d) }

// FTB-05: duplicate rows for one account (including two encodings of the same script
// hash) must collapse into a single leaf with summed balances, never two leaves.
func TestAggregateDuplicatesCollapse(t *testing.T) {
	u, err := util.Uint160DecodeStringBE(acctA[2:])
	if err != nil {
		t.Fatal(err)
	}
	le := hex.EncodeToString(u.BytesLE()) // the same hash in the bare LE form
	rows := [][]string{head,
		{acctA, "100", "10"},
		{le, "50", "5"},
		{acctB, "7", "0"},
	}
	items, total, skipped, err := aggregate(rows, rat(1, 1), rat(2, 1), noWarn)
	if err != nil || skipped != 0 {
		t.Fatalf("err=%v skipped=%d", err, skipped)
	}
	if len(items) != 2 {
		t.Fatalf("duplicates did not collapse: %d items", len(items))
	}
	// acctA: (100+50)*1 + (10+5)*2 = 180; acctB: 7. total 187.
	if total.String() != "187" {
		t.Fatalf("total = %s, want 187", total)
	}
}

// FTB-16: a negative balance is rejected and counted, never silently folded into the tree.
func TestAggregateNegativeSkipped(t *testing.T) {
	rows := [][]string{head,
		{acctA, "-5", "10"},
		{acctB, "3", "0"},
	}
	items, _, skipped, err := aggregate(rows, rat(1, 1), rat(1, 1), noWarn)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 || len(items) != 1 {
		t.Fatalf("skipped=%d items=%d, want 1 and 1", skipped, len(items))
	}
}

// A bad address is a skipped row, not a panic and not a silent zero.
func TestAggregateBadAddressSkipped(t *testing.T) {
	rows := [][]string{head, {"not-an-address", "5", "5"}}
	items, _, skipped, err := aggregate(rows, rat(1, 1), rat(1, 1), noWarn)
	if err != nil || skipped != 1 || len(items) != 0 {
		t.Fatalf("err=%v skipped=%d items=%d", err, skipped, len(items))
	}
}

// Entitlements are floored to whole base units: floor(3 * 1/2) = 1.
func TestAggregateFloors(t *testing.T) {
	rows := [][]string{head, {acctA, "3", "0"}}
	items, total, _, err := aggregate(rows, rat(1, 2), rat(0, 1), noWarn)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].amt.String() != "1" || total.String() != "1" {
		t.Fatalf("items=%v total=%s", items, total)
	}
}

// Zero entitlement yields no leaf and skips nothing.
func TestAggregateZeroEntitlement(t *testing.T) {
	rows := [][]string{head, {acctA, "0", "0"}}
	items, total, skipped, err := aggregate(rows, rat(1, 1), rat(4, 1), noWarn)
	if err != nil || skipped != 0 || len(items) != 0 || total.Sign() != 0 {
		t.Fatalf("err=%v skipped=%d items=%d total=%s", err, skipped, len(items), total)
	}
}

// Missing columns are a hard error.
func TestAggregateMissingColumns(t *testing.T) {
	rows := [][]string{{"address_or_scripthash", "flm_raw"}, {acctA, "1"}}
	if _, _, _, err := aggregate(rows, rat(1, 1), rat(1, 1), noWarn); err == nil {
		t.Fatal("missing flocks_raw column accepted")
	}
}

// FuzzAggregateRow is the LangSec fuzz gate for the snapshot parser: one fuzzed row must
// never panic the builder, never produce a negative total, and never both skip and emit.
func FuzzAggregateRow(f *testing.F) {
	f.Add(acctA, "123", "456")
	f.Add(acctA, "-1", "0")
	f.Add("garbage", "abc", "")
	f.Add("", "", "")
	f.Fuzz(func(t *testing.T, addr, flm, flk string) {
		if len(addr) > 128 || len(flm) > 64 || len(flk) > 64 {
			t.Skip()
		}
		rows := [][]string{head, {addr, flm, flk}}
		items, total, skipped, err := aggregate(rows, rat(1, 1), rat(4, 1), noWarn)
		if err != nil {
			t.Fatalf("fixed valid header rejected: %v", err)
		}
		if skipped != 0 && skipped != 1 {
			t.Fatalf("one row, skipped=%d", skipped)
		}
		if skipped == 1 && len(items) != 0 {
			t.Fatal("a skipped row still produced an item")
		}
		if len(items) > 1 {
			t.Fatal("one row produced more than one item")
		}
		if total.Sign() < 0 {
			t.Fatal("negative total")
		}
	})
}
