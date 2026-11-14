package main

import (
	"math/big"
	"testing"
)

func TestParseHashlock(t *testing.T) {
	if _, err := parseHashlock("632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80"); err != nil {
		t.Fatalf("valid 32-byte hex: %v", err)
	}
	if _, err := parseHashlock("0x632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80"); err != nil {
		t.Fatalf("0x-prefixed: %v", err)
	}
	if _, err := parseHashlock("abcd"); err == nil {
		t.Fatal("short hex must be rejected")
	}
}

func TestNetOutFloorsCreditToUser(t *testing.T) {
	// Go bible IV.12 / DEX I7: the user's credit is FLOORED; the solver keeps the sub-unit
	// dust, so rounding never leaks a satoshi to the user. amountOut = floor(in*(10000-bps)/10000).
	// Case 100/20 is the catcher: the old "in - floor(fee)" form gave 100 (a satoshi leaked);
	// flooring the credit gives 99.
	cases := []struct {
		in   string
		bps  int
		want string
	}{
		{"100000", 20, "99800"},
		{"100", 20, "99"},
		{"1", 20, "0"},
		{"1000000000000000000", 20, "998000000000000000"},
	}
	for _, c := range cases {
		in, _ := new(big.Int).SetString(c.in, 10)
		if got := netOut(in, c.bps).String(); got != c.want {
			t.Fatalf("netOut(%s, %d) = %s, want %s", c.in, c.bps, got, c.want)
		}
	}
}
