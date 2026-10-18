// Package codes assigns short jump codes (1-3 letters) to grid items, and
// resolves typed input against them.
//
// Codes are prefix-free by construction: no code is a proper prefix of
// another code. Single letters come from the configured charset; every
// longer code starts with an "initiator" letter drawn from a disjoint set
// (the top-row mirror of the default charset). Typed input therefore never
// needs a disambiguation timeout: a buffer is either a complete code (fire
// immediately), a live prefix (keep typing), or a dead end (reject).
package codes

import "strings"

// DefaultCharset is the home-row-adjacent set used for single-letter codes.
const DefaultCharset = "jklfds"

// DefaultInitiators is the first-letter set for multi-letter codes: the
// top-row mirror of DefaultCharset (j→u k→i l→o f→r d→e s→w), so pair codes
// read as "the key above" + a single letter.
const DefaultInitiators = "uiorew"

// Code is a single assigned jump code.
type Code string

// Initiators returns the letters that may begin a multi-letter code for the
// given single-letter charset: DefaultInitiators minus any charset overlap,
// with a fallback alphabet for exotic charsets. Guaranteeing
// charset ∩ initiators = ∅ is what makes the table prefix-free.
func Initiators(charset string) string {
	used := map[rune]bool{}
	for _, r := range charset {
		used[r] = true
	}
	var b strings.Builder
	for _, r := range DefaultInitiators {
		if !used[r] {
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		return b.String()
	}
	for r := 'a'; r <= 'z'; r++ {
		if !used[r] {
			b.WriteRune(r)
		}
	}
	return b.String()
}

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
//   - the first len(charset) items get one letter;
//   - the next len(Initiators)*len(charset) get pairs (initiator + letter);
//   - anything beyond that gets triples (initiator + initiator + letter).
//
// With the defaults that is 6 + 36 + 216 codewords — far beyond any real
// layout.
func Table(n int, charset string) []Code {
	if n <= 0 || charset == "" {
		return nil
	}
	letters := uniqueRunes(charset)
	init := []rune(Initiators(charset))
	out := make([]Code, 0, n)

	// Single letters first.
	for _, r := range letters {
		if len(out) == n {
			return out
		}
		out = append(out, Code(string(r)))
	}
	// Then pairs: initiator + letter (initiator-major, so each initiator
	// block reads as a group).
	for _, a := range init {
		for _, b := range letters {
			if len(out) == n {
				return out
			}
			out = append(out, Code(string([]rune{a, b})))
		}
	}
	// Deep layouts only: triples, initiator + initiator + letter.
	for _, a := range init {
		for _, b := range init {
			for _, c := range letters {
				if len(out) == n {
					return out
				}
				out = append(out, Code(string([]rune{a, b, c})))
			}
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
		return -2, false, false // dead end (typed past a 1-letter code)
	default:
		return -2, false, false
	}
}
