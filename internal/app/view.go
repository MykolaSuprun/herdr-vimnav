package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// layout describes the master-detail split: a master list on the left, a
// live-preview panel filling the rest.
type layout struct {
	listW   int // master list column width (cells; the 1-cell gap is not included)
	detailW int // detail panel width (cells; border included)
	bodyH   int // body rows between the header and footer lines
}

// Master list width bounds for the auto mode; a master_width config value
// overrides them. Widened with the per-letter code column (4 cells): the
// label zone must stay useful even at the floor.
const (
	listMinW   = 28
	listMaxW   = 42
	detailMinW = 24
)

func (m Model) layout() layout {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		w, h = 80, 24
	}

	listW := m.cfg.MasterWidth
	if listW <= 0 { // auto: about a third of the width, clamped
		listW = clamp(w/3, listMinW, listMaxW)
	}
	detailW := w - listW - 1 // one gap column between list and panel
	if detailW < detailMinW {
		detailW = detailMinW
		listW = clamp(w-detailW-1, 8, listW)
	}
	bodyH := h - 2
	if bodyH < 3 {
		bodyH = 3
	}
	return layout{listW: listW, detailW: detailW, bodyH: bodyH}
}

// listRows returns how many master-list rows fit the body.
func (m Model) listRows() int { return m.layout().bodyH }

// detailLines returns how many preview lines fit inside the detail panel
// (its two border rows and the title line subtracted).
func (m Model) detailLines() int {
	n := m.layout().bodyH - 3
	if n < 1 {
		n = 1
	}
	return n
}

// previewReadLines is the pane.read window: the detail height plus headroom
// so blank-line squeezing still fills the panel.
func (m Model) previewReadLines() int {
	n := m.detailLines()
	if m.cfg.SqueezeBlankLines {
		n += 8
	}
	return n
}

// numColW sizes the number column from the live data: 3 cells normally, 4
// when a live cumulative workspace number needs them. Deriving the width
// keeps a 4-digit number from spilling past the column and shoving the
// pinned rail, without stealing a cell from labels at ordinary widths.
func (m Model) numColW() int {
	wide := 3
	for _, it := range m.items {
		if d := len(strconv.Itoa(it.Number)); d > wide {
			wide = d
		}
	}
	return clamp(wide, 3, 4)
}

// clampScroll keeps the list scroll offset in range.
func (m *Model) clampScroll() {
	maxScroll := len(m.items) - m.listRows()
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.scroll = clamp(m.scroll, 0, maxScroll)
}

// ensureVisible scrolls the list so the cursor row is inside the viewport.
func (m *Model) ensureVisible() {
	rows := m.listRows()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+rows {
		m.scroll = m.cursor - rows + 1
	}
	m.clampScroll()
}

// --- view -------------------------------------------------------------------

var (
	headerStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle       = lipgloss.NewStyle().Faint(true)
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	onStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true)
	markerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#1e66f5")).Bold(true)
	focusedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#40a02b")).Bold(true)
	attentionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d20f39")).Bold(true)
)

// View implements tea.Model.
func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("loading…")
	}
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	var b strings.Builder

	// Header.
	title := "spaces"
	if m.mode == ModeAgents {
		title = "agents"
	}
	var attn int
	for _, it := range m.items {
		if it.AwaitingInput {
			attn++
		}
	}
	header := headerStyle.Render("◆ " + title)
	if attn > 0 {
		header += attentionStyle.Render(" · " + strconv.Itoa(attn) + " attention")
	}
	if m.filterActive {
		header += "  " + dimStyle.Render("search: ") + m.search + "█"
	} else if m.codeBuf != "" {
		header += "  " + onStyle.Render("code: "+m.codeBuf+"…")
	}
	if m.errMsg != "" {
		header += "  " + errStyle.Render(m.errMsg)
	}
	b.WriteString(header)
	b.WriteString("\n")

	// Body: master list + detail panel, joined side by side.
	l := m.layout()
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.renderList(l), " ", m.renderDetail(l)))
	b.WriteString("\n")

	// Footer.
	scroll := ""
	if len(m.items) > l.bodyH {
		last := min(m.scroll+l.bodyH, len(m.items))
		scroll = dimStyle.Render("  [rows " + strconv.Itoa(m.scroll+1) + "-" + strconv.Itoa(last) + "/" + strconv.Itoa(len(m.items)) + "]")
	}
	b.WriteString(dimStyle.Render("ctrl+j/k move · / search · code select · enter switch · esc close") + scroll)
	return b.String()
}

// renderList renders the master column: one row per item, padded to an exact
// height so the layout never jitters.
func (m Model) renderList(l layout) string {
	rows := make([]string, 0, l.bodyH)
	if len(m.items) == 0 {
		rows = append(rows, dimStyle.Render(truncateANSI("  (no items)", l.listW)))
	}
	last := min(m.scroll+l.bodyH, len(m.items))
	for i := m.scroll; i < last; i++ {
		rows = append(rows, m.renderListRow(i, l.listW))
	}
	for len(rows) < l.bodyH {
		rows = append(rows, strings.Repeat(" ", l.listW))
	}
	return strings.Join(rows, "\n")
}

// renderListRow renders one master-list entry as aligned columns:
//
//	▌  chip  num  label … cwd   dot flag
//
// marker(2) chips(4 — badge exactly as wide as the code, column wash-padded)
// number(3-4, right-aligned, dim — data-sized via numColW) label(flex),
// then a dim cwd segment when both fit, ending in a pinned right rail
// (status dot + focused flag) so attention state scans down one straight
// line. The selected row carries a faint background wash blended toward the
// item's accent hue.
func (m Model) renderListRow(idx, w int) string {
	it := m.items[idx]
	selected := idx == m.cursor
	accent := accentFor(it)
	wash := ""
	if selected {
		wash = washColor(accent)
	}

	// Column widths: the number column is data-sized (3 normally, 4 when a
	// live cumulative number needs it) so it never spills into the rail.
	const chipW = 4 // badge-per-letter codes: up to 2 badges + gap (triples run tight)
	numW := m.numColW()
	railW := 3                        // status dot + gap + focused flag
	headW := 2 + chipW + 1 + numW + 1 // marker + chips + gap + number + gap
	tailW := 1 + railW                // gap before the rail

	code := ""
	if idx < len(m.table) {
		code = string(m.table[idx])
	}

	marker := withWash(lipgloss.NewStyle(), wash).Render("  ")
	if selected {
		marker = withWash(markerStyle, wash).Render("▌ ")
	}
	chip := chips(code, accent)
	if gap := chipW - lipgloss.Width(chip); gap > 0 {
		chip += withWash(lipgloss.NewStyle(), wash).Render(strings.Repeat(" ", gap))
	}

	num := strings.Repeat(" ", numW)
	if it.Number > 0 {
		s := strconv.Itoa(it.Number)
		if len(s) > numW { // 5+ digits: low digits win — alignment over identity
			s = s[len(s)-numW:]
		}
		num = strings.Repeat(" ", numW-len(s)) + s
	}
	num = withWash(dimStyle, wash).Render(num)

	// Flexible middle: label, then the cwd as a dim disambiguator when both
	// fit (narrow lists keep the full label and drop the cwd).
	labelZone := w - headW - tailW
	if labelZone < 4 {
		labelZone = 4
	}
	body := withWash(accentStyle(accent, selected), wash).Render(truncateANSI(it.Label, labelZone))
	if it.Cwd != "" && lipgloss.Width(it.Label)+1+lipgloss.Width(it.Cwd) <= labelZone {
		body += withWash(dimStyle, wash).Render(" " + it.Cwd)
	}
	if f := labelZone - lipgloss.Width(body); f > 0 {
		body += withWash(lipgloss.NewStyle(), wash).Render(strings.Repeat(" ", f))
	}

	// Right rail: status dot, gap, focused flag — never moves.
	dot := withWash(
		lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor(it.Status))),
		wash,
	).Render("●")
	flag := withWash(lipgloss.NewStyle(), wash).Render(" ")
	if it.Focused {
		flag = withWash(focusedStyle, wash).Render("●")
	}
	rail := withWash(lipgloss.NewStyle(), wash).Render(" ") + dot +
		withWash(lipgloss.NewStyle(), wash).Render(" ") + flag

	row := marker + chip + " " + num + " " + body + rail
	if gap := w - lipgloss.Width(row); gap > 0 {
		row += withWash(lipgloss.NewStyle(), wash).Render(strings.Repeat(" ", gap))
	}
	return row
}

// renderDetail renders the live preview panel for the selected item at an
// exact bodyH height so the two columns stay aligned.
func (m Model) renderDetail(l layout) string {
	inner := l.detailW - 4 // border (2) + padding (2)
	if inner < 4 {
		inner = 4
	}
	want := l.bodyH - 2 // content rows inside the border
	if want < 1 {
		want = 1
	}
	accent := "#6c6f85" // neutral until an item is selected
	var lines []string

	if len(m.items) == 0 {
		lines = append(lines, dimStyle.Render("(no items)"))
	} else {
		it := m.items[m.cursor]
		accent = accentFor(it)
		code := ""
		if m.cursor < len(m.table) {
			code = string(m.table[m.cursor])
		}

		// Title: chip + label (accent) + dim meta. Falls back to a plain
		// truncated line when the parts would overflow the inner width.
		plain := pad(code, 2) + " " + it.Label + " · " + it.Status + " · " + it.Kind
		var title string
		if lipgloss.Width(plain) > inner {
			title = accentStyle(accent, true).Render(truncateANSI(plain, inner))
		} else {
			title = chips(code, accent) + " " +
				accentStyle(accent, true).Render(it.Label) +
				dimStyle.Render(" · "+it.Status+" · "+it.Kind)
		}
		lines = append(lines, title)

		body := previewBody(m.previews[it.PaneID], want-1, m.cfg.SqueezeBlankLines)
		if len(body) == 0 {
			hint := "(no preview yet)"
			if it.PaneID == "" {
				hint = "(no pane to preview)"
			}
			body = []string{"", dimStyle.Render(hint)}
		}
		for _, raw := range body {
			lines = append(lines, ansiSafe(truncateANSI(cleanLineANSI(raw), inner)))
		}
	}
	for len(lines) < want {
		lines = append(lines, "")
	}
	if len(lines) > want {
		lines = lines[:want]
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accent)).
		Padding(0, 1).
		Width(l.detailW).
		Render(strings.Join(lines, "\n"))
}

// catppuccinAccents are the identity hues for per-item accents. LATTE-family
// colors: saturated and readable on the user's light (latte) terminal — the
// earlier mocha set washed out on light backgrounds. Indexed by a stable
// hash of the item key, so a space keeps its color across opens.
var catppuccinAccents = []string{
	"#dc8a78", "#dd7878", "#ea76cb", "#8839ef", "#d20f39", "#e64553",
	"#fe640b", "#df8e1d", "#40a02b", "#179299", "#04a5e5", "#209fb5",
	"#1e66f5", "#7287fd",
}

// accentFor picks an item's identity accent: a deterministic Catppuccin hue
// derived from the item's stable key.
func accentFor(it Item) string {
	var h uint32 = 2166136261 // FNV-1a offset basis
	for _, r := range it.Key {
		h ^= uint32(r)
		h *= 16777619
	}
	return catppuccinAccents[int(h)%len(catppuccinAccents)]
}

// accentStyle is the helper for accent-tinted text (label, title).
func accentStyle(accent string, bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
	if bold {
		return s.Bold(true)
	}
	return s
}

// chipStyle is the jump-code badge: the accented background — softened
// toward the latte base (same hue, lower intensity) — with dark text.
// Full-intensity accent read as glare down a column of badges.
func chipStyle(accent string) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1e1e2e")).
		Background(lipgloss.Color(chipBg(accent))).
		Bold(true)
}

// chipBg softens an accent toward the latte base (55% of the way): the badge
// keeps its identity hue at ~45% intensity — readable dark text, no glare.
func chipBg(accent string) string {
	if _, _, _, ok := splitHexColor(accent); !ok {
		return "#eff1f5"
	}
	return blendHex(accent, "#eff1f5", 0.55)
}

// chips renders the code badge exactly as wide as the code: a single letter
// fills its 1-cell block (centered by construction — the old pad-to-2 badge
// lopsidedly left-hugged singles with a trailing colored blank), pairs sit
// tight ("uj") as one uninterrupted block, triples likewise ("uuu"). Each
// glyph occupies its own cell. Callers pad the 4-cell chip column with the
// row wash.
func chips(code, accent string) string {
	return chipStyle(accent).Render(code)
}

// withWash layers the selected-row wash background onto a segment style
// (empty wash = unchanged). Applied per segment, not by wrapping the finished
// row: inner SGR resets would otherwise strip a wrapping background.
func withWash(base lipgloss.Style, wash string) lipgloss.Style {
	if wash == "" {
		return base
	}
	return base.Background(lipgloss.Color(wash))
}

// washColor is the selected-row wash: the Catppuccin latte base faintly
// blended toward the item's accent hue. An explicit hex keeps the wash stable
// across terminal profiles; a broken accent falls back to the plain base.
func washColor(accent string) string {
	if _, _, _, ok := splitHexColor(accent); !ok {
		return "#eff1f5"
	}
	return blendHex("#eff1f5", accent, 0.12) // base, 12% toward the accent
}

// blendHex mixes two #rrggbb colors channel-wise: t=0 keeps a, t=1 returns b.
// A broken first color falls back to b.
func blendHex(a, b string, t float64) string {
	ar, ag, ab, ok := splitHexColor(a)
	if !ok {
		return b
	}
	br, bg, bb, _ := splitHexColor(b)
	mix := func(from, to int) int {
		return int(math.Round(float64(from)*(1-t) + float64(to)*t))
	}
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}

// splitHexColor parses "#rrggbb" into 0-255 components.
func splitHexColor(hex string) (r, g, b int, ok bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(n>>16) & 0xff, int(n>>8) & 0xff, int(n) & 0xff, true
}

// statusColor maps a herdr agent_status to its semantic color: working
// peach, blocked red, done green, idle blue, unknown/empty gray. LATTE-family
// hexes — saturated and readable on light terminals (mocha hexes washed out).
func statusColor(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "working":
		return "#fe640b"
	case "blocked":
		return "#d20f39"
	case "done":
		return "#40a02b"
	case "idle":
		return "#1e66f5"
	default:
		return "#6c6f85"
	}
}

// ansiSafe terminates any color state a preview line may hold so it cannot
// bleed into lipgloss padding or the panel frame.
// Idempotent: lines already ending in a reset are left untouched.
func ansiSafe(line string) string {
	const reset = "\x1b[0m"
	if strings.Contains(line, "\x1b[") && !strings.HasSuffix(line, reset) {
		return line + reset
	}
	return line
}

// --- helpers ----------------------------------------------------------------

// truncateANSI cuts a string to at most w display cells, adding an ellipsis.
// Escape sequences (SGR and others) are copied through verbatim (zero display
// width) while only printable runes consume the budget, so an escape is never
// cut in half — the single truncator for labels, titles, and previews; plain
// and styled input share one code path.
func truncateANSI(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b {
			// Copy the whole sequence atomically; escapes are zero-width.
			end := escapeEnd(rs, i)
			b.WriteRune(r)
			for j := i + 1; j < end; j++ {
				b.WriteRune(rs[j])
			}
			i = end - 1
			continue
		}
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 { // leave room for the ellipsis
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

// escapeEnd returns the index just past the escape sequence starting at rs[i]
// (rs[i] == 0x1b). Mirrors the sequence grammar used by stripANSI.
func escapeEnd(rs []rune, i int) int {
	if i+1 >= len(rs) {
		return i + 1
	}
	switch rs[i+1] {
	case '[': // CSI: consume until final byte 0x40–0x7E
		j := i + 2
		for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
			j++
		}
		return min(j+1, len(rs))
	case ']': // OSC: consume until BEL or ST
		j := i + 2
		for j < len(rs) {
			if rs[j] == 0x07 {
				return j + 1
			}
			if rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	default: // two-char escape
		return i + 2
	}
}

func pad(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// tailLines returns the last n non-empty-trailing lines of s.
func tailLines(s string, n int) []string {
	if n <= 0 {
		return nil
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// previewBody builds the raw preview lines for the detail panel from the pane
// snapshot. With squeeze enabled it trims leading/trailing blanks and
// collapses runs of blank lines to separators, so the fixed panel height
// shows more real content; otherwise it is a plain tail. Either way the
// result is at most n lines.
func previewBody(s string, n int, squeeze bool) []string {
	if n <= 0 {
		return nil
	}
	if !squeeze {
		return tailLines(s, n)
	}
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := make([]string, 0, len(all))
	blankRun := false
	for _, l := range all {
		if strings.TrimSpace(stripANSI(l)) == "" {
			if blankRun {
				continue
			}
			blankRun = true
			out = append(out, "")
			continue
		}
		blankRun = false
		out = append(out, l)
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > n {
		out = out[len(out)-n:]
		// The tail window may open on a separator blank: re-trim so the
		// panel never starts with a blank line.
		for len(out) > 0 && out[0] == "" {
			out = out[1:]
		}
	}
	return out
}

// cleanLineANSI keeps SGR (color/bold/dim) sequences but drops every other
// escape: cursor movement/erase CSI would corrupt the fixed panel layout, and
// OSC titles would leak. Also normalizes tabs and drops stray C0 controls.
// Callers append an SGR reset when they render a line that may contain kept
// codes.
func cleanLineANSI(s string) string {
	return stripC0(stripNonSGR(s))
}

// stripC0 drops C0 control characters (tab becomes spaces) while copying
// escape sequences through untouched. Pane output can carry BEL/BS/VT/FF/DEL
// mid-line; a stray backspace or bell corrupts the fixed panel layout just
// as a cursor move would. \n cannot occur here — callers split lines first.
func stripC0(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == 0x1b {
			end := escapeEnd(rs, i)
			for j := i; j < end; j++ {
				b.WriteRune(rs[j])
			}
			i = end - 1
			continue
		}
		switch {
		case rs[i] == '\t':
			b.WriteString("    ")
		case rs[i] < 0x20 || rs[i] == 0x7f:
			// dropped: BEL, BS, VT, FF, SO/SI, DEL and friends
		default:
			b.WriteRune(rs[i])
		}
	}
	return b.String()
}

// stripNonSGR removes all escape sequences except SGR (…m) ones.
func stripNonSGR(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r != 0x1b {
			b.WriteRune(r)
			continue
		}
		end := escapeEnd(rs, i)
		// Keep the sequence only when it is a CSI ending in 'm' (SGR).
		if end > i+2 && rs[i+1] == '[' && rs[end-1] == 'm' {
			for j := i; j < end; j++ {
				b.WriteRune(rs[j])
			}
		}
		i = end - 1
	}
	return b.String()
}

// stripANSI removes all escape sequences without eating plain text.
func stripANSI(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == 0x1b {
			i = escapeEnd(rs, i) - 1
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
