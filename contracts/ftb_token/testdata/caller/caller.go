// caller is a tiny test-only contract used to exercise the migrator-only mint path:
// when it calls mintForMigration, the token sees THIS contract's hash as the caller,
// which is exactly how the real Migrator will mint.
package caller

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
)

func Mint(token interop.Hash160, to interop.Hash160, amount int) {
	contract.Call(token, "mintForMigration", contract.All, to, amount)
}
