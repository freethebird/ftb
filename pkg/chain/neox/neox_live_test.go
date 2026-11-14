//go:build neoxlive

package neox

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/freethebird/ftb/pkg/evm"
	"github.com/freethebird/ftb/pkg/fasp"
)

const (
	liveRPC      = "https://testnet.rpc.banelabs.org/"
	liveContract = "0x607FBFC1E22Dcbe152D6e8A7ffD7902f0DD7da85"
	liveHashHex  = "632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80"
	livePreimage = "d12e3e2e084e72846432ae802da90d6f13b92b6e80a9f66865393f835fc1fe9f"
)

func liveHash() [32]byte {
	b, _ := hex.DecodeString(liveHashHex)
	var h [32]byte
	copy(h[:], b)
	return h
}

func liveAdapter(t *testing.T) *Adapter {
	a, err := New(context.Background(), Config{RPCURL: liveRPC, Contract: liveContract})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestGetLive(t *testing.T) {
	rec, ok := liveAdapter(t).Get(liveHash())
	if !ok {
		t.Fatal("swap not found")
	}
	if rec.State != fasp.StateClaimed {
		t.Errorf("state = %d, want CLAIMED", rec.State)
	}
	t.Logf("live record: %+v", rec)
}

func TestPreimageLive(t *testing.T) {
	pre, ok := liveAdapter(t).Preimage(liveHash())
	if !ok {
		t.Fatal("preimage not found")
	}
	if hex.EncodeToString(pre[:]) != livePreimage {
		t.Errorf("preimage = %x, want %s", pre, livePreimage)
	}
}

// TestSigHashBindLive binds the EIP-1559 SigHash field order to a transaction the
// chain already accepted: it reads the real lock tx (via the Locked event),
// re-derives the signing hash with our encoder, and recovers the signer. A match
// proves the field order without sending anything.
func TestSigHashBindLive(t *testing.T) {
	a := liveAdapter(t)
	ctx := context.Background()
	lockedTopic := evm.Keccak256([]byte("Locked(bytes32,address,address,address,uint256,uint256)"))
	contract, _ := parseAddr(liveContract)
	topics := []any{"0x" + hex.EncodeToString(lockedTopic[:]), "0x" + liveHashHex}
	logs, err := a.rpc.Logs(ctx, contract, topics, "earliest", "latest")
	if err != nil || len(logs) == 0 {
		t.Fatalf("no Locked log: err=%v n=%d", err, len(logs))
	}
	info, err := a.rpc.TxByHash(ctx, logs[0].TxHash)
	if err != nil {
		t.Fatal(err)
	}
	tx := &evm.DynTx{
		ChainID: info.ChainID, Nonce: info.Nonce,
		GasTipCap: info.GasTipCap, GasFeeCap: info.GasFeeCap, Gas: info.Gas,
		To: info.To, Value: info.Value, Data: info.Data,
	}
	compact := make([]byte, 65)
	compact[0] = byte(27 + info.YParity)
	copy(compact[1+32-len(info.R):33], info.R)
	copy(compact[33+32-len(info.S):65], info.S)
	got, err := evm.RecoverSender(tx.SigHash(), compact)
	if err != nil {
		t.Fatal(err)
	}
	if got != info.From {
		t.Fatalf("recovered %x != tx from %x (SigHash field order mismatch)", got, info.From)
	}
	t.Logf("SigHash field order BOUND to on-chain lock tx 0x%x: recovered==from, method=%x", logs[0].TxHash, info.Data[:4])
}
