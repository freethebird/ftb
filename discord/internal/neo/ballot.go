package neo

import (
	"fmt"
	"math/big"

	"github.com/nspcc-dev/neo-go/pkg/util"
)

// CaseStatus mirrors the accusation ballot's status enum.
type CaseStatus int

const (
	CaseOpen       CaseStatus = 0
	CaseLiquidated CaseStatus = 1
	CaseDismissed  CaseStatus = 2
)

func (s CaseStatus) String() string {
	switch s {
	case CaseLiquidated:
		return "liquidated"
	case CaseDismissed:
		return "dismissed"
	default:
		return "open"
	}
}

// Case is one accusation on the ballot (separate from protocol governance).
//
// INTERFACE (getCase(caseId) -> Struct/Array):
//
//	[0] accusedBondId  Integer     the bond under accusation
//	[1] reasonCode     Integer     machine reason (see handoff.Reason*)
//	[2] evidenceHash   ByteString  32-byte sha-256 of the off-chain evidence bundle
//	[3] forVotes       Integer     votes to LIQUIDATE
//	[4] againstVotes   Integer     votes to DISMISS
//	[5] status         Integer     0 open / 1 liquidated / 2 dismissed
//	[6] deadline       Integer     unix-ms voting deadline
//	[7] threshold      Integer     votes required to resolve (1 bonded member = 1 vote)
type Case struct {
	ID            *big.Int
	AccusedBondID *big.Int
	ReasonCode    int
	EvidenceHash  []byte
	ForVotes      *big.Int
	AgainstVotes  *big.Int
	Status        CaseStatus
	Deadline      *big.Int
	Threshold     *big.Int
}

// GetCase reads one case by id. found=false (nil error) when absent.
func (c *Client) GetCase(ballot util.Uint160, method string, caseID *big.Int) (*Case, bool, error) {
	stack, err := c.call(ballot, method, caseID)
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
		return nil, false, nil
	}
	if len(arr) < 8 {
		return nil, false, fmt.Errorf("getCase returned %d fields, want 8", len(arr))
	}
	cs := &Case{ID: new(big.Int).Set(caseID)}
	if cs.AccusedBondID, err = itemBig(arr[0]); err != nil {
		return nil, false, fmt.Errorf("case accusedBondId: %w", err)
	}
	reason, err := itemBig(arr[1])
	if err != nil {
		return nil, false, fmt.Errorf("case reason: %w", err)
	}
	cs.ReasonCode = int(reason.Int64())
	if cs.EvidenceHash, err = itemBytes(arr[2]); err != nil {
		return nil, false, fmt.Errorf("case evidence: %w", err)
	}
	if cs.ForVotes, err = itemBig(arr[3]); err != nil {
		return nil, false, fmt.Errorf("case forVotes: %w", err)
	}
	if cs.AgainstVotes, err = itemBig(arr[4]); err != nil {
		return nil, false, fmt.Errorf("case againstVotes: %w", err)
	}
	statusBig, err := itemBig(arr[5])
	if err != nil {
		return nil, false, fmt.Errorf("case status: %w", err)
	}
	cs.Status = CaseStatus(statusBig.Int64())
	if cs.Deadline, err = itemBig(arr[6]); err != nil {
		return nil, false, fmt.Errorf("case deadline: %w", err)
	}
	if cs.Threshold, err = itemBig(arr[7]); err != nil {
		return nil, false, fmt.Errorf("case threshold: %w", err)
	}
	return cs, true, nil
}

// ListOpenCases returns the ids of all open cases (listOpenCases() -> []Integer).
func (c *Client) ListOpenCases(ballot util.Uint160, method string) ([]*big.Int, error) {
	stack, err := c.call(ballot, method)
	if err != nil {
		return nil, err
	}
	top, err := single(stack)
	if err != nil {
		return nil, err
	}
	arr, err := itemArray(top)
	if err != nil {
		return nil, err
	}
	out := make([]*big.Int, 0, len(arr))
	for i, it := range arr {
		n, err := itemBig(it)
		if err != nil {
			return nil, fmt.Errorf("open case id #%d: %w", i, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// IsBondedVoter reports whether a wallet currently holds an active bond and is
// therefore eligible to file cases and vote (isBondedVoter(scriptHash) -> Bool).
func (c *Client) IsBondedVoter(ballot util.Uint160, method string, wallet util.Uint160) (bool, error) {
	stack, err := c.call(ballot, method, wallet)
	if err != nil {
		return false, err
	}
	top, err := single(stack)
	if err != nil {
		return false, err
	}
	return itemBool(top)
}
