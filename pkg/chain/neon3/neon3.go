// Package neon3 implements fasp.HTLC for a deployed FtbHtlc on Neo N3 via the
// neo-go SDK - the one pinned dependency. The actor signs with CalledByEntry and
// util.Uint160 carries the correct internal bytes, so the witness-scope and
// to-endianness traps of raw CLI construction cannot recur here.
package neon3

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/actor"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/gas"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/invoker"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/nep17"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/unwrap"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/vm/vmstate"
	"github.com/nspcc-dev/neo-go/pkg/wallet"

	"github.com/freethebird/ftb/pkg/fasp"
)

var errReadOnly = errors.New("neon3: read-only adapter has no signing account")

// Adapter implements fasp.HTLC against one Neo N3 FtbHtlc deployment.
type Adapter struct {
	cli        *rpcclient.Client
	inv        *invoker.Invoker
	act        *actor.Actor // nil => read-only
	token      *nep17.Token // the asset to lock (write path)
	tokenHash  util.Uint160
	contract   util.Uint160
	acc        *wallet.Account // nil => read-only
	name       string
	scanWindow uint32
}

// Config configures a Neo N3 adapter. WalletPath empty => read-only (Get,
// Preimage). TokenHash empty => GAS. Contract is a 0x-prefixed script hash (neo-go
// display form, little-endian hex as getcontractstate shows it) or an N3 address.
type Config struct {
	RPCURL     string
	Contract   string
	WalletPath string
	WalletPass string
	Account    string
	TokenHash  string
	Name       string
	ScanBlocks uint32
}

var _ fasp.HTLC = (*Adapter)(nil)

// New builds an adapter and initialises the client.
func New(ctx context.Context, cfg Config) (*Adapter, error) {
	cli, err := rpcclient.New(ctx, cfg.RPCURL, rpcclient.Options{DialTimeout: 30 * time.Second, RequestTimeout: 40 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("neon3: rpc: %w", err)
	}
	if err := cli.Init(); err != nil {
		return nil, fmt.Errorf("neon3: init: %w", err)
	}
	contract, err := parseU160(cfg.Contract)
	if err != nil {
		return nil, fmt.Errorf("neon3: contract: %w", err)
	}
	a := &Adapter{
		cli: cli, contract: contract, name: cfg.Name, scanWindow: cfg.ScanBlocks,
		inv: invoker.New(cli, nil), tokenHash: gas.Hash,
	}
	if a.name == "" {
		a.name = "neo-n3"
	}
	if a.scanWindow == 0 {
		a.scanWindow = 256
	}
	if cfg.TokenHash != "" {
		if a.tokenHash, err = parseU160(cfg.TokenHash); err != nil {
			return nil, fmt.Errorf("neon3: token: %w", err)
		}
	}
	if cfg.WalletPath != "" {
		w, err := wallet.NewWalletFromFile(cfg.WalletPath)
		if err != nil {
			return nil, fmt.Errorf("neon3: wallet: %w", err)
		}
		var acc *wallet.Account
		if cfg.Account != "" {
			h, err := address.StringToUint160(cfg.Account)
			if err != nil {
				return nil, fmt.Errorf("neon3: account: %w", err)
			}
			acc = w.GetAccount(h)
		} else if len(w.Accounts) > 0 {
			acc = w.Accounts[0]
		}
		if acc == nil {
			return nil, errors.New("neon3: account not found in wallet")
		}
		if err := acc.Decrypt(cfg.WalletPass, keys.NEP2ScryptParams()); err != nil {
			return nil, fmt.Errorf("neon3: decrypt: %w", err)
		}
		act, err := actor.NewSimple(cli, acc)
		if err != nil {
			return nil, fmt.Errorf("neon3: actor: %w", err)
		}
		a.act, a.acc = act, acc
		if a.tokenHash == gas.Hash {
			a.token = gas.New(act)
		} else {
			a.token = nep17.New(act, a.tokenHash)
		}
	}
	return a, nil
}

// Name identifies the leg.
func (a *Adapter) Name() string { return a.name }

// Address returns the signing account's address (empty when read-only).
func (a *Adapter) Address() string {
	if a.acc == nil {
		return ""
	}
	return a.acc.Address
}

// Lock escrows amount of the configured token under hashlock, payable to
// recipient (an N3 address), refundable to from at or after timelock (block
// height). Value-in flows through OnNEP17Payment with ["lock", ...] data.
func (a *Adapter) Lock(from, recipient, token string, amount *big.Int, hashlock [32]byte, timelock int64) error {
	if a.act == nil || a.token == nil {
		return errReadOnly
	}
	fromU, err := address.StringToUint160(from)
	if err != nil {
		return fmt.Errorf("neon3 lock: from: %w", err)
	}
	if fromU != a.acc.ScriptHash() {
		return errors.New("neon3 lock: from is not the adapter account")
	}
	recU, err := address.StringToUint160(recipient)
	if err != nil {
		return fmt.Errorf("neon3 lock: recipient: %w", err)
	}
	tokU, err := parseU160(token)
	if err != nil {
		return fmt.Errorf("neon3 lock: token: %w", err)
	}
	if tokU != a.tokenHash {
		return errors.New("neon3 lock: token does not match the adapter asset")
	}
	if amount == nil || amount.Sign() < 0 {
		return errors.New("neon3 lock: amount must be non-nil and non-negative")
	}
	data := []any{"lock", hashlock[:], recU, timelock}
	txid, vub, err := a.token.Transfer(fromU, a.contract, amount, data)
	return a.wait(txid, vub, err)
}

// Get fetches the Swap(hashlock) record; ok is false for an unused hashlock.
func (a *Adapter) Get(hashlock [32]byte) (fasp.SwapRecord, bool) {
	items, err := unwrap.Array(a.inv.Call(a.contract, "swap", hashlock[:]))
	if err != nil || len(items) < 6 {
		return fasp.SwapRecord{}, false
	}
	stateI, err := items[5].TryInteger()
	if err != nil || stateI.Sign() == 0 {
		return fasp.SwapRecord{}, false // NONE
	}
	rec := fasp.SwapRecord{State: fasp.SwapState(stateI.Int64())}
	if s, ok := addrOf(items[0]); ok {
		rec.Sender = s
	}
	if s, ok := addrOf(items[1]); ok {
		rec.Recipient = s
	}
	if s, ok := hashOf(items[2]); ok {
		rec.Token = s
	}
	if amt, err := items[3].TryInteger(); err == nil {
		rec.Amount = amt
	}
	if tl, err := items[4].TryInteger(); err == nil {
		rec.Timelock = tl.Int64()
	}
	return rec, true
}

// Claim reveals preimage to pay the recipient; permissionless, the actor pays.
func (a *Adapter) Claim(caller string, hashlock, preimage [32]byte) error {
	if a.act == nil {
		return errReadOnly
	}
	txid, vub, err := a.act.SendCall(a.contract, "claim", hashlock[:], preimage[:])
	return a.wait(txid, vub, err)
}

// Refund returns the escrow to the sender after the timelock; permissionless.
func (a *Adapter) Refund(caller string, hashlock [32]byte) error {
	if a.act == nil {
		return errReadOnly
	}
	txid, vub, err := a.act.SendCall(a.contract, "refund", hashlock[:])
	return a.wait(txid, vub, err)
}

// Preimage reads the secret from the Claimed notification. N3 has no event-query
// RPC, so this scans the last scanWindow blocks (reference approach; a production
// solver watches via a WS subscription from the lock height).
func (a *Adapter) Preimage(hashlock [32]byte) ([32]byte, bool) {
	height, err := a.cli.GetBlockCount()
	if err != nil {
		return [32]byte{}, false
	}
	var start uint32
	if height > a.scanWindow {
		start = height - a.scanWindow
	}
	for h := height; h > start; h-- {
		blk, err := a.cli.GetBlockByIndex(h - 1)
		if err != nil {
			continue
		}
		for i := range blk.Transactions {
			aer, err := a.cli.GetApplicationLog(blk.Transactions[i].Hash(), nil)
			if err != nil {
				continue
			}
			if pre, ok := claimedPreimage(aer.Executions, a.contract, hashlock); ok {
				return pre, true
			}
		}
	}
	return [32]byte{}, false
}

// ---- internals ----

func (a *Adapter) wait(txid util.Uint256, vub uint32, err error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	res, werr := a.act.Wait(ctx, txid, vub, err)
	if werr != nil {
		return fmt.Errorf("neon3: await %s: %w", txid.StringLE(), werr)
	}
	if res.VMState != vmstate.Halt {
		return fmt.Errorf("neon3: tx %s ended %s: %s", txid.StringLE(), res.VMState, res.FaultException)
	}
	return nil
}

// claimedPreimage finds the Claimed(hashlock, preimage) notification for contract
// in a transaction's executions and returns the revealed preimage.
func claimedPreimage(execs []state.Execution, contract util.Uint160, hashlock [32]byte) ([32]byte, bool) {
	for i := range execs {
		for _, ev := range execs[i].Events {
			if ev.ScriptHash != contract || ev.Name != "Claimed" || ev.Item == nil {
				continue
			}
			vals, ok := ev.Item.Value().([]stackitem.Item)
			if !ok || len(vals) < 2 {
				continue
			}
			hb, err := vals[0].TryBytes()
			if err != nil || len(hb) != 32 {
				continue
			}
			var h [32]byte
			copy(h[:], hb)
			if h != hashlock {
				continue
			}
			pb, err := vals[1].TryBytes()
			if err != nil || len(pb) != 32 {
				continue
			}
			var pre [32]byte
			copy(pre[:], pb)
			return pre, true
		}
	}
	return [32]byte{}, false
}

func addrOf(it stackitem.Item) (string, bool) {
	b, err := it.TryBytes()
	if err != nil || len(b) != 20 {
		return "", false
	}
	u, err := util.Uint160DecodeBytesLE(b)
	if err != nil {
		return "", false
	}
	return address.Uint160ToString(u), true
}

func hashOf(it stackitem.Item) (string, bool) {
	b, err := it.TryBytes()
	if err != nil || len(b) != 20 {
		return "", false
	}
	u, err := util.Uint160DecodeBytesLE(b)
	if err != nil {
		return "", false
	}
	return "0x" + u.StringBE(), true
}

func parseU160(s string) (util.Uint160, error) {
	switch {
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		return util.Uint160DecodeStringLE(s[2:])
	case strings.HasPrefix(s, "N"):
		return address.StringToUint160(s)
	default:
		return util.Uint160DecodeStringLE(s)
	}
}

// Height returns the latest Neo N3 block height.
func (a *Adapter) Height(ctx context.Context) (uint64, error) {
	h, err := a.cli.GetBlockCount()
	return uint64(h), err
}
