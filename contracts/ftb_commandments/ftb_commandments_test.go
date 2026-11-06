// Reference neotest suite for FtbCommandments: owner-only + sequential write-once inking,
// validation, immutable reads, and the one-way renounce that freezes the record forever.
package ftbcommandments_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func deploy(t *testing.T) (*neotest.Executor, *neotest.ContractInvoker) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_commandments.go", "ftb_commandments.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())
	return e, e.NewInvoker(c.Hash, owner)
}

func sampleBallot() []byte {
	b := make([]byte, 20)
	for i := range b {
		b[i] = byte(i + 1)
	}
	return b
}

func TestInkReadAndCount(t *testing.T) {
	_, cmd := deploy(t)
	cmd.Invoke(t, stackitem.Make(0), "count")
	cmd.Invoke(t, stackitem.Null{}, "ink",
		1, "FTB claim window?", sampleBallot(),
		[]any{"6 months", "1 year"}, []any{246039807, 721934}, 1, "6 months")
	cmd.Invoke(t, stackitem.Make(1), "count")
	cmd.InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		if len(stack) != 1 {
			t.Fatalf("want 1 item, got %d", len(stack))
		}
		arr, ok := stack[0].Value().([]stackitem.Item)
		if !ok {
			t.Fatalf("commandment is not an array")
		}
		if len(arr) != 9 {
			t.Fatalf("want 9 fields, got %d", len(arr))
		}
		w, err := arr[5].TryInteger()
		if err != nil {
			t.Fatal(err)
		}
		if w.Int64() != 1 {
			t.Fatalf("winner = %d, want 1", w.Int64())
		}
		r, err := arr[6].TryBytes()
		if err != nil {
			t.Fatal(err)
		}
		if string(r) != "6 months" {
			t.Fatalf("ratified = %q", string(r))
		}
	}, "commandment", 1)
}

func TestInkOwnerOnly(t *testing.T) {
	e, cmd := deploy(t)
	other := e.NewAccount(t)
	e.NewInvoker(cmd.Hash, other).InvokeFail(t, "only owner", "ink",
		1, "Q", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
}

func TestInkSequentialWriteOnce(t *testing.T) {
	_, cmd := deploy(t)
	cmd.InvokeFail(t, "sequential", "ink", 2, "Q", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.Invoke(t, stackitem.Null{}, "ink", 1, "Q1", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.InvokeFail(t, "sequential", "ink", 1, "Q1b", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.InvokeFail(t, "sequential", "ink", 3, "Q3", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.Invoke(t, stackitem.Null{}, "ink", 2, "Q2", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 2, "b")
	cmd.Invoke(t, stackitem.Make(2), "count")
}

func TestInkValidation(t *testing.T) {
	_, cmd := deploy(t)
	bad := make([]byte, 19)
	cmd.InvokeFail(t, "question required", "ink", 1, "", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.InvokeFail(t, "20-byte hash", "ink", 1, "Q", bad, []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.InvokeFail(t, "2 options", "ink", 1, "Q", sampleBallot(), []any{"a"}, []any{1}, 1, "a")
	cmd.InvokeFail(t, "one tally each", "ink", 1, "Q", sampleBallot(), []any{"a", "b"}, []any{1}, 1, "a")
	cmd.InvokeFail(t, "winner out of range", "ink", 1, "Q", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 3, "a")
	cmd.InvokeFail(t, "ratified value required", "ink", 1, "Q", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "")
}

func TestRenounceFreezes(t *testing.T) {
	_, cmd := deploy(t)
	cmd.Invoke(t, stackitem.Null{}, "ink", 1, "Q1", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
	cmd.Invoke(t, stackitem.Null{}, "renounceOwnership")
	cmd.InvokeFail(t, "only owner", "ink", 2, "Q2", sampleBallot(), []any{"a", "b"}, []any{1, 2}, 1, "a")
}

func TestCommandmentMissing(t *testing.T) {
	_, cmd := deploy(t)
	cmd.InvokeFail(t, "no such commandment", "commandment", 9)
}
