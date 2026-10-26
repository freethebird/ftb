package store

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestPutGetPersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bonds.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Count() != 0 {
		t.Fatalf("new store not empty: %d", s.Count())
	}
	rec := Record{DiscordID: "u1", Username: "a", Wallet: "Nxxx", BondID: "42", TxID: "0xabc", VerifiedAt: 1000}
	if err := s.Put(rec); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk — must survive.
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("u1")
	if !ok || got.BondID != "42" || got.Wallet != "Nxxx" {
		t.Fatalf("record not persisted: %+v ok=%v", got, ok)
	}
	if s2.Count() != 1 {
		t.Fatalf("count = %d", s2.Count())
	}
}

func TestDeletePersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bonds.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{DiscordID: "u1", BondID: "42", VerifiedAt: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("u1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("u1"); ok {
		t.Fatal("record still present after Delete")
	}
	// Deletion must survive a reopen from disk.
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("u1"); ok {
		t.Fatal("deletion not persisted")
	}
	if s2.Count() != 0 {
		t.Fatalf("count = %d, want 0", s2.Count())
	}
}

func TestAllSorted(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bonds.json")
	s, _ := Open(p)
	_ = s.Put(Record{DiscordID: "b", VerifiedAt: 200})
	_ = s.Put(Record{DiscordID: "a", VerifiedAt: 100})
	all := s.All()
	if len(all) != 2 || all[0].DiscordID != "a" {
		t.Fatalf("not sorted oldest-first: %+v", all)
	}
}

func TestConcurrentPut(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bonds.json")
	s, _ := Open(p)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = s.Put(Record{DiscordID: itoa(n), VerifiedAt: int64(n)})
			_, _ = s.Get(itoa(n))
			_ = s.All()
		}(i)
	}
	wg.Wait()
	if s.Count() != 20 {
		t.Fatalf("count = %d, want 20", s.Count())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
