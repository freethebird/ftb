package evm

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

func privFromHex(h string) *secp256k1.PrivateKey {
	b, _ := hex.DecodeString(h)
	return secp256k1.PrivKeyFromBytes(b)
}

// TestAddressKAT pins address derivation on the canonical "private key = 1" vector.
func TestAddressKAT(t *testing.T) {
	priv := privFromHex("0000000000000000000000000000000000000000000000000000000000000001")
	got := AddressFromPriv(priv)
	want := "7e5f4552091a69125d5dfcb7b8c2659029395bdf"
	if hex.EncodeToString(got[:]) != want {
		t.Errorf("address = %x, want %s", got, want)
	}
}

// TestSignRecover proves sign -> recover -> address over the real tx sighash.
func TestSignRecover(t *testing.T) {
	priv := privFromHex("4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318")
	signer := AddressFromPriv(priv)
	tx := &DynTx{
		ChainID: big.NewInt(12227332), Nonce: 7,
		GasTipCap: big.NewInt(25_000_000_000), GasFeeCap: big.NewInt(60_000_000_000),
		Gas: 120000, To: [20]byte{0x60, 0x7f, 0xbf, 0xc1}, Value: big.NewInt(1000),
		Data: []byte{0x84, 0xcc, 0x9d, 0xfb},
	}
	h := tx.SigHash()
	sig := ecdsa.SignCompact(priv, h[:], false)
	got, err := RecoverSender(h, sig)
	if err != nil {
		t.Fatal(err)
	}
	if got != signer {
		t.Errorf("recovered %x, want %x", got, signer)
	}
}

// TestSignRawDeterministic: RFC6979 signing is reproducible; the raw tx is a
// well-formed type-2 envelope.
func TestSignRawDeterministic(t *testing.T) {
	priv := privFromHex("4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318")
	tx := &DynTx{
		ChainID: big.NewInt(12227332), Nonce: 1,
		GasTipCap: big.NewInt(20_000_000_000), GasFeeCap: big.NewInt(50_000_000_000),
		Gas: 90000, To: [20]byte{0xab}, Value: big.NewInt(0), Data: nil,
	}
	raw1, hash1, err := SignRaw(tx, priv)
	if err != nil {
		t.Fatal(err)
	}
	raw2, hash2, _ := SignRaw(tx, priv)
	if hex.EncodeToString(raw1) != hex.EncodeToString(raw2) || hash1 != hash2 {
		t.Error("signing is not deterministic")
	}
	if len(raw1) == 0 || raw1[0] != dynTxType {
		t.Error("raw tx does not start with type 0x02")
	}
}
