// Package mru keeps a small most-recently-used ledger of jump targets so the
// grid can default to usage order. Persisted as JSON in the plugin state dir.
package mru

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Ledger maps a stable target key (e.g. "workspace:w15") to a monotonically
// increasing usage counter. Higher = more recently used.
type Ledger struct {
	path string
	Use  map[string]uint64 `json:"use"`
	Seq  uint64            `json:"seq"`
}

// Load reads the ledger, tolerating a missing or corrupt file — either way
// the result is fresh state and the next save heals the file.
func Load(stateDir string) *Ledger {
	l := &Ledger{Use: map[string]uint64{}}
	if stateDir == "" {
		return l
	}
	l.path = filepath.Join(stateDir, "mru.json")
	data, err := os.ReadFile(l.path)
	if err != nil {
		return l
	}
	_ = json.Unmarshal(data, l)
	if l.Use == nil {
		l.Use = map[string]uint64{}
	}
	return l
}

// Touch records a jump to key.
func (l *Ledger) Touch(key string) {
	l.Seq++
	l.Use[key] = l.Seq
	l.save()
}

// Order sorts items by descending recency, preserving the input order as the
// tie-breaker (stable). keyOf extracts the ledger key for an item.
func Order[T any](l *Ledger, items []T, keyOf func(T) string) []T {
	out := make([]T, len(items))
	copy(out, items)
	orig := make(map[string]int, len(items))
	for i, it := range items {
		if _, seen := orig[keyOf(it)]; !seen {
			orig[keyOf(it)] = i
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		ka, kb := keyOf(out[a]), keyOf(out[b])
		// Recency desc; equal recency keeps the original order (stable).
		if ua, ub := l.Use[ka], l.Use[kb]; ua != ub {
			return ua > ub
		}
		return orig[ka] < orig[kb]
	})
	return out
}

// save persists the ledger atomically (temp file + rename), so a crash
// mid-write can never leave a truncated mru.json behind.
func (l *Ledger) save() {
	if l.path == "" {
		return
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return
	}
	writeAtomic(l.path, data)
}

// writeAtomic replaces path with data via a temp file + rename. Duplicated in
// internal/jump (kept local so each package stays self-contained).
func writeAtomic(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
