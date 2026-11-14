// Unit suite for the FASP client safety core: the timelock-ladder check (including the theft
// scenario the ladder exists to refuse), the minimal-timelock computation, the native-unit
// converters, and the preimage/hashlock helpers (tied to the differential-KAT vectors so the
// client hash matches the contracts). Run: go test ./pkg/fasp/
package fasp

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func n3() Chain {
	return Chain{Name: "NeoN3", Kind: Height, BlockTime: 15 * time.Second, Confs: 1, ClaimBudget: 30 * time.Second}
}

func neox() Chain {
	return Chain{Name: "NeoX", Kind: Timestamp, BlockTime: 1 * time.Second, Confs: 12, ClaimBudget: 30 * time.Second}
}

// User holds A on N3, wants B on Neo X: user locks on N3 (T_A), solver locks on Neo X (T_B).
func legs(userT, solverT time.Duration) (Leg, Leg) {
	return Leg{Chain: n3(), Timelock: userT}, Leg{Chain: neox(), Timelock: solverT}
}

func TestComputeTimelocksSatisfiesValidate(t *testing.T) {
	for _, m := range []int{2, 3, 5, 10} {
		userT, solverT, err := ComputeTimelocks(n3(), neox(), m)
		if err != nil {
			t.Fatalf("m=%d: ComputeTimelocks: %v", m, err)
		}
		u, s := legs(userT, solverT)
		if err := Validate(u, s, m); err != nil {
			t.Fatalf("m=%d: computed timelocks rejected by Validate: %v (T_A=%s T_B=%s)", m, err, userT, solverT)
		}
		// The minimum is exactly safe: shaving either leg by a nanosecond must be refused.
		if err := Validate(Leg{n3(), userT - 1}, s, m); err == nil {
			t.Fatalf("m=%d: T_A minus 1ns was accepted", m)
		}
		if err := Validate(u, Leg{neox(), solverT - 1}, m); err == nil {
			t.Fatalf("m=%d: T_B minus 1ns was accepted", m)
		}
	}
}

func TestValidateHappy(t *testing.T) {
	// T_B = 60s (>= Neo X window 54s at m=2); T_A = 60s + N3 window 60s = 120s.
	u, s := legs(120*time.Second, 60*time.Second)
	if err := Validate(u, s, 2); err != nil {
		t.Fatalf("expected safe, got %v", err)
	}
}

func TestValidateRefusals(t *testing.T) {
	cases := []struct {
		name           string
		userT, solverT time.Duration
		m              int
		want           error
	}{
		{"multiplier too low", 120 * time.Second, 60 * time.Second, 1, ErrMultiplier},
		{"solver leg too short", 120 * time.Second, 50 * time.Second, 2, ErrShortClaimWindow},
		{"equal timelocks (theft)", 60 * time.Second, 60 * time.Second, 2, ErrLadder},
		{"T_A one ns short of the ladder", 120*time.Second - 1, 60 * time.Second, 2, ErrLadder},
		{"T_A shorter than T_B", 50 * time.Second, 60 * time.Second, 2, ErrLadder},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, s := legs(c.userT, c.solverT)
			err := Validate(u, s, c.m)
			if !errors.Is(err, c.want) {
				t.Fatalf("Validate = %v, want %v", err, c.want)
			}
		})
	}
}

// The core anti-theft property, stated as its own test: for a swept range of parameters, the
// naive "equal timelocks" choice is always refused.
func TestEqualTimelocksAlwaysRefused(t *testing.T) {
	for _, T := range []time.Duration{time.Minute, time.Hour, 24 * time.Hour} {
		for _, m := range []int{2, 4, 8} {
			u, s := legs(T, T)
			if err := Validate(u, s, m); !errors.Is(err, ErrLadder) {
				// If T is below the solver claim window it fails earlier (also a refusal), which is fine.
				if !errors.Is(err, ErrShortClaimWindow) {
					t.Fatalf("T=%s m=%d: equal timelocks not refused: %v", T, m, err)
				}
			}
		}
	}
}

func TestTimelockConverters(t *testing.T) {
	// Height chain (N3): 114s at 15s blocks rounds up to 8 blocks.
	h, err := n3().TimelockHeight(1000, 114*time.Second)
	if err != nil {
		t.Fatalf("TimelockHeight: %v", err)
	}
	if h != 1008 {
		t.Fatalf("TimelockHeight = %d, want 1008", h)
	}
	// Exactly on a block boundary does not round up past it.
	h2, _ := n3().TimelockHeight(0, 30*time.Second)
	if h2 != 2 {
		t.Fatalf("TimelockHeight(30s) = %d, want 2", h2)
	}
	// Timestamp chain (Neo X): inclusion + duration.
	inc := time.Unix(1_700_000_000, 0)
	u, err := neox().TimelockUnix(inc, 54*time.Second)
	if err != nil {
		t.Fatalf("TimelockUnix: %v", err)
	}
	if u != 1_700_000_054 {
		t.Fatalf("TimelockUnix = %d, want 1700000054", u)
	}
	// Wrong-kind conversions are refused.
	if _, err := n3().TimelockUnix(inc, time.Second); err == nil {
		t.Fatal("N3 (height) accepted a unix conversion")
	}
	if _, err := neox().TimelockHeight(0, time.Second); err == nil {
		t.Fatal("Neo X (timestamp) accepted a height conversion")
	}
}

func mustHex32(t *testing.T, s string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	var a [32]byte
	copy(a[:], b)
	return a
}

func TestHashlockMatchesKAT(t *testing.T) {
	// The first claim vector from contracts/ftb_htlc/kat/vectors.json: Hashlock(preimage) must
	// equal the shared hashlock the contracts check, so the client, the N3 leg, and the Neo X leg
	// all agree on the commitment.
	preimage := mustHex32(t, "d12e3e2e084e72846432ae802da90d6f13b92b6e80a9f66865393f835fc1fe9f")
	hashlock := mustHex32(t, "632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80")
	if got := Hashlock(preimage); got != hashlock {
		t.Fatalf("Hashlock(preimage) = %x, want %x", got, hashlock)
	}
	if !VerifyPreimage(preimage, hashlock) {
		t.Fatal("VerifyPreimage rejected the correct preimage")
	}
	wrong := mustHex32(t, "4f50d616a506eb9899d08a9bdd858e39209ddc586164dd0f25ff9e4156d9133f")
	if VerifyPreimage(wrong, hashlock) {
		t.Fatal("VerifyPreimage accepted a wrong preimage")
	}
}

func TestNewSecretDistinctAndNonZero(t *testing.T) {
	var zero [32]byte
	a, err := NewSecret()
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	b, err := NewSecret()
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	if a == zero || b == zero {
		t.Fatal("NewSecret returned the zero secret")
	}
	if a == b {
		t.Fatal("NewSecret returned the same secret twice")
	}
}
