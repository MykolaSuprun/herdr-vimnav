package codes

import (
	"strings"
	"testing"
)

func TestTablePrefixFree(t *testing.T) {
	// The defining invariant: no code is a proper prefix of another code,
	// for a range of sizes and charsets.
	for _, n := range []int{1, 3, 6, 7, 12, 20, 42, 43, 100, 250} {
		table := Table(n, DefaultCharset)
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

func TestTableShape(t *testing.T) {
	// Custom charset: singles, then initiator-major pairs.
	table := Table(8, "abc") // initiators for "abc": u i o r e w
	want := []string{"a", "b", "c", "ua", "ub", "uc", "ia", "ib"}
	for i, w := range want {
		if string(table[i]) != w {
			t.Fatalf("index %d: got %q want %q", i, table[i], w)
		}
	}

	// Defaults: first six singles, then the u-family pairs.
	table = Table(14, DefaultCharset)
	want = []string{"j", "k", "l", "f", "d", "s", "uj", "uk", "ul", "uf", "ud", "us", "ij", "ik"}
	for i, w := range want {
		if string(table[i]) != w {
			t.Fatalf("index %d: got %q want %q", i, table[i], w)
		}
	}
}

func TestTableTriples(t *testing.T) {
	// 3 singles + 6*3 pairs = 21 codewords; index 21 starts the triples.
	table := Table(22, "abc")
	if got := string(table[21]); got != "uua" {
		t.Fatalf("first triple: got %q want %q", got, "uua")
	}
}

// A duplicated charset letter must not assign one code twice (the second row
// would be unreachable); Table dedupes defensively.
func TestTableDedupesCharset(t *testing.T) {
	table := Table(4, "jjk")
	if len(table) != 4 {
		t.Fatalf("Table returned %d codes", len(table))
	}
	want := []string{"j", "k", "uj", "uk"}
	for i, w := range want {
		if string(table[i]) != w {
			t.Fatalf("index %d: got %q want %q", i, table[i], w)
		}
	}
}

func TestInitiatorsDisjoint(t *testing.T) {
	init := Initiators(DefaultCharset)
	if init != "uiorew" {
		t.Fatalf("got %q", init)
	}
	for _, r := range init {
		if strings.ContainsRune(DefaultCharset, r) {
			t.Fatalf("initiator %q overlaps charset", r)
		}
	}

	// Charset that eats part of the initiator set stays disjoint.
	init = Initiators("uikl")
	if strings.ContainsAny(init, "uikl") {
		t.Fatalf("initiators %q overlap charset", init)
	}
}

func TestResolvePrefixFree(t *testing.T) {
	table := Table(8, "abc") // a b c ua ub uc ia ib
	cases := []struct {
		typed     string
		idx       int
		ambiguous bool
		ok        bool
	}{
		{"a", 0, false, true},     // complete, nothing extends: fires at once
		{"u", -1, false, true},    // live prefix: keep typing
		{"ub", 4, false, true},    // unique complete
		{"ux", -2, false, false},  // dead end
		{"aab", -2, false, false}, // dead end past a single
		{"x", -2, false, false},
	}
	for _, c := range cases {
		idx, amb, ok := Resolve(c.typed, table)
		if idx != c.idx || amb != c.ambiguous || ok != c.ok {
			t.Errorf("Resolve(%q) = (%d,%v,%v), want (%d,%v,%v)", c.typed, idx, amb, ok, c.idx, c.ambiguous, c.ok)
		}
	}
}
