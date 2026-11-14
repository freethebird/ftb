package fasp

import (
	"errors"
	"math/big"
	"testing"
)

// mockChain is an in-memory FtbHtlc that mirrors the contract semantics: escrow-on-lock,
// claim-with-preimage, refund-after-timelock, one swap per hashlock, permissionless claim/refund.
// Balances and amounts are *big.Int (full uint256), matching the fasp.HTLC interface.
// `now` is the chain clock the test advances to cross a timelock.
type mockChain struct {
	name      string
	now       int64
	swaps     map[[32]byte]SwapRecord
	preimages map[[32]byte][32]byte
	bal       map[string]*big.Int
}

func newMockChain(name string, now int64) *mockChain {
	return &mockChain{
		name:      name,
		now:       now,
		swaps:     map[[32]byte]SwapRecord{},
		preimages: map[[32]byte][32]byte{},
		bal:       map[string]*big.Int{},
	}
}

func bi(n int64) *big.Int { return big.NewInt(n) }

func bkey(party, token string) string { return party + "|" + token }

func (m *mockChain) balanceOf(p, tok string) *big.Int {
	if v, ok := m.bal[bkey(p, tok)]; ok {
		return v
	}
	return big.NewInt(0)
}

func (m *mockChain) credit(p, tok string, a *big.Int) {
	k := bkey(p, tok)
	m.bal[k] = new(big.Int).Add(m.balanceOf(p, tok), a)
}

func (m *mockChain) Name() string { return m.name }

func (m *mockChain) Lock(from, recipient, token string, amount *big.Int, hashlock [32]byte, timelock int64) error {
	if _, exists := m.swaps[hashlock]; exists {
		return errors.New("hashlock used")
	}
	if amount == nil || amount.Sign() <= 0 {
		return errors.New("amount must be positive")
	}
	if timelock <= m.now {
		return errors.New("timelock in past")
	}
	if m.balanceOf(from, token).Cmp(amount) < 0 {
		return errors.New("insufficient balance")
	}
	m.bal[bkey(from, token)] = new(big.Int).Sub(m.balanceOf(from, token), amount)
	m.swaps[hashlock] = SwapRecord{
		Sender: from, Recipient: recipient, Token: token, Amount: new(big.Int).Set(amount), Timelock: timelock, State: StateLocked,
	}
	return nil
}

func (m *mockChain) Get(hashlock [32]byte) (SwapRecord, bool) {
	r, ok := m.swaps[hashlock]
	return r, ok
}

func (m *mockChain) Claim(caller string, hashlock, preimage [32]byte) error {
	r, ok := m.swaps[hashlock]
	if !ok || r.State != StateLocked {
		return errors.New("not locked")
	}
	if Hashlock(preimage) != hashlock {
		return errors.New("preimage mismatch")
	}
	r.State = StateClaimed
	m.swaps[hashlock] = r
	m.preimages[hashlock] = preimage
	m.credit(r.Recipient, r.Token, r.Amount)
	return nil
}

func (m *mockChain) Refund(caller string, hashlock [32]byte) error {
	r, ok := m.swaps[hashlock]
	if !ok || r.State != StateLocked {
		return errors.New("not locked")
	}
	if m.now < r.Timelock {
		return errors.New("timelock not reached")
	}
	r.State = StateRefunded
	m.swaps[hashlock] = r
	m.credit(r.Sender, r.Token, r.Amount)
	return nil
}

func (m *mockChain) Preimage(hashlock [32]byte) ([32]byte, bool) {
	p, ok := m.preimages[hashlock]
	return p, ok
}

// fixtures returns two funded chains (user holds A on chainA, solver holds B on chainB) and the
// swap secret. now=100, T_B=150, T_A=200 (T_A > T_B).
func fixtures() (a, b *mockChain, secret [32]byte) {
	a = newMockChain("A", 100)
	b = newMockChain("B", 100)
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	a.credit("user", "FTB", bi(1000))
	b.credit("solver", "FTBx", bi(1000))
	return a, b, secret
}

func newSwap(a, b *mockChain, secret [32]byte) *Swap {
	return &Swap{
		ChainA: a, ChainB: b,
		UserA: "user", UserB: "user", SolverA: "solver", SolverB: "solver",
		TokenA: "FTB", TokenB: "FTBx",
		AmountA: bi(1000), AmountB: bi(1000),
		TimelockA: 200, TimelockB: 150,
		Hashlock: Hashlock(secret),
	}
}

func TestOrchestrationHappyPath(t *testing.T) {
	a, b, secret := fixtures()
	sw := newSwap(a, b, secret)

	must(t, sw.SolverLockB(), "solver lock B")
	must(t, sw.UserVerifyAndLockA(), "user verify + lock A")
	must(t, sw.UserClaimB(secret), "user claim B")
	must(t, sw.SolverClaimA(), "solver claim A")

	if got := b.balanceOf("user", "FTBx"); got.Cmp(bi(1000)) != 0 {
		t.Fatalf("user B balance = %d, want 1000", got)
	}
	if got := a.balanceOf("solver", "FTB"); got.Cmp(bi(1000)) != 0 {
		t.Fatalf("solver A balance = %d, want 1000", got)
	}
	if r, _ := b.Get(sw.Hashlock); r.State != StateClaimed {
		t.Fatalf("B state = %d, want CLAIMED", r.State)
	}
	if r, _ := a.Get(sw.Hashlock); r.State != StateClaimed {
		t.Fatalf("A state = %d, want CLAIMED", r.State)
	}
	// Conservation: nothing left with the original holders.
	if got := b.balanceOf("solver", "FTBx"); got.Sign() != 0 {
		t.Fatalf("solver still holds %d B", got)
	}
	if got := a.balanceOf("user", "FTB"); got.Sign() != 0 {
		t.Fatalf("user still holds %d A", got)
	}
}

func TestUserRefusesBadSolverLock(t *testing.T) {
	cases := []struct {
		name  string
		setup func(b *mockChain, hl [32]byte)
		want  error
	}{
		{"no lock", func(b *mockChain, hl [32]byte) {}, ErrNoLock},
		{"wrong recipient", func(b *mockChain, hl [32]byte) {
			_ = b.Lock("solver", "attacker", "FTBx", bi(1000), hl, 150)
		}, ErrSpec},
		{"wrong token", func(b *mockChain, hl [32]byte) {
			b.credit("solver", "WRONG", bi(1000))
			_ = b.Lock("solver", "user", "WRONG", bi(1000), hl, 150)
		}, ErrSpec},
		{"short amount", func(b *mockChain, hl [32]byte) {
			_ = b.Lock("solver", "user", "FTBx", bi(500), hl, 150)
		}, ErrSpec},
		{"wrong timelock", func(b *mockChain, hl [32]byte) {
			_ = b.Lock("solver", "user", "FTBx", bi(1000), hl, 999)
		}, ErrSpec},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b, secret := fixtures()
			sw := newSwap(a, b, secret)
			c.setup(b, sw.Hashlock)
			err := sw.UserVerifyAndLockA()
			if !errors.Is(err, c.want) {
				t.Fatalf("UserVerifyAndLockA = %v, want %v", err, c.want)
			}
			if _, ok := a.Get(sw.Hashlock); ok {
				t.Fatal("user locked A despite a bad solver lock (funds at risk)")
			}
			if got := a.balanceOf("user", "FTB"); got.Cmp(bi(1000)) != 0 {
				t.Fatalf("user A balance = %d, want 1000 (untouched)", got)
			}
		})
	}
}

func TestUserNeverLocksSolverRefunds(t *testing.T) {
	a, b, secret := fixtures()
	sw := newSwap(a, b, secret)
	must(t, sw.SolverLockB(), "solver lock B")

	// User never locks or claims; before T_B the solver cannot refund.
	if err := sw.SolverRefundB(); err == nil {
		t.Fatal("solver refunded B before T_B")
	}
	b.now = 150 // reach T_B
	must(t, sw.SolverRefundB(), "solver refund B")
	if got := b.balanceOf("solver", "FTBx"); got.Cmp(bi(1000)) != 0 {
		t.Fatalf("solver B balance = %d, want 1000 (refunded)", got)
	}
}

func TestUserNeverClaimsBothRefund(t *testing.T) {
	a, b, secret := fixtures()
	sw := newSwap(a, b, secret)
	must(t, sw.SolverLockB(), "solver lock B")
	must(t, sw.UserVerifyAndLockA(), "user verify + lock A")

	// Nobody claims; both refund after their own timelocks.
	b.now = 150
	a.now = 200
	must(t, sw.SolverRefundB(), "solver refund B")
	must(t, sw.UserRefundA(), "user refund A")

	if got := b.balanceOf("solver", "FTBx"); got.Cmp(bi(1000)) != 0 {
		t.Fatalf("solver B balance = %d, want 1000", got)
	}
	if got := a.balanceOf("user", "FTB"); got.Cmp(bi(1000)) != 0 {
		t.Fatalf("user A balance = %d, want 1000", got)
	}
}

func TestSolverClaimOrdering(t *testing.T) {
	a, b, secret := fixtures()
	sw := newSwap(a, b, secret)
	must(t, sw.SolverLockB(), "solver lock B")
	must(t, sw.UserVerifyAndLockA(), "user verify + lock A")

	// Before the user reveals the secret on B, the solver cannot claim A.
	if err := sw.SolverClaimA(); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("SolverClaimA before reveal = %v, want ErrNoSecret", err)
	}
	must(t, sw.UserClaimB(secret), "user claim B")
	must(t, sw.SolverClaimA(), "solver claim A after reveal")
	if got := a.balanceOf("solver", "FTB"); got.Cmp(bi(1000)) != 0 {
		t.Fatalf("solver A balance = %d, want 1000", got)
	}
}

func must(t *testing.T, err error, step string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", step, err)
	}
}
