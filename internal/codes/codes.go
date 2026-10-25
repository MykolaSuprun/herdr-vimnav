// Package codes assigns short jump codes (2 letters, then 3) to grid items,
// and resolves typed input against them.
//
// Codes are prefix-free by construction: no code is a proper prefix of
// another code. With a ten-letter pool that means:
//
//   - tier 1 — every ordered pair of DISTINCT pool letters ("sd", "sf", …):
//     10 × 9 = 90 codes. No single-letter codes exist, so a one-letter buffer
//     is always a live prefix.
//   - tier 2 — three-letter codes with a DOUBLED first letter ("ssd",
//     "ssf", …): 10 × 9 = 90 more. A pair never repeats a letter, so "ss" is
//     not a pair — and that is exactly what keeps tier 2 prefix-free: no
//     tier-1 code can be a prefix of a tier-2 code (whose first two letters
//     repeat). 180 codewords in total, all firing the moment they complete.
//
// Typed input therefore never needs a disambiguation timeout: a buffer is
// either a complete code (fire immediately), a live prefix (keep typing), or
// a dead end (reject).
package codes

import "strings"

// DefaultCharset is the ten-letter pool used for jump codes: the home row
// plus its neighbours (s d f j k l) and the inner top row (e r u i).
const DefaultCharset = "sdfjklerui"

// Code is a single assigned jump code.
type Code string

// uniqueRunes keeps the first occurrence of each rune in charset. Codes
// must differ between rows, so a duplicated letter ("jj") would otherwise
// assign one code twice and make the second row unreachable.
func uniqueRunes(charset string) []rune {
	seen := map[rune]bool{}
	out := make([]rune, 0, len(charset))
	for _, r := range charset {
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// Table assigns codes to n items drawn from charset. Prefix-free:
//
//   - the first len(letters)*(len(letters)-1) items get ordered pairs of
//     distinct letters ("sd", "sf", …) — 90 with the default pool;
//   - the rest get doubled-first triples ("ssd", "ssf", …) — another 90,
//     continuing after the pairs.
//
// Callers beyond 180 items get a truncated table: every code stays unique
// and prefix-free, and the overflow items simply have no code.
func Table(n int, charset string) []Code {
	if n <= 0 {
		return nil
	}
	letters := uniqueRunes(charset)
	if len(letters) < 2 {
		return nil // a single letter cannot yield a prefix-free pair tier
	}
	out := make([]Code, 0, n)

	// Tier 1: ordered pairs of distinct letters, first letter major.
	for _, a := range letters {
		for _, b := range letters {
			if a == b {
				continue // "ss" is reserved for the tier-2 doubled prefix
			}
			if len(out) == n {
				return out
			}
			out = append(out, Code(string([]rune{a, b})))
		}
	}
	// Tier 2: doubled first letter, then any other letter. Prefix-free
	// against tier 1 because no pair repeats its first letter.
	for _, a := range letters {
		for _, b := range letters {
			if a == b {
				continue
			}
			if len(out) == n {
				return out
			}
			out = append(out, Code(string([]rune{a, a, b})))
		}
	}
	return out
}

// Resolve maps typed input to a code index.
//
// Return values:
//   - idx >= 0: unique complete match, act now.
//   - idx == -1 && ambiguous: input is a proper prefix of longer codes AND is
//     itself a complete code; caller may wait for more input or timeout.
//     Prefix-free tables never produce this — it remains for non-prefix-free
//     custom charsets.
//   - idx == -2 (NotFound): no code equals or extends input; caller clears.
func Resolve(typed string, table []Code) (idx int, ambiguous bool, ok bool) {
	if typed == "" {
		return -2, false, false
	}
	exact := -1
	extends := false
	prefix := false
	for i, c := range table {
		s := string(c)
		switch {
		case s == typed && exact == -1:
			exact = i
		case strings.HasPrefix(s, typed):
			extends = true
		}
		if exact == -1 && strings.HasPrefix(typed, s) && s != typed {
			prefix = true
		}
	}
	switch {
	case exact >= 0 && extends:
		return -1, true, true // complete now, but longer codes exist
	case exact >= 0:
		return exact, false, true
	case extends:
		return -1, false, true // keep typing
	case prefix:
		return -2, false, false // dead end (typed past a complete code)
	default:
		return -2, false, false
	}
}
