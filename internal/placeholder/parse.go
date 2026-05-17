package placeholder

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse splits a placeholder identifier into its semantic prefix and
// the per-prefix counter. It is the inverse of the minting format in
// mint.go: `STRIPE_KEY_3` → ("STRIPE_KEY", 3).
//
// The whole input must match Pattern(); a partial match (e.g. an
// identifier embedded in a larger token) returns an error so callers
// can decide whether to skip the candidate.
func Parse(name string) (prefix string, n int, err error) {
	if placeholderRE.FindString(name) != name {
		return "", 0, fmt.Errorf("placeholder: %q is not a valid placeholder identifier", name)
	}
	i := strings.LastIndexByte(name, '_')
	// LastIndexByte always finds the `_` separator: Pattern() requires
	// at least one underscore (the one before the digit run). i == 0
	// would mean the underscore is the first byte, which Pattern's
	// `[A-Z]` anchor rules out.
	num, err := strconv.Atoi(name[i+1:])
	if err != nil {
		return "", 0, fmt.Errorf("placeholder: counter parse on %q: %w", name, err)
	}
	return name[:i], num, nil
}

// ValidPrefix reports whether p is a legal placeholder prefix
// (`[A-Z][A-Z0-9_]+`). Empty prefixes and prefixes starting with a
// digit/underscore are rejected so the resulting placeholder always
// satisfies Pattern().
func ValidPrefix(p string) bool {
	if len(p) < 2 {
		return false
	}
	if p[0] < 'A' || p[0] > 'Z' {
		return false
	}
	for i := 1; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_':
		default:
			return false
		}
	}
	return true
}
