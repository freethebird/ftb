package neon3

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func TestParseU160(t *testing.T) {
	u, err := parseU160("0x3d7e6916972f550ba3405204d3f3bf328996d656")
	if err != nil {
		t.Fatal(err)
	}
	if u.StringLE() != "3d7e6916972f550ba3405204d3f3bf328996d656" {
		t.Errorf("StringLE = %s (want the 0x display hash as neo-go stores it)", u.StringLE())
	}
	if _, err := parseU160("NMK83aXpLED2xaWqFdUukNhiwihcurBsx9"); err != nil {
		t.Errorf("N3 address parse: %v", err)
	}
}

func TestClaimedPreimage(t *testing.T) {
	var contract util.Uint160
	contract[0] = 0xab
	var hl, pre [32]byte
	for i := range hl {
		hl[i] = byte(i)
		pre[i] = byte(255 - i)
	}
	ev := state.NotificationEvent{
		ScriptHash: contract,
		Name:       "Claimed",
		Item: stackitem.NewArray([]stackitem.Item{
			stackitem.NewByteArray(hl[:]),
			stackitem.NewByteArray(pre[:]),
		}),
	}
	execs := []state.Execution{{Events: []state.NotificationEvent{ev}}}
	got, ok := claimedPreimage(execs, contract, hl)
	if !ok || got != pre {
		t.Errorf("claimedPreimage ok=%v got=%x want %x", ok, got, pre)
	}
	var other [32]byte
	other[0] = 0x99
	if _, ok := claimedPreimage(execs, contract, other); ok {
		t.Error("should not match a different hashlock")
	}
}
