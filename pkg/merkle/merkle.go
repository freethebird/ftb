// Package merkle builds the snapshot Merkle tree for the FTB migration. It is the single
// source of truth for the leaf format and is imported by BOTH the merkleroot CLI and the
// migrator's neotest, so the on-chain Claim and the off-chain builder can never drift.
//
// Leaf format, matching ftb_migrator.Claim EXACTLY:
//
//	leaf = sha256( 0x00 ++ accountBytes(20) ++ ':' ++ decimal(amount) )
//	node = sha256( 0x01 ++ min(l,r) ++ max(l,r) )   sorted-pair, domain-separated
//
// Sibling hashing is sorted-pair (smaller 32-byte hash first). Odd nodes carry up.
package merkle

import (
	"bytes"
	"crypto/sha256"
	"math/big"

	"github.com/nspcc-dev/neo-go/pkg/util"
)

// Domain separation tags (audit item FTB-11): leaves and internal nodes are hashed with distinct
// prefixes so a leaf hash can never be reinterpreted as an internal node.
const (
	leafTag = 0x00
	nodeTag = 0x01
)

// accountBytes returns the 20 bytes the contract receives as its Hash160 parameter.
//
// NOTE: if the migrator claim test fails with "bad proof", this byte order is the one
// thing to flip. Change BytesBE to BytesLE here (one place) and the CLI and the tests
// both follow, because they all call this function.
func accountBytes(a util.Uint160) []byte {
	return a.BytesBE()
}

// Leaf computes the leaf hash for one (account, amount) entitlement.
func Leaf(account util.Uint160, amount *big.Int) []byte {
	b := []byte{leafTag}
	b = append(b, accountBytes(account)...)
	b = append(b, ':')
	b = append(b, []byte(amount.String())...)
	h := sha256.Sum256(b)
	return h[:]
}

func hashPair(a, b []byte) []byte {
	var h [32]byte
	if bytes.Compare(a, b) < 0 {
		h = sha256.Sum256(append(append([]byte{nodeTag}, a...), b...))
	} else {
		h = sha256.Sum256(append(append([]byte{nodeTag}, b...), a...))
	}
	return h[:]
}

// Build returns the Merkle root and, for each input leaf (by index), its proof: the
// ordered list of sibling hashes from the leaf up to the root.
func Build(leaves [][]byte) (root []byte, proofs [][][]byte) {
	n := len(leaves)
	if n == 0 {
		return nil, nil
	}
	proofs = make([][][]byte, n)
	for i := range proofs {
		proofs[i] = [][]byte{}
	}
	layer := make([][]byte, n)
	copy(layer, leaves)
	under := make([][]int, n) // original leaf indices beneath each current node
	for i := range under {
		under[i] = []int{i}
	}
	for len(layer) > 1 {
		var next [][]byte
		var nextUnder [][]int
		for i := 0; i < len(layer); i += 2 {
			if i+1 < len(layer) {
				a, b := layer[i], layer[i+1]
				for _, li := range under[i] {
					proofs[li] = append(proofs[li], b)
				}
				for _, li := range under[i+1] {
					proofs[li] = append(proofs[li], a)
				}
				next = append(next, hashPair(a, b))
				nextUnder = append(nextUnder, append(append([]int{}, under[i]...), under[i+1]...))
			} else {
				next = append(next, layer[i]) // odd node carries up, no sibling
				nextUnder = append(nextUnder, under[i])
			}
		}
		layer = next
		under = nextUnder
	}
	return layer[0], proofs
}

// Verify mirrors the on-chain Claim loop: fold the proof into the leaf with sorted-pair
// hashing and check it reproduces the root. Used by the pure-Go tests.
func Verify(leaf []byte, proof [][]byte, root []byte) bool {
	node := leaf
	for _, sib := range proof {
		node = hashPair(node, sib)
	}
	return bytes.Equal(node, root)
}
