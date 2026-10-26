// Package neo is the read-only Neo N3 seam for the FTB Discord bots.
//
// It holds NO wallet and NO signer: every call here is an RPC read
// (invokefunction in test mode, or getapplicationlog). Writing to the chain —
// filing a case, voting, refunding or burning a bond — is always done by a
// bonded member signing with their own wallet on the member-operated signing
// page (see internal/handoff). This keeps the no-admin-key / ownerless posture:
// no bot ever moves a bond or files on anyone's behalf.
//
// Interface-first: the Discord bond-vault and accusation-ballot contracts are
// not deployed yet. The method surface and the returned stack-item encodings
// defined here ARE the interface the contracts must conform to. The README
// restates it as the canonical spec for the contract/git chat.
package neo

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/neorpc/result"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient"
	"github.com/nspcc-dev/neo-go/pkg/rpcclient/invoker"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/vm/vmstate"
)

// Client is a thin read-only wrapper over a neo-go RPC client + invoker.
type Client struct {
	rpc *rpcclient.Client
	inv *invoker.Invoker
}

// Dial connects to a Neo N3 RPC endpoint and initialises the client. It performs
// network I/O (getversion); callers should treat a failure as "chain
// unreachable" and degrade gracefully rather than crashing — standby features
// do not need the chain at all.
func Dial(ctx context.Context, endpoint string, timeout time.Duration) (*Client, error) {
	cli, err := rpcclient.New(ctx, endpoint, rpcclient.Options{RequestTimeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("rpc new: %w", err)
	}
	if err := cli.Init(); err != nil {
		return nil, fmt.Errorf("rpc init: %w", err)
	}
	return &Client{rpc: cli, inv: invoker.New(cli, nil)}, nil
}

// call runs a read-only invoke and returns the result stack on HALT.
func (c *Client) call(hash util.Uint160, method string, params ...interface{}) ([]stackitem.Item, error) {
	res, err := c.inv.Call(hash, method, params...)
	if err != nil {
		return nil, fmt.Errorf("invoke %s: %w", method, err)
	}
	if res.State != vmstate.Halt.String() {
		return nil, fmt.Errorf("invoke %s faulted: %s", method, faultText(res))
	}
	return res.Stack, nil
}

func faultText(res *result.Invoke) string {
	if res.FaultException != "" {
		return res.FaultException
	}
	return res.State
}

// ---- hash / address parsing (input validation) ----

// ParseScriptHash parses a Neo N3 contract/script hash in the conventional
// 0x-prefixed form shown by explorers, NeoLine and contract manifests. That form
// is Neo's little-endian internal encoding (util.Uint160.StringLE), so it MUST be
// decoded with DecodeStringLE. DecodeStringBE yields the byte-reversed hash, and
// every contract call then targets a non-existent address (SYSCALL
// System.Contract.Call -> "called contract <reversed> not found").
func ParseScriptHash(s string) (util.Uint160, error) {
	return util.Uint160DecodeStringLE(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x"))
}

// ParseAddress parses a Neo N3 wallet address ("N..."), validating its version
// byte and checksum, into a script hash.
func ParseAddress(s string) (util.Uint160, error) {
	return address.StringToUint160(strings.TrimSpace(s))
}

// ParseTxHash parses a transaction hash in the conventional 0x-prefixed display
// form (explorers, NeoLine). Like script hashes, that form is the little-endian
// StringLE encoding, so decode with DecodeStringLE (not BE).
func ParseTxHash(s string) (util.Uint256, error) {
	return util.Uint256DecodeStringLE(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x"))
}

// ---- small stack-item helpers ----

func itemBytes(it stackitem.Item) ([]byte, error) { return it.TryBytes() }

func itemBig(it stackitem.Item) (*big.Int, error) { return it.TryInteger() }

func itemBool(it stackitem.Item) (bool, error) { return it.TryBool() }

func itemString(it stackitem.Item) (string, error) {
	b, err := it.TryBytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func itemArray(it stackitem.Item) ([]stackitem.Item, error) {
	if it == nil {
		return nil, fmt.Errorf("nil stack item")
	}
	if it.Type() == stackitem.AnyT {
		// Null — treated as empty/not-found by callers.
		return nil, nil
	}
	arr, ok := it.Value().([]stackitem.Item)
	if !ok {
		return nil, fmt.Errorf("stack item is %s, not an array", it.Type())
	}
	return arr, nil
}

func single(stack []stackitem.Item) (stackitem.Item, error) {
	if len(stack) == 0 {
		return nil, fmt.Errorf("empty result stack")
	}
	return stack[len(stack)-1], nil
}
