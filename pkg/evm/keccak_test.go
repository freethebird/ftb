package evm

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestKeccak256KAT pins the hash to canonical Keccak-256 vectors (the same the C
// accel-rail satisfies). Empty exercises the all-padding block; "abc" and the fox
// a partial block.
func TestKeccak256KAT(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		{"abc", "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"},
		{"The quick brown fox jumps over the lazy dog", "4d741b6f1eb29cb2a9b9911c82f56fa8d73b04959d3d9d222895df6c0b28aa15"},
	}
	for _, c := range cases {
		got := Keccak256([]byte(c.in))
		if hex.EncodeToString(got[:]) != c.want {
			t.Errorf("Keccak256(%q) = %x, want %s", c.in, got, c.want)
		}
	}
}

// TestKeccak256BlockBoundary exercises the multi-block absorb loop at and around
// the 136-byte rate; a structural guard pending a value KAT bound to the rail.
func TestKeccak256BlockBoundary(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range []int{135, 136, 137, 272} {
		h := Keccak256(bytes.Repeat([]byte("a"), n))
		if len(h) != 32 {
			t.Fatalf("len=%d", len(h))
		}
		s := hex.EncodeToString(h[:])
		if seen[s] {
			t.Errorf("collision at n=%d", n)
		}
		seen[s] = true
	}
}
