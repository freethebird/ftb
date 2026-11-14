package evm

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// RPC is a minimal Ethereum JSON-RPC client over HTTP for the Neo X leg. Only
// the methods the HTLC adapter needs are implemented; every error is returned,
// never swallowed.
type RPC struct {
	url string
	hc  *http.Client
}

// NewRPC returns a client for the given endpoint with a bounded timeout.
func NewRPC(url string) *RPC {
	return &RPC{url: url, hc: &http.Client{Timeout: 30 * time.Second}}
}

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("evm rpc error %d: %s", e.Code, e.Message) }

type rpcResp struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func (r *RPC) call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(rpcReq{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var out rpcResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("evm rpc %s: decode: %w", method, err)
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Result, nil
}

// ---- hex helpers (eth quantity and data encodings) ----

func hexQuantity(x *big.Int) string {
	if x == nil || x.Sign() == 0 {
		return "0x0"
	}
	return "0x" + x.Text(16)
}

func hexData(b []byte) string { return "0x" + hex.EncodeToString(b) }

func hexAddr(a [20]byte) string { return "0x" + hex.EncodeToString(a[:]) }

func parseQuantity(s string) (*big.Int, error) {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return big.NewInt(0), nil
	}
	x, ok := new(big.Int).SetString(s, 16)
	if !ok {
		return nil, fmt.Errorf("evm: bad quantity %q", s)
	}
	return x, nil
}

func parseData(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	if len(s)%2 != 0 {
		s = "0" + s
	}
	return hex.DecodeString(s)
}

func unquote(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

// ---- typed methods ----

// ChainID returns eth_chainId.
func (r *RPC) ChainID(ctx context.Context) (*big.Int, error) {
	raw, err := r.call(ctx, "eth_chainId")
	if err != nil {
		return nil, err
	}
	s, err := unquote(raw)
	if err != nil {
		return nil, err
	}
	return parseQuantity(s)
}

// Nonce returns the pending transaction count for a.
func (r *RPC) Nonce(ctx context.Context, a [20]byte) (uint64, error) {
	raw, err := r.call(ctx, "eth_getTransactionCount", hexAddr(a), "pending")
	if err != nil {
		return 0, err
	}
	s, err := unquote(raw)
	if err != nil {
		return 0, err
	}
	q, err := parseQuantity(s)
	if err != nil {
		return 0, err
	}
	return q.Uint64(), nil
}

// Call performs eth_call against to with calldata data at the latest block.
func (r *RPC) Call(ctx context.Context, to [20]byte, data []byte) ([]byte, error) {
	raw, err := r.call(ctx, "eth_call", map[string]any{"to": hexAddr(to), "data": hexData(data)}, "latest")
	if err != nil {
		return nil, err
	}
	s, err := unquote(raw)
	if err != nil {
		return nil, err
	}
	return parseData(s)
}

// EstimateGas returns eth_estimateGas for the call.
func (r *RPC) EstimateGas(ctx context.Context, from, to [20]byte, value *big.Int, data []byte) (uint64, error) {
	c := map[string]any{"from": hexAddr(from), "to": hexAddr(to), "data": hexData(data)}
	if value != nil && value.Sign() > 0 {
		c["value"] = hexQuantity(value)
	}
	raw, err := r.call(ctx, "eth_estimateGas", c)
	if err != nil {
		return 0, err
	}
	s, err := unquote(raw)
	if err != nil {
		return 0, err
	}
	q, err := parseQuantity(s)
	if err != nil {
		return 0, err
	}
	return q.Uint64(), nil
}

// SuggestFees returns (tip, maxFee): the node's tip (floored at tipFloor) and
// maxFee = 2*baseFee + tip (floored at feeFloor), meeting the Neo X minimums.
func (r *RPC) SuggestFees(ctx context.Context, tipFloor, feeFloor *big.Int) (tip, maxFee *big.Int, err error) {
	raw, err := r.call(ctx, "eth_getBlockByNumber", "latest", false)
	if err != nil {
		return nil, nil, err
	}
	var hdr struct {
		BaseFeePerGas string `json:"baseFeePerGas"`
	}
	if err := json.Unmarshal(raw, &hdr); err != nil {
		return nil, nil, err
	}
	base, err := parseQuantity(hdr.BaseFeePerGas)
	if err != nil {
		base = big.NewInt(0)
	}
	tip = new(big.Int).Set(tipFloor)
	if traw, e := r.call(ctx, "eth_maxPriorityFeePerGas"); e == nil {
		if s, e2 := unquote(traw); e2 == nil {
			if q, e3 := parseQuantity(s); e3 == nil && q.Cmp(tip) > 0 {
				tip = q
			}
		}
	}
	maxFee = new(big.Int).Add(new(big.Int).Lsh(base, 1), tip)
	if maxFee.Cmp(feeFloor) < 0 {
		maxFee = new(big.Int).Set(feeFloor)
	}
	return tip, maxFee, nil
}

// SendRaw broadcasts a raw transaction and returns its hash.
func (r *RPC) SendRaw(ctx context.Context, raw []byte) ([32]byte, error) {
	res, err := r.call(ctx, "eth_sendRawTransaction", hexData(raw))
	if err != nil {
		return [32]byte{}, err
	}
	s, err := unquote(res)
	if err != nil {
		return [32]byte{}, err
	}
	b, err := parseData(s)
	if err != nil {
		return [32]byte{}, err
	}
	var h [32]byte
	if len(b) != 32 {
		return h, fmt.Errorf("evm: bad tx hash length %d", len(b))
	}
	copy(h[:], b)
	return h, nil
}

// Receipt is the subset of a transaction receipt the adapter inspects.
type Receipt struct {
	Status      uint64
	BlockNumber uint64
}

// Receipt returns the receipt and whether it was found (nil until mined).
func (r *RPC) Receipt(ctx context.Context, txHash [32]byte) (*Receipt, bool, error) {
	raw, err := r.call(ctx, "eth_getTransactionReceipt", hexData(txHash[:]))
	if err != nil {
		return nil, false, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	var jr struct {
		Status      string `json:"status"`
		BlockNumber string `json:"blockNumber"`
	}
	if err := json.Unmarshal(raw, &jr); err != nil {
		return nil, false, err
	}
	st, _ := parseQuantity(jr.Status)
	bn, _ := parseQuantity(jr.BlockNumber)
	return &Receipt{Status: st.Uint64(), BlockNumber: bn.Uint64()}, true, nil
}

// Log is one event log: its indexed topics and its data payload.
type Log struct {
	Topics [][32]byte
	Data   []byte
	TxHash [32]byte
}

// Logs queries eth_getLogs for addr over [fromBlock,toBlock] matching topics
// (each topic a 0x hex string, or nil for a wildcard position).
func (r *RPC) Logs(ctx context.Context, addr [20]byte, topics []any, fromBlock, toBlock string) ([]Log, error) {
	filter := map[string]any{
		"address":   hexAddr(addr),
		"topics":    topics,
		"fromBlock": fromBlock,
		"toBlock":   toBlock,
	}
	raw, err := r.call(ctx, "eth_getLogs", filter)
	if err != nil {
		return nil, err
	}
	var jls []struct {
		Topics []string `json:"topics"`
		Data   string   `json:"data"`
		TxHash string   `json:"transactionHash"`
	}
	if err := json.Unmarshal(raw, &jls); err != nil {
		return nil, err
	}
	out := make([]Log, 0, len(jls))
	for _, jl := range jls {
		var lg Log
		for _, ts := range jl.Topics {
			tb, err := parseData(ts)
			if err != nil || len(tb) != 32 {
				continue
			}
			var t [32]byte
			copy(t[:], tb)
			lg.Topics = append(lg.Topics, t)
		}
		lg.Data, _ = parseData(jl.Data)
		if tb, e := parseData(jl.TxHash); e == nil && len(tb) == 32 {
			copy(lg.TxHash[:], tb)
		}
		out = append(out, lg)
	}
	return out, nil
}

// BlockNumber returns the latest block height.
func (r *RPC) BlockNumber(ctx context.Context) (uint64, error) {
	raw, err := r.call(ctx, "eth_blockNumber")
	if err != nil {
		return 0, err
	}
	s, err := unquote(raw)
	if err != nil {
		return 0, err
	}
	q, err := parseQuantity(s)
	if err != nil {
		return 0, err
	}
	return q.Uint64(), nil
}

// TxInfo is the subset of an EIP-1559 transaction needed to re-derive its
// signing hash and recover its sender.
type TxInfo struct {
	ChainID   *big.Int
	Nonce     uint64
	GasTipCap *big.Int
	GasFeeCap *big.Int
	Gas       uint64
	To        [20]byte
	Value     *big.Int
	Data      []byte
	YParity   uint64
	R, S      []byte
	From      [20]byte
}

// TxByHash returns transaction fields via eth_getTransactionByHash.
func (r *RPC) TxByHash(ctx context.Context, h [32]byte) (*TxInfo, error) {
	raw, err := r.call(ctx, "eth_getTransactionByHash", hexData(h[:]))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("evm: tx 0x%x not found", h)
	}
	var j struct {
		ChainID string `json:"chainId"`
		Nonce   string `json:"nonce"`
		GasTip  string `json:"maxPriorityFeePerGas"`
		GasFee  string `json:"maxFeePerGas"`
		Gas     string `json:"gas"`
		To      string `json:"to"`
		Value   string `json:"value"`
		Input   string `json:"input"`
		YParity string `json:"yParity"`
		V       string `json:"v"`
		R       string `json:"r"`
		S       string `json:"s"`
		From    string `json:"from"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	info := &TxInfo{}
	info.ChainID, _ = parseQuantity(j.ChainID)
	if q, e := parseQuantity(j.Nonce); e == nil {
		info.Nonce = q.Uint64()
	}
	info.GasTipCap, _ = parseQuantity(j.GasTip)
	info.GasFeeCap, _ = parseQuantity(j.GasFee)
	if q, e := parseQuantity(j.Gas); e == nil {
		info.Gas = q.Uint64()
	}
	if b, e := parseData(j.To); e == nil && len(b) == 20 {
		copy(info.To[:], b)
	}
	info.Value, _ = parseQuantity(j.Value)
	info.Data, _ = parseData(j.Input)
	yp := j.YParity
	if yp == "" {
		yp = j.V
	}
	if q, e := parseQuantity(yp); e == nil {
		info.YParity = q.Uint64()
	}
	info.R, _ = parseData(j.R)
	info.S, _ = parseData(j.S)
	if b, e := parseData(j.From); e == nil && len(b) == 20 {
		copy(info.From[:], b)
	}
	return info, nil
}
