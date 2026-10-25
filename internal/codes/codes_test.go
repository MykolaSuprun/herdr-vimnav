package codes

import (
	"strings"
	"testing"
)

// DefaultPool is the ten-letter set the plugin ships with.
const defaultPool = "sdfjklerui"

func TestTablePrefixFree(t *testing.T) {
	// The defining invariant: no code is a proper prefix of another code,
	// across the single-letter→pair→triple transitions and beyond.
	for _, n := range []int{1, 2, 9, 10, 89, 90, 91, 150, 180} {
		table := Table(n, defaultPool)
		if len(table) != n {
			t.Fatalf("Table(%d) returned %d codes", n, len(table))
		}
		for i, a := range table {
			for j, b := range table {
				if i == j {
					continue
				}
				if strings.HasPrefix(string(b), string(a)) {
					t.Fatalf("n=%d: code %q is a proper prefix of %q", n, a, b)
				}
			}
		}
	}
}

func TestTablePairTierCovers90(t *testing.T) {
	table := Table(90, defaultPool)
	if len(table) != 90 {
		t.Fatalf("got %d codes, want 90", len(table))
	}
	seen := map[string]bool{}
	for _, c := range table {
		s := string(c)
		if len(s) != 2 {
			t.Fatalf("pair tier produced %q (len %d)", s, len(s))
		}
		if s[0] == s[1] {
			t.Fatalf("pair %q repeats a letter (reserved for tier 2)", s)
		}
		if seen[s] {
			t.Fatalf("duplicate code %q", s)
		}
		seen[s] = true
	}
	// First letter major, pool order ("sdfjklerui").
	if got := string(table[0]); got != "sd" {
		t.Fatalf("first code = %q, want sd", got)
	}
	if got := string(table[8]); got != "si" {
		t.Fatalf("9th code = %q, want si (last s-block pair)", got)
	}
	if got := string(table[9]); got != "ds" {
		t.Fatalf("10th code = %q, want ds (first d-block pair)", got)
	}
	if got := string(table[17]); got != "di" {
		t.Fatalf("18th code = %q, want di (last d-block pair)", got)
	}
}

func TestTableNoSingleLetterCodes(t *testing.T) {
	// The user's rule: never one letter — a 1-letter buffer must stay a live
	// prefix so the second keystroke always lands.
	for _, c := range Table(5, defaultPool) {
		if len(string(c)) < 2 {
			t.Fatalf("single-letter code %q in the table", c)
		}
	}
}

func TestTableTripleTierDoublesFirst(t *testing.T) {
	table := Table(92, defaultPool)
	if got := string(table[90]); got != "ssd" {
		t.Fatalf("first triple = %q, want ssd", got)
	}
	if got := string(table[91]); got != "ssf" {
		t.Fatalf("second triple = %q, want ssf", got)
	}
	// Every tier-2 code doubles its first letter; that is what keeps it
	// prefix-free against tier 1 (pairs never repeat a letter).
	for _, c := range table[90:] {
		s := string(c)
		if len(s) != 3 || s[0] != s[1] || s[1] == s[2] {
			t.Fatalf("bad triple %q", s)
		}
	}
}

func TestTableTruncatesBeyondCapacity(t *testing.T) {
	// 180 codewords with the default pool; beyond that, items get no code
	// rather than a duplicate.
	if got := len(Table(200, defaultPool)); got != 180 {
		t.Fatalf("capacity = %d, want 180", got)
	}
}

func TestTableDedupesCharset(t *testing.T) {
	// "sds" dedupes to "sd": pairs sd, ds, then the doubled triples ssd, dds.
	table := Table(4, "sds")
	want := []string{"sd", "ds", "ssd", "dds"}
	for i, w := range want {
		if string(table[i]) != w {
			t.Fatalf("index %d: got %q want %q", i, table[i], w)
		}
	}
}

func TestTableTinyPools(t *testing.T) {
	// A one-letter pool cannot form a prefix-free pair tier.
	if got := Table(3, "s"); got != nil {
		t.Fatalf("single-letter pool should yield no codes, got %v", got)
	}
	// An empty charset yields nothing.
	if got := Table(3, ""); got != nil {
		t.Fatalf("empty charset should yield no codes, got %v", got)
	}
}

func TestResolvePairFirst(t *testing.T) {
	// 10 codes = the whole s-block; "d" then starts the next block.
	table := Table(10, defaultPool) // sd sf sj sk sl se sr su si ds
	cases := []struct {
		typed     string
		idx       int
		ambiguous bool
		ok        bool
	}{
		{"s", -1, false, true},    // live prefix, never a complete code
		{"sd", 0, false, true},    // complete pair fires at once
		{"sf", 1, false, true},    // complete pair
		{"sx", -2, false, false},  // dead end (x not in pool)
		{"sdf", -2, false, false}, // dead end: no table code extends "sdf"
		{"d", -1, false, true},    // live prefix for the ds block
		{"ds", 9, false, true},    // completes the d-block pair
	}
	for _, c := range cases {
		idx, amb, ok := Resolve(c.typed, table)
		if idx != c.idx || amb != c.ambiguous || ok != c.ok {
			t.Errorf("Resolve(%q) = (%d,%v,%v), want (%d,%v,%v)", c.typed, idx, amb, ok, c.idx, c.ambiguous, c.ok)
		}
	}
}

func TestResolveTripleTier(t *testing.T) {
	table := Table(91, defaultPool) // 90 pairs + ssd
	if idx, amb, ok := Resolve("ss", table); idx != -1 || amb || !ok {
		t.Errorf("ss should be a live prefix for the triple tier, got (%d,%v,%v)", idx, amb, ok)
	}
	if idx, amb, ok := Resolve("ssd", table); idx != 90 || amb || !ok {
		t.Errorf("ssd should complete, got (%d,%v,%v)", idx, amb, ok)
	}
	if idx, _, ok := Resolve("sd", table); idx != 0 || !ok {
		t.Errorf("sd should still complete instantly, got (%d,%v)", idx, ok)
	}
}
