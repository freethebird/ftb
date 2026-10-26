// Package handoff builds the keyless "sign-to-file" and "sign-to-vote" deep
// links that carry an accusation from Discord to the member-operated signing
// page. The bot NEVER signs or broadcasts: it only assembles a URL whose
// parameters a bonded member then signs with their own wallet (NeoLine /
// WalletConnect) on the signing page. No bot key exists anywhere.
//
// Parameters live in the URL FRAGMENT (after '#'), not the query string, so
// they are not sent to the IPFS gateway and stay off server logs — consistent
// with the stealth / privacy-forward posture. The signing page reads
// location.hash.
//
// URL contract (the signing page, web/git track, must honour this):
//
//	file a case:  {base}#/file-case?accused={bondId}&reason={code}&evidence={sha256hex}&nonce={hex}
//	vote:         {base}#/vote?case={caseId}&choice={liquidate|dismiss}
//
// The signing page builds the fileCase / vote invocation from these, has the
// member sign, broadcasts, and shows the resulting txid. The bot later reads the
// ballot contract to confirm the case/vote landed.
package handoff

import (
	"net/url"
	"strings"
)

// Reason is the machine reason code recorded on-chain with a case. It must stay
// in sync with the accusation-ballot contract's reason enum.
type Reason int

const (
	ReasonUnspecified Reason = 0
	ReasonSpamRate    Reason = 1 // high message rate
	ReasonSpamDup     Reason = 2 // repeated / near-duplicate content
	ReasonMassMention Reason = 3 // mass user/role mentions
	ReasonLinkFlood   Reason = 4 // link flooding
	ReasonInviteSpam  Reason = 5 // discord invite spam
	ReasonJoinBurst   Reason = 6 // post-join burst (fresh account dumping)
)

// String gives a short human label used in the Discord accusation card.
func (r Reason) String() string {
	switch r {
	case ReasonSpamRate:
		return "message-rate spam"
	case ReasonSpamDup:
		return "duplicate-content spam"
	case ReasonMassMention:
		return "mass mentions"
	case ReasonLinkFlood:
		return "link flooding"
	case ReasonInviteSpam:
		return "invite spam"
	case ReasonJoinBurst:
		return "post-join burst"
	default:
		return "unspecified"
	}
}

// Vote choices.
const (
	ChoiceLiquidate = "liquidate"
	ChoiceDismiss   = "dismiss"
)

// Builder constructs deep links against a configured signing-page base URL.
type Builder struct {
	base string
}

// NewBuilder returns a Builder. base is the signing page root, e.g.
// "https://freethebird.co/sign". A trailing slash is trimmed.
func NewBuilder(base string) Builder {
	return Builder{base: strings.TrimRight(strings.TrimSpace(base), "/")}
}

// Configured reports whether a signing-page base URL is set. When false the bot
// posts the accusation card without a clickable file button (standby).
func (b Builder) Configured() bool { return b.base != "" }

// FileCaseURL builds the member-signed "file this case" link.
func (b Builder) FileCaseURL(accusedBondID string, reason Reason, evidenceHex, nonce string) string {
	q := url.Values{}
	q.Set("accused", accusedBondID)
	q.Set("reason", itoa(int(reason)))
	q.Set("evidence", evidenceHex)
	if nonce != "" {
		q.Set("nonce", nonce)
	}
	return b.base + "#/file-case?" + q.Encode()
}

// VoteURL builds the member-signed vote link (liquidate or dismiss).
func (b Builder) VoteURL(caseID, choice string) string {
	q := url.Values{}
	q.Set("case", caseID)
	q.Set("choice", choice)
	return b.base + "#/vote?" + q.Encode()
}

func itoa(n int) string {
	// tiny, allocation-light int->string for small non-negative codes
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
