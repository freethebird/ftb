package merkle

import (
	"math/big"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/util"
)

func leafFor(i int, amount int64) []byte {
	var u util.Uint160
	u[0] = byte(i + 1) // distinct, deterministic accounts
	return Leaf(u, big.NewInt(amount))
}

// Every leaf's proof must reproduce the root, for sizes 1..9 (covers odd and even layers).
func TestBuildAndVerifyAllSizes(t *testing.T) {
	for n := 1; n <= 9; n++ {
		leaves := make([][]byte, n)
		for i := 0; i < n; i++ {
			leaves[i] = leafFor(i, int64(100*(i+1)))
		}
		root, proofs := Build(leaves)
		if len(root) != 32 {
			t.Fatalf("n=%d: root len %d", n, len(root))
		}
		for i := 0; i < n; i++ {
			if !Verify(leaves[i], proofs[i], root) {
				t.Fatalf("n=%d: proof %d did not verify", n, i)
			}
		}
	}
}

// A wrong amount (hence wrong leaf) must NOT verify against the root.
func TestWrongLeafFails(t *testing.T) {
	leaves := [][]byte{leafFor(0, 100), leafFor(1, 200), leafFor(2, 300)}
	root, proofs := Build(leaves)
	bad := leafFor(0, 999) // same account, wrong amount
	if Verify(bad, proofs[0], root) {
		t.Fatal("a tampered amount verified; binding is broken")
	}
}

// Single-leaf tree: root == leaf, empty proof verifies.
func TestSingleLeaf(t *testing.T) {
	l := leafFor(0, 42)
	root, proofs := Build([][]byte{l})
	if !Verify(l, proofs[0], root) {
		t.Fatal("single-leaf proof failed")
	}
}
