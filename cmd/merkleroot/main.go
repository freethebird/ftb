// merkleroot reads the published snapshot.csv, applies the community-chosen ratios to
// turn each holder's pre-exploit FLM and FLOCKS into an FTB entitlement, and emits the
// Merkle root plus one proof per address. The root goes into FtbMigrator.Configure; the
// proofs are what holders pass to Claim. Uses pkg/merkle, the same leaf format as the
// contract, so the root is verifiable by anyone who re-runs this.
//
//	go run ./cmd/merkleroot -snapshot snapshot.csv -r1 1 -r2 4 -out proofs.json
//
// -r1 is FTB per 1 FLM (base units), -r2 is FTB per 1 FLOCKS. Both accept fractions
// like 3/2 or decimals like 1.5. Entitlements are floored to whole base units.
package main

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"

	"github.com/freethebird/ftb/pkg/merkle"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/util"
)

type entry struct {
	Address string   `json:"address"`
	Amount  string   `json:"amount"`
	Proof   []string `json:"proof"`
}
type output struct {
	Root     string  `json:"root"`
	TotalFTB string  `json:"total_ftb"`
	Count    int     `json:"count"`
	Entries  []entry `json:"entries"`
}

func parseAccount(s string) (util.Uint160, error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "0x"):
		return util.Uint160DecodeStringBE(s[2:])
	case strings.HasPrefix(s, "N"):
		return address.StringToUint160(s)
	default:
		return util.Uint160DecodeStringLE(s)
	}
}

func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		fmt.Fprintf(os.Stderr, "bad ratio: %q\n", s)
		os.Exit(2)
	}
	return r
}

type item struct {
	acc util.Uint160
	raw string
	amt *big.Int
}

// aggregate folds snapshot CSV rows (header row included) into per-account entitlements:
// ftb = floor(flm*r1 + flocks*r2). Duplicates and re-encodings of one account collapse
// into a single leaf (FTB-05); negative balances are rejected and counted as skipped
// (FTB-16: the caller must refuse to build an incomplete tree unless explicitly
// overridden). warnf receives one line per skipped row. Zero entitlements produce no item.
func aggregate(rows [][]string, r1, r2 *big.Rat, warnf func(format string, args ...any)) (items []item, total *big.Int, skipped int, err error) {
	total = new(big.Int)
	if len(rows) < 2 {
		return nil, total, 0, fmt.Errorf("empty or unreadable snapshot")
	}
	head := map[string]int{}
	for i, h := range rows[0] {
		head[strings.TrimSpace(h)] = i
	}
	ciAddr, okA := head["address_or_scripthash"]
	ciFlm, okF := head["flm_raw"]
	ciFlk, okL := head["flocks_raw"]
	if !okA || !okF || !okL {
		return nil, total, 0, fmt.Errorf("snapshot must have columns: address_or_scripthash, flm_raw, flocks_raw")
	}
	// Aggregate per account so duplicate rows, or two encodings of the same script hash,
	// cannot create two leaves for one holder or overstate the cap total. Audit item FTB-05.
	accFlm := map[util.Uint160]*big.Int{}
	accFlk := map[util.Uint160]*big.Int{}
	rawOf := map[util.Uint160]string{}
	var order []util.Uint160
	for _, row := range rows[1:] {
		acc, perr := parseAccount(row[ciAddr])
		if perr != nil {
			warnf("skip bad address %q: %v", row[ciAddr], perr)
			skipped++
			continue
		}
		flm, _ := new(big.Int).SetString(strings.TrimSpace(row[ciFlm]), 10)
		flk, _ := new(big.Int).SetString(strings.TrimSpace(row[ciFlk]), 10)
		if flm == nil {
			flm = new(big.Int)
		}
		if flk == nil {
			flk = new(big.Int)
		}
		if flm.Sign() < 0 || flk.Sign() < 0 {
			warnf("skip negative balance for %q", row[ciAddr])
			skipped++
			continue // a snapshot balance is never negative; reject rather than distort the tree
		}
		if _, seen := accFlm[acc]; !seen {
			accFlm[acc] = new(big.Int)
			accFlk[acc] = new(big.Int)
			rawOf[acc] = strings.TrimSpace(row[ciAddr])
			order = append(order, acc)
		}
		accFlm[acc].Add(accFlm[acc], flm)
		accFlk[acc].Add(accFlk[acc], flk)
	}
	for _, acc := range order {
		// ftb = floor(flm*r1 + flocks*r2)
		sum := new(big.Rat).Add(
			new(big.Rat).Mul(r1, new(big.Rat).SetInt(accFlm[acc])),
			new(big.Rat).Mul(r2, new(big.Rat).SetInt(accFlk[acc])),
		)
		amt := new(big.Int).Quo(sum.Num(), sum.Denom()) // floor for non-negative
		if amt.Sign() <= 0 {
			continue // zero entitlement, nothing to claim
		}
		items = append(items, item{acc, rawOf[acc], amt})
		total.Add(total, amt)
	}
	return items, total, skipped, nil
}

func main() {
	snap := flag.String("snapshot", "snapshot.csv", "snapshot CSV from snapshot_tool.py")
	r1s := flag.String("r1", "1", "FTB per 1 FLM base unit")
	r2s := flag.String("r2", "1", "FTB per 1 FLOCKS base unit")
	out := flag.String("out", "proofs.json", "output proofs JSON")
	allowSkip := flag.Bool("allow-skip", false, "continue past malformed rows (DANGEROUS: silently excludes holders, FTB-16)")
	flag.Parse()

	r1, r2 := mustRat(*r1s), mustRat(*r2s)

	f, err := os.Open(*snap)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		fmt.Fprintln(os.Stderr, "empty or unreadable snapshot")
		os.Exit(1)
	}

	items, total, skipped, err := aggregate(rows, r1, r2, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// FTB-16: a skipped row produces a root and cap that agree with each other but quietly
	// exclude that holder forever. Refuse to emit the tree unless explicitly overridden.
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: %d snapshot rows were skipped (see above)\n", skipped)
		if !*allowSkip {
			fmt.Fprintln(os.Stderr, "refusing to build an incomplete tree; fix the rows or pass -allow-skip")
			os.Exit(1)
		}
	}

	// deterministic order (by address BE) so the root is reproducible
	sort.Slice(items, func(i, j int) bool {
		return strings.Compare(items[i].acc.StringBE(), items[j].acc.StringBE()) < 0
	})

	leaves := make([][]byte, len(items))
	for i, it := range items {
		leaves[i] = merkle.Leaf(it.acc, it.amt)
	}
	root, proofs := merkle.Build(leaves)

	o := output{Root: hex.EncodeToString(root), TotalFTB: total.String(), Count: len(items)}
	for i, it := range items {
		ph := make([]string, len(proofs[i]))
		for j, p := range proofs[i] {
			ph[j] = hex.EncodeToString(p)
		}
		o.Entries = append(o.Entries, entry{Address: it.raw, Amount: it.amt.String(), Proof: ph})
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("root       : 0x%s\n", o.Root)
	fmt.Printf("addresses  : %d\n", o.Count)
	fmt.Printf("total FTB  : %s base units\n", o.TotalFTB)
	fmt.Printf("wrote      : %s\n", *out)
	fmt.Printf("IMPORTANT  : set ftb_token maxSupply to EXACTLY %s base units before launch (audit FTB-01).\n", o.TotalFTB)
	fmt.Println("Set this root in FtbMigrator.Configure, and publish proofs.json so anyone can verify.")
}
