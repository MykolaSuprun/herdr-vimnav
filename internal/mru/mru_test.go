package mru

import "testing"

func TestOrderByUsage(t *testing.T) {
	l := &Ledger{Use: map[string]uint64{}}
	l.Touch("b")
	l.Touch("c")
	items := []string{"a", "b", "c"}
	got := Order(l, items, func(s string) string { return s })
	// c (2), b (1), a (0)
	if got[0] != "c" || got[1] != "b" || got[2] != "a" {
		t.Fatalf("got %v", got)
	}
}

func TestOrderStableTieBreak(t *testing.T) {
	l := &Ledger{Use: map[string]uint64{}}
	items := []string{"a", "b", "c"}
	got := Order(l, items, func(s string) string { return s })
	for i, w := range items {
		if got[i] != w {
			t.Fatalf("stable order broken: %v", got)
		}
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := Load(dir)
	l.Touch("x")
	l2 := Load(dir)
	if l2.Use["x"] == 0 {
		t.Fatalf("lost usage after reload: %+v", l2)
	}
	if l2.Seq != 1 {
		t.Fatalf("seq = %d", l2.Seq)
	}
}
