// Package evm implements the minimal Ethereum wire primitives the Neo X HTLC
// adapter needs - Keccak-256, RLP, an ABI subset, EIP-1559 transaction signing,
// and a JSON-RPC client - in pure Go: the standard library plus the secp256k1
// already compiled inside the one pinned neo-go dependency's closure. No cgo, no
// further module (Go Bible Book III).
//
// Keccak is hand-rolled here because the standard library's crypto/sha3 is
// NIST-padded SHA3, not Ethereum's legacy Keccak (0x01 domain). Correctness is
// pinned by the canonical KATs in keccak_test.go - the same vectors the C
// accel-rail's Keccak satisfies - so the two agree byte-for-byte without any
// language boundary. Iterative throughout (Article 9: no recursion).
package evm

import (
	"encoding/binary"
	"math/bits"
)

var keccakRC = [24]uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808a, 0x8000000080008000,
	0x000000000000808b, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
	0x000000000000008a, 0x0000000000000088, 0x0000000080008009, 0x000000008000000a,
	0x000000008000808b, 0x800000000000008b, 0x8000000000008089, 0x8000000000008003,
	0x8000000000008002, 0x8000000000000080, 0x000000000000800a, 0x800000008000000a,
	0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
}

var keccakRot = [24]uint{1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14, 27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44}

var keccakPiln = [24]int{10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4, 15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1}

// keccakF1600 applies the 24-round Keccak-f[1600] permutation in place.
func keccakF1600(a *[25]uint64) {
	var bc [5]uint64
	for round := 0; round < 24; round++ {
		// Theta.
		for i := 0; i < 5; i++ {
			bc[i] = a[i] ^ a[i+5] ^ a[i+10] ^ a[i+15] ^ a[i+20]
		}
		for i := 0; i < 5; i++ {
			t := bc[(i+4)%5] ^ bits.RotateLeft64(bc[(i+1)%5], 1)
			for j := 0; j < 25; j += 5 {
				a[j+i] ^= t
			}
		}
		// Rho and Pi.
		t := a[1]
		for i := 0; i < 24; i++ {
			j := keccakPiln[i]
			c := a[j]
			a[j] = bits.RotateLeft64(t, int(keccakRot[i]))
			t = c
		}
		// Chi.
		for j := 0; j < 25; j += 5 {
			for i := 0; i < 5; i++ {
				bc[i] = a[j+i]
			}
			for i := 0; i < 5; i++ {
				a[j+i] ^= (^bc[(i+1)%5]) & bc[(i+2)%5]
			}
		}
		// Iota.
		a[0] ^= keccakRC[round]
	}
}

// keccakRate is the byte rate for Keccak-256 (1088-bit rate, 512-bit capacity).
const keccakRate = 136

func keccakAbsorb(st *[25]uint64, block []byte) {
	for k := 0; k < keccakRate/8; k++ {
		st[k] ^= binary.LittleEndian.Uint64(block[k*8:])
	}
}

// Keccak256 returns the Ethereum Keccak-256 (legacy 0x01 domain) digest of in.
func Keccak256(in []byte) [32]byte {
	var st [25]uint64
	i := 0
	for ; i+keccakRate <= len(in); i += keccakRate {
		keccakAbsorb(&st, in[i:i+keccakRate])
		keccakF1600(&st)
	}
	var block [keccakRate]byte
	rem := copy(block[:], in[i:])
	block[rem] = 0x01
	block[keccakRate-1] |= 0x80
	keccakAbsorb(&st, block[:])
	keccakF1600(&st)
	var out [32]byte
	for k := 0; k < 4; k++ {
		binary.LittleEndian.PutUint64(out[k*8:], st[k])
	}
	return out
}
