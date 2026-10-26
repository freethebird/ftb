package neo

import (
	"math/big"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func TestParseFixed(t *testing.T) {
	cases := []struct {
		in       string
		decimals int
		want     string
		wantErr  bool
	}{
		{"100", 8, "10000000000", false},
		{"100.5", 8, "10050000000", false},
		{"0.00000001", 8, "1", false},
		{"0", 8, "0", false},
		{"1.123456789", 8, "", true}, // too many decimals
		{"-5", 8, "", true},
		{"abc", 8, "", true},
	}
	for _, c := range cases {
		got, err := ParseFixed(c.in, c.decimals)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseFixed(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseFixed(%q): %v", c.in, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("ParseFixed(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestFormatFixedRoundTrip(t *testing.T) {
	for _, s := range []string{"100", "100.5", "0.00000001", "1234.00001"} {
		base, err := ParseFixed(s, 8)
		if err != nil {
			t.Fatalf("ParseFixed(%q): %v", s, err)
		}
		if got := FormatFixed(base, 8); got != s {
			t.Errorf("round trip %q -> %s", s, got)
		}
	}
}

func TestParseScriptHash(t *testing.T) {
	h, err := ParseScriptHash("0x1122334455667788990011223344556677889900")
	if err != nil {
		t.Fatalf("ParseScriptHash: %v", err)
	}
	// address round trip: hash -> N-address -> hash
	addr := address.Uint160ToString(h)
	back, err := ParseAddress(addr)
	if err != nil {
		t.Fatalf("ParseAddress(%s): %v", addr, err)
	}
	if !back.Equals(h) {
		t.Fatalf("address round trip mismatch: %s vs %s", back, h)
	}
	// Canonical 0x form is Neo's little-endian (StringLE) encoding: parsing it and
	// re-encoding via StringLE MUST reproduce the input. Regression guard — decoding
	// with DecodeStringBE reversed the bytes, so every contract call targeted a
	// non-existent byte-reversed address (e.g. vault 0x62f0…2b -> called 2bea…f062).
	const vault = "62f053d7f360d286e6ee0551155d7724564fea2b"
	hv, err := ParseScriptHash("0x" + vault)
	if err != nil {
		t.Fatalf("ParseScriptHash(vault): %v", err)
	}
	if hv.StringLE() != vault {
		t.Fatalf("canonical round trip: ParseScriptHash(0x%s).StringLE() = %s, want %s", vault, hv.StringLE(), vault)
	}
	if _, err := ParseScriptHash("nope"); err == nil {
		t.Fatal("expected error on bad hash")
	}
	if _, err := ParseAddress("not-an-address"); err == nil {
		t.Fatal("expected error on bad address")
	}
}

func TestParseTxHashCanonical(t *testing.T) {
	const tx = "e1b9f0a0000000000000000000000000000000000000000000000000000000aa"
	h, err := ParseTxHash("0x" + tx)
	if err != nil {
		t.Fatalf("ParseTxHash: %v", err)
	}
	if h.StringLE() != tx {
		t.Fatalf("tx canonical round trip: got %s want %s", h.StringLE(), tx)
	}
	if _, err := ParseTxHash("nope"); err == nil {
		t.Fatal("expected error on bad tx hash")
	}
}

func TestAccountMatchesEndiannessTolerant(t *testing.T) {
	h, err := ParseScriptHash("0xaabbccddeeff00112233445566778899aabbccdd")
	if err != nil {
		t.Fatal(err)
	}
	// Both BE and LE byte encodings of the same hash must match.
	if !accountMatches(stackitem.NewByteArray(h.BytesBE()), h) {
		t.Error("BE-encoded account should match")
	}
	if !accountMatches(stackitem.NewByteArray(h.BytesLE()), h) {
		t.Error("LE-encoded account should match")
	}
	// A different hash must not match.
	other, _ := ParseScriptHash("0x0000000000000000000000000000000000000001")
	if accountMatches(stackitem.NewByteArray(other.BytesBE()), h) {
		t.Error("different account should not match")
	}
	// Wrong length must not match.
	if accountMatches(stackitem.NewByteArray([]byte{1, 2, 3}), h) {
		t.Error("short byte string should not match")
	}
}

func TestParseFixedBig(t *testing.T) {
	// sanity: 21 million FTB at 8 decimals fits and parses exactly
	got, err := ParseFixed("21000000", 8)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := new(big.Int).SetString("2100000000000000", 10)
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", got, want)
	}
}
