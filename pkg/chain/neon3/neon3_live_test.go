//go:build n3live

package neon3

import (
	"context"
	"encoding/hex"
	"os"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/util"

	"github.com/freethebird/ftb/pkg/fasp"
)

const (
	liveRPC      = "https://testnet1.neo.coz.io:443"
	liveContract = "0x3d7e6916972f550ba3405204d3f3bf328996d656"
	liveHashHex  = "632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80"
	liveClaimTx  = "67b2114309fc4fa09a2a4a8a7d073952f1bc180f98b19bef4385b3c5393faea5"
	livePreimage = "d12e3e2e084e72846432ae802da90d6f13b92b6e80a9f66865393f835fc1fe9f"
)

func liveHash() [32]byte {
	b, _ := hex.DecodeString(liveHashHex)
	var h [32]byte
	copy(h[:], b)
	return h
}

func liveRPCURL() string {
	if v := os.Getenv("FTB_N3_RPC"); v != "" {
		return v
	}
	return liveRPC
}

func liveAdapter(t *testing.T) *Adapter {
	a, err := New(context.Background(), Config{RPCURL: liveRPCURL(), Contract: liveContract})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestGetLive binds the read path (invoke swap, decode) to the deployed contract.
func TestGetLive(t *testing.T) {
	rec, ok := liveAdapter(t).Get(liveHash())
	if !ok {
		t.Fatal("swap not found")
	}
	if rec.State != fasp.StateClaimed {
		t.Errorf("state = %d, want CLAIMED", rec.State)
	}
	t.Logf("live N3 record: %+v", rec)
}

// TestClaimedParseLive binds the Claimed-notification parse to the real claim tx.
func TestClaimedParseLive(t *testing.T) {
	a := liveAdapter(t)
	txid, err := util.Uint256DecodeStringLE(liveClaimTx)
	if err != nil {
		t.Fatal(err)
	}
	aer, err := a.cli.GetApplicationLog(txid, nil)
	if err != nil {
		t.Fatal(err)
	}
	pre, ok := claimedPreimage(aer.Executions, a.contract, liveHash())
	if !ok {
		t.Fatal("Claimed not found in claim tx")
	}
	if hex.EncodeToString(pre[:]) != livePreimage {
		t.Errorf("preimage = %x, want %s", pre, livePreimage)
	}
}
