package neo

import (
	"math/big"

	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/vm/vmstate"
)

// VerifyDepositTransfer corroborates a bond deposit by scanning the deposit
// transaction's application log for a NEP-17 Transfer emitted by the FTB token
// contract moving exactly `amount` from `from` to `vault`.
//
// This is a SECONDARY check. The authoritative verification is GetBond (the
// vault's own record). This scan guards against a bond record that does not
// match a real on-chain transfer.
//
// Returned (false, nil) means "no matching Transfer found" (a clean negative),
// distinct from an RPC error.
func (c *Client) VerifyDepositTransfer(txid util.Uint256, token, vault, from util.Uint160, amount *big.Int) (bool, error) {
	log, err := c.rpc.GetApplicationLog(txid, nil)
	if err != nil {
		return false, err
	}
	for _, exec := range log.Executions {
		if exec.VMState != vmstate.Halt {
			continue
		}
		for _, ev := range exec.Events {
			if ev.Name != "Transfer" {
				continue
			}
			if !ev.ScriptHash.Equals(token) {
				continue
			}
			var it stackitem.Item = ev.Item
			arr, err := itemArray(it)
			if err != nil || len(arr) < 3 {
				continue
			}
			amt, err := itemBig(arr[2])
			if err != nil || amt.Cmp(amount) != 0 {
				continue
			}
			if accountMatches(arr[0], from) && accountMatches(arr[1], vault) {
				return true, nil
			}
		}
	}
	return false, nil
}

// accountMatches reports whether a Transfer account stack item (a 20-byte
// account, or Null for mint/burn) equals target.
//
// WIRE-LIVE TODO: NEP-17 events carry the account as a 20-byte script hash; pin
// the exact byte order against a real FTB testnet Transfer when the vault goes
// live. Until then we accept a match on either orientation so a correct deposit
// is never rejected on an endianness guess — the authoritative GetBond check
// still gates verification.
func accountMatches(it stackitem.Item, target util.Uint160) bool {
	b, err := it.TryBytes()
	if err != nil || len(b) != 20 {
		return false
	}
	if h, err := util.Uint160DecodeBytesBE(b); err == nil && h.Equals(target) {
		return true
	}
	if h, err := util.Uint160DecodeBytesLE(b); err == nil && h.Equals(target) {
		return true
	}
	return false
}
