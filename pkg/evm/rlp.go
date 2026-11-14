package evm

import "math/big"

// RLP (Recursive Length Prefix) encoding, iterative (Article 9: no recursion).
// The list encoder takes already-encoded elements, so a nested structure is
// composed bottom-up by the caller rather than by self-recursion.

// rlpStr encodes a byte string.
func rlpStr(b []byte) []byte {
	if len(b) == 1 && b[0] < 0x80 {
		return []byte{b[0]}
	}
	if len(b) <= 55 {
		return append([]byte{byte(0x80 + len(b))}, b...)
	}
	lb := lenBytes(uint64(len(b)))
	out := append([]byte{byte(0xb7 + len(lb))}, lb...)
	return append(out, b...)
}

// rlpBig encodes a non-negative integer as the RLP of its minimal big-endian
// byte form (zero is the empty string, 0x80).
func rlpBig(x *big.Int) []byte {
	if x.Sign() < 0 {
		return []byte{0x80} // callers pass only non-negative values; fail to empty
	}
	return rlpStr(x.Bytes())
}

// rlpUint is rlpBig for a uint64.
func rlpUint(x uint64) []byte {
	return rlpBig(new(big.Int).SetUint64(x))
}

// rlpList wraps already-encoded elements in a list header.
func rlpList(items ...[]byte) []byte {
	var payload []byte
	for _, it := range items {
		payload = append(payload, it...)
	}
	if len(payload) <= 55 {
		return append([]byte{byte(0xc0 + len(payload))}, payload...)
	}
	lb := lenBytes(uint64(len(payload)))
	out := append([]byte{byte(0xf7 + len(lb))}, lb...)
	return append(out, payload...)
}

// lenBytes returns the minimal big-endian encoding of n (n > 0).
func lenBytes(n uint64) []byte {
	var b [8]byte
	i := 8
	for n > 0 {
		i--
		b[i] = byte(n)
		n >>= 8
	}
	return b[i:]
}
