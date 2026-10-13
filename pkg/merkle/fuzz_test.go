package merkle

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// FuzzBuildVerify is the Article 5 fuzz gate for the Merkle builder: for arbitrary
// content and tree sizes 1..7, EVERY emitted proof must reproduce the root, and a leaf
// that is not in the tree must NOT verify. The on-chain Claim runs the same fold, so a
// panic or a false accept here would be a consensus-layer bug.
func FuzzBuildVerify(f *testing.F) {
	f.Add([]byte{7}, []byte("alice"), []byte("bob"), []byte("carol"))
	f.Add([]byte{0}, []byte{}, []byte{0xff}, []byte("x"))
	f.Fuzz(func(t *testing.T, seed, x, y, z []byte) {
		if len(seed) == 0 || len(seed) > 1024 || len(x) > 1024 || len(y) > 1024 || len(z) > 1024 {
			t.Skip()
		}
		n := int(seed[0])%7 + 1 // 1..7 leaves: odd and even layers, carry-up included
		src := [][]byte{x, y, z}
		leaves := make([][]byte, n)
		for i := 0; i < n; i++ {
			h := sha256.Sum256(append(append([]byte{}, seed...), src[i%len(src)]...))
			leaves[i] = h[:]
		}
		root, proofs := Build(leaves)
		if len(root) != 32 {
			t.Fatalf("root length %d", len(root))
		}
		if len(proofs) != n {
			t.Fatalf("got %d proofs for %d leaves", len(proofs), n)
		}
		for i := 0; i < n; i++ {
			if !Verify(leaves[i], proofs[i], root) {
				t.Fatalf("n=%d: proof %d does not verify", n, i)
			}
		}
		// A leaf that is not in the tree must never verify against the root.
		stranger := sha256.Sum256([]byte("not in this tree"))
		inTree := false
		for _, l := range leaves {
			if bytes.Equal(l, stranger[:]) {
				inTree = true
			}
		}
		if !inTree && Verify(stranger[:], proofs[0], root) {
			t.Fatal("a foreign leaf verified; the binding is broken")
		}
	})
}

// FuzzHashPairCommutative pins the sorted-pair rule: node hashing must be
// order-independent, matching the on-chain lessBytes fold exactly.
func FuzzHashPairCommutative(f *testing.F) {
	f.Add([]byte("alpha-32-byte-pad..............."), []byte("omega-32-byte-pad..............."))
	f.Fuzz(func(t *testing.T, a, b []byte) {
		if len(a) != 32 || len(b) != 32 {
			t.Skip()
		}
		if !bytes.Equal(hashPair(a, b), hashPair(b, a)) {
			t.Fatal("hashPair is not commutative; sorted-pair order violated")
		}
	})
}
