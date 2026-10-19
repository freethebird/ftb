// caller2 is a second tiny test-only contract, identical to caller but with a distinct
// manifest name (and therefore a distinct contract hash): it plays the role of a
// REPLACEMENT migrator, so tests can prove the token's claimed ledger survives a
// migrator swap — the exact R2 + R3 double-claim path seen on TestNet.
package caller2

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
)

func Mint(token interop.Hash160, to interop.Hash160, amount int) {
	contract.Call(token, "mintForMigration", contract.All, to, amount)
}
