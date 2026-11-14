package evm

import (
	"encoding/hex"
	"math/big"
	"testing"
)

func TestSelectorKAT(t *testing.T) {
	cases := []struct{ sig, want string }{
		{"transfer(address,uint256)", "a9059cbb"},
		{"balanceOf(address)", "70a08231"},
		{"transferFrom(address,address,uint256)", "23b872dd"},
	}
	for _, c := range cases {
		s := Selector(c.sig)
		if hex.EncodeToString(s[:]) != c.want {
			t.Errorf("Selector(%q) = %x, want %s", c.sig, s, c.want)
		}
	}
}

func TestHTLCSelectors(t *testing.T) {
	want := map[string]string{
		"lock(bytes32,address,address,uint256,uint256)": "b21a99d2",
		"claim(bytes32,bytes32)":                        "84cc9dfb",
		"refund(bytes32)":                               "7249fbb6",
		"stateOf(bytes32)":                              "64482ac4",
		"swaps(bytes32)":                                "eb84e7f2",
	}
	for sig, w := range want {
		s := Selector(sig)
		if hex.EncodeToString(s[:]) != w {
			t.Errorf("Selector(%q) = %x, want %s", sig, s, w)
		}
	}
}

func TestABIWordRoundTrip(t *testing.T) {
	var a [20]byte
	for i := range a {
		a[i] = byte(i + 1)
	}
	if got := WordToAddress(WordAddress(a)); got != a {
		t.Error("address round-trip failed")
	}
	x := new(big.Int).SetBytes([]byte{0xde, 0xad, 0xbe, 0xef})
	if got := WordToUint(WordUint(x)); got.Cmp(x) != 0 {
		t.Errorf("uint round-trip: got %s want %s", got, x)
	}
	if ws := SplitWords(make([]byte, 64)); len(ws) != 2 {
		t.Errorf("SplitWords: got %d words", len(ws))
	}
}

func TestABICallKAT(t *testing.T) {
	var a [20]byte
	for i := range a {
		a[i] = byte(i + 1)
	}
	data := EncodeCall(Selector("transfer(address,uint256)"), WordAddress(a), WordUint(big.NewInt(1)))
	want := "a9059cbb" +
		"0000000000000000000000000102030405060708090a0b0c0d0e0f1011121314" +
		"0000000000000000000000000000000000000000000000000000000000000001"
	if hex.EncodeToString(data) != want {
		t.Errorf("calldata = %s\nwant       %s", hex.EncodeToString(data), want)
	}
}
