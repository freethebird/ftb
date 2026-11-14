// Command ftb-solver is the reference FASP solver: it wires the Neo N3 and Neo X
// HTLC adapters behind the fasp.HTLC interface and drives the atomic-swap flow.
// The maker/taker orchestration and its safety checks live in pkg/fasp (Swap,
// VerifyLock, the timelock ladder); this binary wires concrete chain adapters
// into that interface. The `status` subcommand is the runnable read path (no
// keys); executing a swap leg requires a configured wallet on that chain.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/freethebird/ftb/pkg/chain/neon3"
	"github.com/freethebird/ftb/pkg/chain/neox"
	"github.com/freethebird/ftb/pkg/fasp"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "status":
		statusCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
	case "serve":
		serveCmd(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  ftb-solver status -chain <n3|neox> -rpc <url> -contract <addr> -hashlock <hex>")
	fmt.Fprintln(os.Stderr, "  ftb-solver run -config <swap.json> [-secret <hex>]")
	fmt.Fprintln(os.Stderr, "  ftb-solver serve -config <solver.json>")
	os.Exit(2)
}

func statusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	chain := fs.String("chain", "", "n3 or neox")
	rpc := fs.String("rpc", "", "RPC URL")
	contract := fs.String("contract", "", "FtbHtlc address")
	hlHex := fs.String("hashlock", "", "32-byte hashlock hex")
	_ = fs.Parse(args)

	hb, err := hex.DecodeString(*hlHex)
	if err != nil || len(hb) != 32 {
		fail("hashlock must be 32-byte hex")
	}
	var hl [32]byte
	copy(hl[:], hb)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leg, err := openLeg(ctx, *chain, *rpc, *contract)
	if err != nil {
		fail(err.Error())
	}

	rec, ok := leg.Get(hl)
	if !ok {
		fmt.Printf("%s: no swap for hashlock %x\n", leg.Name(), hl)
		return
	}
	fmt.Printf("%s swap %x\n", leg.Name(), hl)
	fmt.Printf("  state=%s\n", stateName(rec.State))
	fmt.Printf("  sender=%s recipient=%s\n", rec.Sender, rec.Recipient)
	fmt.Printf("  token=%s amount=%d timelock=%d\n", rec.Token, rec.Amount, rec.Timelock)
	if pre, ok := leg.Preimage(hl); ok {
		fmt.Printf("  preimage=%x (secret revealed)\n", pre)
	}
}

// openLeg builds a read-only adapter for the named chain behind fasp.HTLC.
func openLeg(ctx context.Context, chain, rpc, contract string) (fasp.HTLC, error) {
	switch chain {
	case "neox":
		return neox.New(ctx, neox.Config{RPCURL: rpc, Contract: contract})
	case "n3":
		return neon3.New(ctx, neon3.Config{RPCURL: rpc, Contract: contract})
	default:
		return nil, fmt.Errorf("chain must be n3 or neox, got %q", chain)
	}
}

func stateName(s fasp.SwapState) string {
	switch s {
	case fasp.StateLocked:
		return "LOCKED"
	case fasp.StateClaimed:
		return "CLAIMED"
	case fasp.StateRefunded:
		return "REFUNDED"
	default:
		return "NONE"
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "ftb-solver:", msg)
	os.Exit(1)
}
