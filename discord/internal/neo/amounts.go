package neo

import (
	"fmt"
	"math/big"
	"strings"
)

// ParseFixed converts a decimal amount string in whole token units (e.g. "100"
// or "100.5") into base units given the token's decimals (FTB uses 8, like the
// Flamingo NEP-17 line it recycles). It is exact — no float — and rejects more
// fractional digits than the token supports.
func ParseFixed(amount string, decimals int) (*big.Int, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, fmt.Errorf("empty amount")
	}
	neg := strings.HasPrefix(amount, "-")
	if neg {
		return nil, fmt.Errorf("amount must be positive")
	}
	intPart, fracPart := amount, ""
	if i := strings.IndexByte(amount, '.'); i >= 0 {
		intPart, fracPart = amount[:i], amount[i+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > decimals {
		return nil, fmt.Errorf("amount %q has more than %d decimal places", amount, decimals)
	}
	// Right-pad the fractional part to `decimals` digits.
	fracPart += strings.Repeat("0", decimals-len(fracPart))
	digits := intPart + fracPart
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount %q", amount)
	}
	return n, nil
}

// FormatFixed renders base units back to a trimmed decimal string (for display
// in Discord messages). It never uses float.
func FormatFixed(base *big.Int, decimals int) string {
	if base == nil {
		return "0"
	}
	neg := base.Sign() < 0
	abs := new(big.Int).Abs(base)
	s := abs.String()
	if decimals == 0 {
		if neg {
			return "-" + s
		}
		return s
	}
	for len(s) <= decimals {
		s = "0" + s
	}
	intPart := s[:len(s)-decimals]
	fracPart := strings.TrimRight(s[len(s)-decimals:], "0")
	out := intPart
	if fracPart != "" {
		out += "." + fracPart
	}
	if neg {
		out = "-" + out
	}
	return out
}
