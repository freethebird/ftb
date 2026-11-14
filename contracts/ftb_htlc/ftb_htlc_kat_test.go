// Neo N3 side of the cross-chain differential KAT. Runs the shared vectors (kat/vectors.json,
// generated with evm/test/KatVectors.sol by kat/gen.py) through the N3 FtbHtlc and asserts the
// lock/claim/refund outcomes. The Neo X Foundry harness (evm/test/FtbHtlcKat.t.sol) runs the
// same vectors; both legs must settle every vector identically, which is what makes the two a
// single swap. The hash pin (sha256(preimage) == hashlock) proves the SHA-256 commitment is the
// same value the EVM leg uses.
//
// Reuses the sys harness and helpers from ftb_htlc_test.go. Run: go test ./contracts/ftb_htlc/
package ftbhtlc_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type katVector struct {
	Amount   int    `json:"amount"`
	Hashlock string `json:"hashlock"`
	Preimage string `json:"preimage"`
	Scenario string `json:"scenario"`
}

type katDoc struct {
	WrongPreimage string      `json:"wrongPreimage"`
	Vectors       []katVector `json:"vectors"`
}

const katWindow = 20 // blocks; refund vectors cross this timelock with GenerateNewBlocks

func katHex(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		t.Fatalf("kat hex %q: %v", s, err)
	}
	return b
}

func TestKatDifferential(t *testing.T) {
	raw, err := os.ReadFile("kat/vectors.json")
	if err != nil {
		t.Fatalf("read kat/vectors.json: %v", err)
	}
	var doc katDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse kat/vectors.json: %v", err)
	}
	if len(doc.Vectors) == 0 {
		t.Fatal("no kat vectors")
	}
	wrong := katHex(t, doc.WrongPreimage)

	for _, v := range doc.Vectors {
		v := v
		t.Run(fmt.Sprintf("%s-%d", v.Scenario, v.Amount), func(t *testing.T) {
			hl := katHex(t, v.Hashlock)
			pre := katHex(t, v.Preimage)

			// Hash pin: the on-chain hash matches the shared vector (identical to the Neo X leg).
			sum := sha256.Sum256(pre)
			if !bytes.Equal(sum[:], hl) {
				t.Fatalf("hash pin: sha256(preimage) != hashlock")
			}

			s := setup(t)
			s.mintTok(t, s.alice.ScriptHash(), v.Amount)
			s.lockTok(t, s.alice, v.Amount, hl, s.bob.ScriptHash(), s.top(t)+katWindow)
			s.stateOf(t, hl, 1) // LOCKED

			switch v.Scenario {
			case "claim":
				s.claimFail(t, s.other, hl, wrong, "claim: preimage mismatch")
				s.claim(t, s.other, hl, pre)
				s.stateOf(t, hl, 2) // CLAIMED
				s.tokBal(t, s.bob.ScriptHash(), v.Amount)
				s.tokBal(t, s.htlc, 0)
				s.claimFail(t, s.other, hl, pre, "claim: not locked")
			case "refund":
				s.refundFail(t, s.other, hl, "refund: timelock not reached")
				s.e.GenerateNewBlocks(t, katWindow+5)
				s.refund(t, s.other, hl)
				s.stateOf(t, hl, 3) // REFUNDED
				s.tokBal(t, s.alice.ScriptHash(), v.Amount)
				s.tokBal(t, s.htlc, 0)
			default:
				t.Fatalf("unknown scenario %q", v.Scenario)
			}
		})
	}
}
