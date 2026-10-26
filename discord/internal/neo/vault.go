package neo

import (
	"fmt"
	"math/big"

	"github.com/nspcc-dev/neo-go/pkg/util"
)

// BondStatus mirrors the vault's status enum.
type BondStatus int

const (
	BondNone       BondStatus = 0
	BondActive     BondStatus = 1
	BondRefunded   BondStatus = 2
	BondLiquidated BondStatus = 3
)

func (s BondStatus) String() string {
	switch s {
	case BondActive:
		return "active"
	case BondRefunded:
		return "refunded"
	case BondLiquidated:
		return "liquidated"
	default:
		return "none"
	}
}

// Bond is the vault's record for one posted bond.
//
// INTERFACE (getBond(bondId) -> Struct/Array), the shape the contract must emit:
//
//	[0] owner      ByteString  20-byte script hash (big-endian) of the depositor's wallet
//	[1] amount     Integer     bond amount in FTB base units (8 decimals)
//	[2] discordId  ByteString  the depositor's immutable Discord user ID (snowflake, as UTF-8)
//	[3] status     Integer     0 none / 1 active / 2 refunded / 3 liquidated
//	[4] depositTx  ByteString  32-byte tx hash (big-endian) of the deposit
//	[5] createdAt  Integer     unix-ms timestamp
//
// Note on the tag (field [2]): the spec said "tagged with the Discord handle".
// We pin it to the Discord USER ID instead — handles are mutable, IDs are not —
// so the bond↔member link never breaks on a rename. Documented in the README.
type Bond struct {
	ID         *big.Int
	Owner      util.Uint160
	AmountBase *big.Int
	DiscordID  string
	Status     BondStatus
	DepositTx  util.Uint256
	CreatedAt  *big.Int
}

// GetBond reads a bond by id. The bool is false (with nil error) when the vault
// has no such bond (status none / null), so callers can distinguish
// "not found" from "RPC error".
func (c *Client) GetBond(vault util.Uint160, method string, bondID *big.Int) (*Bond, bool, error) {
	stack, err := c.call(vault, method, bondID)
	if err != nil {
		return nil, false, err
	}
	top, err := single(stack)
	if err != nil {
		return nil, false, err
	}
	arr, err := itemArray(top)
	if err != nil {
		return nil, false, err
	}
	if len(arr) == 0 {
		return nil, false, nil // null => not found
	}
	if len(arr) < 6 {
		return nil, false, fmt.Errorf("getBond returned %d fields, want 6", len(arr))
	}

	b := &Bond{ID: new(big.Int).Set(bondID)}

	ownerBytes, err := itemBytes(arr[0])
	if err != nil {
		return nil, false, fmt.Errorf("bond owner: %w", err)
	}
	if b.Owner, err = util.Uint160DecodeBytesBE(ownerBytes); err != nil {
		return nil, false, fmt.Errorf("bond owner hash: %w", err)
	}
	if b.AmountBase, err = itemBig(arr[1]); err != nil {
		return nil, false, fmt.Errorf("bond amount: %w", err)
	}
	if b.DiscordID, err = itemString(arr[2]); err != nil {
		return nil, false, fmt.Errorf("bond discordId: %w", err)
	}
	statusBig, err := itemBig(arr[3])
	if err != nil {
		return nil, false, fmt.Errorf("bond status: %w", err)
	}
	b.Status = BondStatus(statusBig.Int64())
	if b.Status == BondNone {
		return nil, false, nil
	}
	txBytes, err := itemBytes(arr[4])
	if err != nil {
		return nil, false, fmt.Errorf("bond depositTx: %w", err)
	}
	if len(txBytes) != 0 {
		if b.DepositTx, err = util.Uint256DecodeBytesBE(txBytes); err != nil {
			return nil, false, fmt.Errorf("bond depositTx hash: %w", err)
		}
	}
	if b.CreatedAt, err = itemBig(arr[5]); err != nil {
		return nil, false, fmt.Errorf("bond createdAt: %w", err)
	}
	return b, true, nil
}

// GetBondByDiscord resolves a Discord user ID to its bond id via the vault's
// reverse index (getBondByDiscord(discordId) -> Integer, 0 if none). found is
// false (nil error) when the member holds no bond.
func (c *Client) GetBondByDiscord(vault util.Uint160, method, discordID string) (*big.Int, bool, error) {
	stack, err := c.call(vault, method, []byte(discordID))
	if err != nil {
		return nil, false, err
	}
	top, err := single(stack)
	if err != nil {
		return nil, false, err
	}
	id, err := itemBig(top)
	if err != nil {
		return nil, false, fmt.Errorf("bondId: %w", err)
	}
	if id.Sign() == 0 {
		return nil, false, nil
	}
	return id, true, nil
}

// BondAmountBase reads the vault's fixed bond amount (base units).
func (c *Client) BondAmountBase(vault util.Uint160, method string) (*big.Int, error) {
	stack, err := c.call(vault, method)
	if err != nil {
		return nil, err
	}
	top, err := single(stack)
	if err != nil {
		return nil, err
	}
	return itemBig(top)
}
