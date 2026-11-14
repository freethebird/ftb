// Package neox implements fasp.HTLC for a deployed FtbHtlc.sol on a Neo X (EVM)
// chain, built entirely on pkg/evm: the standard library plus the secp256k1
// already compiled inside the one pinned neo-go dependency. No cgo, no further
// module. Identities are 0x-hex EVM addresses; the native coin (GAS) is the zero
// address, matching the contract's NATIVE sentinel.
package neox

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/freethebird/ftb/pkg/evm"
	"github.com/freethebird/ftb/pkg/fasp"
)

var (
	selLock   = evm.Selector("lock(bytes32,address,address,uint256,uint256)")
	selClaim  = evm.Selector("claim(bytes32,bytes32)")
	selRefund = evm.Selector("refund(bytes32)")
	selSwaps  = evm.Selector("swaps(bytes32)")
	// topicClaimed is keccak256("Claimed(bytes32,bytes32)"), the log topic0.
	topicClaimed = evm.Keccak256([]byte("Claimed(bytes32,bytes32)"))
)

// zeroAddr is the native-coin sentinel (address(0)).
var zeroAddr [20]byte

var errReadOnly = errors.New("neox: read-only adapter has no signing key")

// Adapter implements fasp.HTLC against one FtbHtlc.sol deployment.
type Adapter struct {
	rpc       *evm.RPC
	contract  [20]byte
	priv      *secp256k1.PrivateKey // nil => read-only (Get/Preimage only)
	addr      [20]byte
	chainID   *big.Int
	tipFloor  *big.Int
	feeFloor  *big.Int
	opTimeout time.Duration
	name      string
}

// Config configures a Neo X adapter. PrivKeyHex may be empty for a read-only
// adapter. Fee floors default to the Neo X minimums; ChainID is fetched if nil.
type Config struct {
	RPCURL      string
	Contract    string
	PrivKeyHex  string
	ChainID     *big.Int
	TipFloorWei *big.Int
	FeeFloorWei *big.Int
	OpTimeout   time.Duration
	Name        string
}

var _ fasp.HTLC = (*Adapter)(nil)

// New builds an adapter, fetching the chain id when not supplied.
func New(ctx context.Context, cfg Config) (*Adapter, error) {
	contract, err := parseAddr(cfg.Contract)
	if err != nil {
		return nil, fmt.Errorf("neox: contract: %w", err)
	}
	a := &Adapter{
		rpc:       evm.NewRPC(cfg.RPCURL),
		contract:  contract,
		chainID:   cfg.ChainID,
		tipFloor:  cfg.TipFloorWei,
		feeFloor:  cfg.FeeFloorWei,
		opTimeout: cfg.OpTimeout,
		name:      cfg.Name,
	}
	if a.name == "" {
		a.name = "neo-x"
	}
	if a.opTimeout <= 0 {
		a.opTimeout = 120 * time.Second
	}
	if a.tipFloor == nil {
		a.tipFloor = new(big.Int).SetUint64(20_000_000_000) // 20 gwei
	}
	if a.feeFloor == nil {
		a.feeFloor = new(big.Int).SetUint64(50_000_000_000) // 50 gwei
	}
	if s := strings.TrimPrefix(cfg.PrivKeyHex, "0x"); s != "" {
		kb, err := hex.DecodeString(s)
		if err != nil || len(kb) != 32 {
			return nil, errors.New("neox: private key must be 32 hex bytes")
		}
		a.priv = secp256k1.PrivKeyFromBytes(kb)
		a.addr = evm.AddressFromPriv(a.priv)
	}
	if a.chainID == nil {
		id, err := a.rpc.ChainID(ctx)
		if err != nil {
			return nil, fmt.Errorf("neox: chainId: %w", err)
		}
		a.chainID = id
	}
	return a, nil
}

// Name identifies the leg.
func (a *Adapter) Name() string { return a.name }

// Address returns the adapter's signing address (zero for a read-only adapter).
func (a *Adapter) Address() string { return addrStr(a.addr) }

// Lock escrows amount of token (zero address for native) under hashlock, payable
// to recipient, refundable to from at or after timelock (unix seconds).
func (a *Adapter) Lock(from, recipient, token string, amount *big.Int, hashlock [32]byte, timelock int64) error {
	if a.priv == nil {
		return errReadOnly
	}
	fromA, err := parseAddr(from)
	if err != nil {
		return fmt.Errorf("neox lock: from: %w", err)
	}
	if fromA != a.addr {
		return fmt.Errorf("neox lock: from %s is not the adapter address %s", from, a.Address())
	}
	recA, err := parseAddr(recipient)
	if err != nil {
		return fmt.Errorf("neox lock: recipient: %w", err)
	}
	tokA, err := parseAddr(token)
	if err != nil {
		return fmt.Errorf("neox lock: token: %w", err)
	}
	if amount == nil || amount.Sign() < 0 {
		return errors.New("neox lock: amount must be non-nil and non-negative")
	}
	data := evm.EncodeCall(selLock,
		evm.WordBytes32(hashlock),
		evm.WordAddress(recA),
		evm.WordAddress(tokA),
		evm.WordUint(amount),
		evm.WordUint(big.NewInt(timelock)),
	)
	value := big.NewInt(0)
	if tokA == zeroAddr {
		value = amount // native: msg.value must equal amount
	}
	return a.sendTx(data, value)
}

// Get fetches the swaps(hashlock) record. ok is false when the hashlock is
// unused (state NONE) or on a read error (fail closed in VerifyLock).
func (a *Adapter) Get(hashlock [32]byte) (fasp.SwapRecord, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), a.opTimeout)
	defer cancel()
	ret, err := a.rpc.Call(ctx, a.contract, evm.EncodeCall(selSwaps, evm.WordBytes32(hashlock)))
	if err != nil {
		return fasp.SwapRecord{}, false
	}
	rec, ok, err := decodeSwap(ret)
	if err != nil {
		return fasp.SwapRecord{}, false
	}
	return rec, ok
}

// Claim reveals preimage to pay the recipient; permissionless, the adapter pays.
func (a *Adapter) Claim(caller string, hashlock, preimage [32]byte) error {
	if a.priv == nil {
		return errReadOnly
	}
	if err := a.requireCaller(caller, "claim"); err != nil {
		return err
	}
	return a.sendTx(evm.EncodeCall(selClaim, evm.WordBytes32(hashlock), evm.WordBytes32(preimage)), big.NewInt(0))
}

// Refund returns the escrow to the sender after the timelock; permissionless.
func (a *Adapter) Refund(caller string, hashlock [32]byte) error {
	if a.priv == nil {
		return errReadOnly
	}
	if err := a.requireCaller(caller, "refund"); err != nil {
		return err
	}
	return a.sendTx(evm.EncodeCall(selRefund, evm.WordBytes32(hashlock)), big.NewInt(0))
}

// Preimage reads the secret revealed in the Claimed event for hashlock.
func (a *Adapter) Preimage(hashlock [32]byte) ([32]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), a.opTimeout)
	defer cancel()
	topics := []any{"0x" + hex.EncodeToString(topicClaimed[:]), "0x" + hex.EncodeToString(hashlock[:])}
	logs, err := a.rpc.Logs(ctx, a.contract, topics, "earliest", "latest")
	if err != nil || len(logs) == 0 {
		return [32]byte{}, false
	}
	// Claimed(bytes32 indexed hashlock, bytes32 preimage): preimage is the only
	// non-indexed field, so it is the first data word.
	d := logs[0].Data
	if len(d) < 32 {
		return [32]byte{}, false
	}
	var pre [32]byte
	copy(pre[:], d[:32])
	return pre, true
}

// ---- internals ----

func (a *Adapter) sendTx(data []byte, value *big.Int) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.opTimeout)
	defer cancel()
	nonce, err := a.rpc.Nonce(ctx, a.addr)
	if err != nil {
		return fmt.Errorf("neox: nonce: %w", err)
	}
	tip, maxFee, err := a.rpc.SuggestFees(ctx, a.tipFloor, a.feeFloor)
	if err != nil {
		return fmt.Errorf("neox: fees: %w", err)
	}
	gas, err := a.rpc.EstimateGas(ctx, a.addr, a.contract, value, data)
	if err != nil {
		return fmt.Errorf("neox: estimateGas: %w", err)
	}
	gas += gas / 5 // 20% headroom over the estimate
	tx := &evm.DynTx{
		ChainID: a.chainID, Nonce: nonce,
		GasTipCap: tip, GasFeeCap: maxFee, Gas: gas,
		To: a.contract, Value: value, Data: data,
	}
	raw, _, err := evm.SignRaw(tx, a.priv)
	if err != nil {
		return fmt.Errorf("neox: sign: %w", err)
	}
	txHash, err := a.rpc.SendRaw(ctx, raw)
	if err != nil {
		return fmt.Errorf("neox: send: %w", err)
	}
	return a.waitReceipt(ctx, txHash)
}

func (a *Adapter) waitReceipt(ctx context.Context, txHash [32]byte) error {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		rec, found, err := a.rpc.Receipt(ctx, txHash)
		if err == nil && found {
			if rec.Status != 1 {
				return fmt.Errorf("neox: tx 0x%x reverted (status %d)", txHash, rec.Status)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("neox: waiting for tx 0x%x: %w", txHash, ctx.Err())
		case <-t.C:
		}
	}
}

func (a *Adapter) requireCaller(caller, op string) error {
	c, err := parseAddr(caller)
	if err != nil {
		return fmt.Errorf("neox %s: caller: %w", op, err)
	}
	if c != a.addr {
		return fmt.Errorf("neox %s: caller %s is not the adapter address %s", op, caller, a.Address())
	}
	return nil
}

// decodeSwap maps a swaps(hashlock) return (6 words: sender, state, recipient,
// token, amount, timelock) to a SwapRecord. ok is false for an unused hashlock.
func decodeSwap(ret []byte) (fasp.SwapRecord, bool, error) {
	ws := evm.SplitWords(ret)
	if len(ws) < 6 {
		return fasp.SwapRecord{}, false, fmt.Errorf("neox: swaps() returned %d words, want 6", len(ws))
	}
	state := fasp.SwapState(evm.WordToUint8(ws[1]))
	if state == fasp.StateNone {
		return fasp.SwapRecord{}, false, nil
	}
	return fasp.SwapRecord{
		Sender:    addrStr(evm.WordToAddress(ws[0])),
		Recipient: addrStr(evm.WordToAddress(ws[2])),
		Token:     addrStr(evm.WordToAddress(ws[3])),
		Amount:    evm.WordToUint(ws[4]),
		Timelock:  evm.WordToUint(ws[5]).Int64(),
		State:     state,
	}, true, nil
}

func parseAddr(s string) ([20]byte, error) {
	var a [20]byte
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return a, err
	}
	if len(b) != 20 {
		return a, fmt.Errorf("evm address must be 20 bytes, got %d", len(b))
	}
	copy(a[:], b)
	return a, nil
}

func addrStr(a [20]byte) string { return "0x" + hex.EncodeToString(a[:]) }

// Height returns the latest Neo X block number.
func (a *Adapter) Height(ctx context.Context) (uint64, error) { return a.rpc.BlockNumber(ctx) }
