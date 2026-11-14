package neox

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/freethebird/ftb/pkg/fasp"
)

const liveSwapsHex = "00000000000000000000000073f67e08d59c6c7b062e45ff0c9b121543388886000000000000000000000000000000000000000000000000000000000000000200000000000000000000000073f67e08d59c6c7b062e45ff0c9b121543388886000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000038d7ea4c68000000000000000000000000000000000000000000000000000000000006af5c342"

// TestDecodeSwapKAT binds decodeSwap to the deployed bytecode via the real
// swaps(632625...) return for the live Neo X demo swap.
func TestDecodeSwapKAT(t *testing.T) {
	ret, err := hex.DecodeString(liveSwapsHex)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := decodeSwap(ret)
	if err != nil || !ok {
		t.Fatalf("decodeSwap: ok=%v err=%v", ok, err)
	}
	if rec.State != fasp.StateClaimed {
		t.Errorf("state = %d, want CLAIMED(2)", rec.State)
	}
	if rec.Token != "0x0000000000000000000000000000000000000000" {
		t.Errorf("token = %s, want native zero", rec.Token)
	}
	if rec.Amount == nil || rec.Amount.Cmp(big.NewInt(1000000000000000)) != 0 {
		t.Errorf("amount = %v, want 1e15 wei (0.001 GAS)", rec.Amount)
	}
	if rec.Sender != "0x73f67e08d59c6c7b062e45ff0c9b121543388886" || rec.Sender != rec.Recipient {
		t.Errorf("sender/recipient = %s / %s", rec.Sender, rec.Recipient)
	}
}

func TestParseAddr(t *testing.T) {
	a, err := parseAddr("0x607FBFC1E22Dcbe152D6e8A7ffD7902f0DD7da85")
	if err != nil {
		t.Fatal(err)
	}
	if addrStr(a) != "0x607fbfc1e22dcbe152d6e8a7ffd7902f0dd7da85" {
		t.Errorf("round-trip = %s", addrStr(a))
	}
	if _, err := parseAddr("0x1234"); err == nil {
		t.Error("expected length error")
	}
}
