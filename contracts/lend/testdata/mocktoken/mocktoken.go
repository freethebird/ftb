// mocktoken is a minimal, openly-mintable NEP-17 used only in lend tests to stand in for
// bNEO (so a test borrower has collateral to deposit). Not for production.
package mocktoken

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/management"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	prefixBal = []byte{0x01}
	keySupply = []byte{0x02}
)

func Symbol() string                  { return "MOCK" }
func Decimals() int                   { return 8 }
func TotalSupply() int                { return gi(keySupply) }
func BalanceOf(a interop.Hash160) int { return gi(bk(a)) }

func Transfer(from, to interop.Hash160, amount int, data any) bool {
	if len(from) != 20 || len(to) != 20 || amount < 0 {
		return false
	}
	if !runtime.CheckWitness(from) {
		return false
	}
	ctx := storage.GetContext()
	fb := gi(bk(from))
	if fb < amount {
		return false
	}
	if amount > 0 {
		storage.Put(ctx, bk(from), fb-amount)
		storage.Put(ctx, bk(to), gi(bk(to))+amount)
	}
	runtime.Notify("Transfer", from, to, amount)
	if management.GetContract(to) != nil {
		contract.Call(to, "onNEP17Payment", contract.All, from, amount, data)
	}
	return true
}

// Mint is open on purpose: tests only.
func Mint(to interop.Hash160, amount int) {
	ctx := storage.GetContext()
	storage.Put(ctx, keySupply, gi(keySupply)+amount)
	storage.Put(ctx, bk(to), gi(bk(to))+amount)
	runtime.Notify("Transfer", interop.Hash160(nil), to, amount)
}

func bk(a interop.Hash160) []byte { return append(prefixBal, a...) }
func gi(k []byte) int {
	v := storage.Get(storage.GetContext(), k)
	if v == nil {
		return 0
	}
	return v.(int)
}
