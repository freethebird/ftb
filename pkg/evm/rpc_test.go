package evm

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHexHelpers(t *testing.T) {
	if hexQuantity(big.NewInt(0)) != "0x0" || hexQuantity(big.NewInt(255)) != "0xff" {
		t.Error("hexQuantity")
	}
	q, err := parseQuantity("0xff")
	if err != nil || q.Int64() != 255 {
		t.Error("parseQuantity 0xff")
	}
	q, _ = parseQuantity("0x")
	if q.Sign() != 0 {
		t.Error("parseQuantity 0x should be 0")
	}
	b, _ := parseData("0xabcd")
	if len(b) != 2 || b[0] != 0xab || b[1] != 0xcd {
		t.Error("parseData")
	}
}

func TestRPCCallMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var in rpcReq
		_ = json.NewDecoder(req.Body).Decode(&in)
		if in.Method != "eth_chainId" {
			http.Error(w, "unexpected", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "result": "0xba8c04"})
	}))
	defer srv.Close()
	id, err := NewRPC(srv.URL).ChainID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id.Int64() != 0xba8c04 {
		t.Errorf("chainId = %d, want %d", id.Int64(), 0xba8c04)
	}
}

func TestRPCErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1,
			"error": map[string]any{"code": -32000, "message": "nonce too low"}})
	}))
	defer srv.Close()
	_, err := NewRPC(srv.URL).Nonce(context.Background(), [20]byte{})
	if err == nil {
		t.Fatal("expected error")
	}
}
