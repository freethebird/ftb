package evm

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestRLPString(t *testing.T) {
	cases := []struct {
		in   []byte
		want string
	}{
		{nil, "80"},
		{[]byte("dog"), "83646f67"},
		{[]byte{0x00}, "00"},
		{[]byte{0x0f}, "0f"},
		{[]byte{0x04, 0x00}, "820400"},
		{bytes.Repeat([]byte{0x61}, 56), "b838" + hex.EncodeToString(bytes.Repeat([]byte{0x61}, 56))},
	}
	for _, c := range cases {
		if got := hex.EncodeToString(rlpStr(c.in)); got != c.want {
			t.Errorf("rlpStr(%x) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestRLPList(t *testing.T) {
	if got := hex.EncodeToString(rlpList()); got != "c0" {
		t.Errorf("empty list = %s, want c0", got)
	}
	got := hex.EncodeToString(rlpList(rlpStr([]byte("cat")), rlpStr([]byte("dog"))))
	if got != "c88363617483646f67" {
		t.Errorf("[cat,dog] = %s, want c88363617483646f67", got)
	}
}

func TestRLPUint(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "80"},
		{15, "0f"},
		{0x7f, "7f"},
		{0x80, "8180"},
		{1024, "820400"},
	}
	for _, c := range cases {
		if got := hex.EncodeToString(rlpUint(c.in)); got != c.want {
			t.Errorf("rlpUint(%d) = %s, want %s", c.in, got, c.want)
		}
	}
}
