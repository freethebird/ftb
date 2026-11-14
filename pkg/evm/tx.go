package evm

import (
	"errors"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// DynTx is an EIP-1559 (type 2) dynamic-fee transaction - the only form the Neo
// X HTLC adapter builds. Wei-scale fields are *big.Int (Book II rule 15.3), and
// the access list is always empty for the HTLC calls.
type DynTx struct {
	ChainID   *big.Int
	Nonce     uint64
	GasTipCap *big.Int // maxPriorityFeePerGas
	GasFeeCap *big.Int // maxFeePerGas
	Gas       uint64
	To        [20]byte
	Value     *big.Int
	Data      []byte
}

const dynTxType = 0x02

// signingFields returns the nine RLP-encoded fields common to the unsigned and
// signed forms of the transaction.
func (tx *DynTx) signingFields() [][]byte {
	return [][]byte{
		rlpBig(tx.ChainID),
		rlpUint(tx.Nonce),
		rlpBig(tx.GasTipCap),
		rlpBig(tx.GasFeeCap),
		rlpUint(tx.Gas),
		rlpStr(tx.To[:]),
		rlpBig(tx.Value),
		rlpStr(tx.Data),
		rlpList(), // empty access list
	}
}

// SigHash is Keccak-256 over the type-prefixed RLP of the unsigned fields.
func (tx *DynTx) SigHash() [32]byte {
	payload := append([]byte{dynTxType}, rlpList(tx.signingFields()...)...)
	return Keccak256(payload)
}

var errBadRecoveryCode = errors.New("evm: unexpected secp256k1 recovery code")

// SignRaw signs tx with priv and returns the raw type-2 transaction bytes for
// eth_sendRawTransaction plus the transaction hash. Signing is RFC6979
// deterministic and low-S (decred), so the output is reproducible.
func SignRaw(tx *DynTx, priv *secp256k1.PrivateKey) (raw []byte, txHash [32]byte, err error) {
	h := tx.SigHash()
	sig := ecdsa.SignCompact(priv, h[:], false) // [27+recid, R(32), S(32)]
	if len(sig) != 65 || sig[0] < 27 || sig[0] > 34 {
		return nil, [32]byte{}, errBadRecoveryCode
	}
	yParity := uint64(sig[0] - 27)
	fields := append(tx.signingFields(),
		rlpUint(yParity),
		rlpStr(trimLeadZero(sig[1:33])),
		rlpStr(trimLeadZero(sig[33:65])),
	)
	raw = append([]byte{dynTxType}, rlpList(fields...)...)
	return raw, Keccak256(raw), nil
}

// trimLeadZero strips leading zero bytes so r and s encode as RLP-minimal
// integers; a value is never all-zero in practice, but one byte is kept if so.
func trimLeadZero(b []byte) []byte {
	i := 0
	for i < len(b)-1 && b[i] == 0 {
		i++
	}
	return b[i:]
}

// AddressFromPriv derives the 20-byte EVM address of priv's public key.
func AddressFromPriv(priv *secp256k1.PrivateKey) [20]byte {
	return AddressFromPub(priv.PubKey())
}

// AddressFromPub derives the 20-byte EVM address: keccak256(uncompressed[1:])[12:].
func AddressFromPub(pub *secp256k1.PublicKey) [20]byte {
	u := pub.SerializeUncompressed() // 65 bytes: 0x04 || X || Y
	h := Keccak256(u[1:])
	var a [20]byte
	copy(a[:], h[12:])
	return a
}

// RecoverSender recovers the signer address from a sighash and a 65-byte
// decred-compact signature; used by the round-trip KAT.
func RecoverSender(sigHash [32]byte, compact []byte) ([20]byte, error) {
	pub, _, err := ecdsa.RecoverCompact(compact, sigHash[:])
	if err != nil {
		return [20]byte{}, err
	}
	return AddressFromPub(pub), nil
}
