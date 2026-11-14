package evm

import "math/big"

// ABI encoding, the static-type subset the FtbHtlc calls need: the function
// selector and the head-only encoding of bytes32, address, and uint256 (each one
// 32-byte word; this contract's surface has no dynamic types). Exported for the
// Neo X adapter.

// Selector returns the 4-byte function selector keccak256(sig)[:4].
func Selector(sig string) [4]byte {
	h := Keccak256([]byte(sig))
	var s [4]byte
	copy(s[:], h[:4])
	return s
}

// WordBytes32 is the identity word for a bytes32 argument.
func WordBytes32(b [32]byte) [32]byte { return b }

// WordAddress left-pads a 20-byte address into a 32-byte ABI word.
func WordAddress(a [20]byte) [32]byte {
	var w [32]byte
	copy(w[12:], a[:])
	return w
}

// WordUint right-aligns the big-endian bytes of x into a 32-byte word; x is
// bounded to 256 bits by the caller.
func WordUint(x *big.Int) [32]byte {
	var w [32]byte
	b := x.Bytes()
	if len(b) > 32 {
		b = b[len(b)-32:]
	}
	copy(w[32-len(b):], b)
	return w
}

// EncodeCall builds calldata: the selector followed by the 32-byte words, in order.
func EncodeCall(sel [4]byte, words ...[32]byte) []byte {
	out := make([]byte, 0, 4+32*len(words))
	out = append(out, sel[:]...)
	for _, w := range words {
		out = append(out, w[:]...)
	}
	return out
}

// SplitWords splits a 32-byte-aligned ABI return into words; a trailing partial
// word (a malformed return) is dropped, and callers check the count they expect.
func SplitWords(data []byte) [][32]byte {
	n := len(data) / 32
	ws := make([][32]byte, n)
	for i := 0; i < n; i++ {
		copy(ws[i][:], data[i*32:])
	}
	return ws
}

// WordToAddress reads the low 20 bytes of a word as an address.
func WordToAddress(w [32]byte) [20]byte {
	var a [20]byte
	copy(a[:], w[12:])
	return a
}

// WordToUint reads a word as a big-endian unsigned integer.
func WordToUint(w [32]byte) *big.Int { return new(big.Int).SetBytes(w[:]) }

// WordToUint8 reads the low byte of a word as a uint8 (an enum or small int).
func WordToUint8(w [32]byte) uint8 { return w[31] }
