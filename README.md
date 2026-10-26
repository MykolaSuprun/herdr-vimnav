# herdr-vimnav

**Master-detail switcher for [herdr](https://herdr.dev)** — pick a space or
agent from a live, preformatted list and watch its pane while you decide.

<p align="center"><img src="docs/screenshots/demo-jump-codes.gif" width="820" alt="Animated demo: sidebar jump-code badges, opening the picker, walking rows, filtering, typing codes, and switching"></p>

*Sidebar badges populate automatically as the layout changes. `prefix+w`
opens the picker: walk with `ctrl+j/k`, filter with `/`, or type a jump code
to select — `enter` switches, and the sidebar follows.*

A herdr plugin: a **master-detail switcher** for spaces (workspaces) and
agents — a list of items with the selected one's live pane preview
alongside — plus **jump codes**: every space/agent row in the native
sidebar carries a code badge (kept in sync automatically), and tabs and
plain panes carry codes too. `prefix+f` (or a direct chord like
`ctrl+shift+e` — "shift+prefix", see
[Keybindings](#keybindings)) opens a tiny popup that jumps to whatever
code you type, with a live listing of every target and its code.

Inside the picker: walk the list with `ctrl+j/k` (or arrows), fuzzy-search
with `/`, and type a code to select its item — `enter` switches.

```sh
herdr plugin install MykolaSuprun/herdr-vimnav
```

The plugin registers as `preview-grid`; bind `prefix+w` / `prefix+a` (see
[Install](#install) and [Keybindings](#keybindings) — no default keybinds ship.
Install verified on a foreign system: `herdr plugin install --yes
MykolaSuprun/herdr-vimnav` on Ubuntu 24.04 (upstream herdr 0.9.5, stock Go)
clones from GitHub, runs the manifest build, and registers every action, pane,
event, and the startup hook in ~11s.

## Demo

The selector in both modes — aligned code badges, cwd hints, the pinned
status rail, and the live preview panel. `prefix+w` — spaces (items owing
attention lead, MRU after):

<p align="center"><img src="docs/screenshots/spaces.png" width="820" alt="Preview Grid in spaces mode: master list with jump-code badges, attention count and cwd hints; live pane preview panel with ANSI colors"></p>

`prefix+a` — agents, same layout:

<p align="center"><img src="docs/screenshots/agents.png" width="820" alt="Preview Grid in agents mode: agent list with per-item accent badges and live preview"></p>

Assets are rendered from a throwaway herdr session (and `tools/demo-server`
for the static shots) — no live workspace names are ever published. To
regenerate: see [Development](#development).

## Features

- **Master list + live preview panel** — aligned columns per row: code badge
  exactly as wide as the code (every glyph centered in its own cell),
  right-aligned dim number, label, dim cwd segment (dedupes identical
  labels), and a pinned right rail (status dot + focused flag); the selected
  row carries a faint wash of its own accent hue and the header sums
  attention (`◆ spaces 13 · 2 attention`). The right panel shows the selected
  item's live pane preview full-height.
- **Colored ANSI previews** — the preview renders the pane's real colors (SGR
  kept; cursor/erase/OSC sequences stripped), so diffs, trees, and highlighted
  output are recognizable at a glance. Each line is reset-terminated so colors
  never bleed into the panel frame.
- **Cursor starts on the focused item** — the picker opens showing where you
  are; the focused row carries a green `●`, the cursor row a `▌` marker.
- **Instant selection preview** — moving the selection re-reads the selected
  pane immediately (on top of the periodic snapshot refresh), so the panel
  never lags behind the highlight.
- **The host space previews the pane behind the picker** — herdr focuses the
  overlay pane itself while it is open, so the picker resolves the space it
  runs in to the pane *underneath* (via its launch context), never previewing
  itself.
- **Per-item identity accent** — a stable hash of the item key picks a
  Catppuccin latte hue (saturated, readable on light terminals): the detail
  border is tinted, the jump code renders as a solid accent badge, and the
  label/title use the accent — so a space keeps the same color across opens
  (learnable landmarks).
- **Status dots** — each row starts with a `●` colored by state: peach
  `working`, red `blocked`, green `done`, blue `idle`, gray unknown/none.
- **Walk the list**: `ctrl+j/k` plus `↓`/`↑`, `pgup`/`pgdn`, `ctrl+u`/`ctrl+d`,
  `home`/`end`. Bare letters are reserved for codes (the code letters
  themselves are `s d f j k l e r u i`, which is why stepping uses ctrl chords).
- **Fuzzy search**: `/` opens the search field; type to filter, `esc` clears.
- **Select-only jump codes** — every row shows a 2-letter code from the
  ten-letter pool `sdfjklerui` (ordered pairs of distinct letters: 90 of
  them), extending to 3-letter codes with a doubled first letter (`ssd`,
  `ssf`, …) once pairs run out. Never a single letter, and prefix-free by
  construction, so a code fires the instant it completes — no ambiguity
  timeout. Typing a complete code moves the
  selection (the preview follows); `backspace` edits the code buffer;
  `enter` commits the switch. A code alone never focuses anything.
  Every row shows its **canonical code** — the same one its sidebar badge
  and the jump popup carry — so sorting (priority/MRU) and searching only
  hide or reorder rows, never relabel, and a code learned from the
  sidebar selects that exact target (clearing the filter when it hid it).
- **Sidebar jump codes** — the native sidebar's spaces/agents rows show a
  `$jump` badge per row (reported via `report_metadata`, source `user:jumpd`).
  Codes re-sync automatically on every layout change (create/close/move/rename/
  reorder/agent-detect) through the plugin's `[[startup]]` + `[[events]]`
  hooks — no daemon, no manual step. **Automatic after a one-time row-layout
  setup:** herdr renders `$jump` only in sidebar rows whose layout includes
  the token (herdr's defaults do not) — see [Sidebar badges](#sidebar-badges). `prefix+f` (or a direct chord) opens a
  tiny popup: type a code → focus moves; `esc` cancels, `backspace` edits.
  Same prefix-free scheme as the picker. Popups sized ~46x12 also show a
  **live listing** of every target.
- **Tabs and panes jump too** — the popup's table continues past spaces
  and agents: every **tab** and every **plain (non-agent) pane** gets a
  code from the same prefix-free sequence (spaces/agents keep exactly
  their badge codes; big layouts extend into 3-letter codes). Tabs have no
  sidebar row, so their codes live in the popup: it lists them (filtered
  as you type), `/` fuzzy-searches labels, `↑/↓` walk, `enter` commits.
  Panes jump *exactly* via `pane.focus` (herdr 0.9.5's schema — the online
  method table omits it); an older server falls back to focusing the
  pane's tab. Agent-hosting panes are excluded from the pane section —
  their agent entry already jumps there with stronger semantics
  (`agent.focus` marks the agent seen).
- **Tab codes live on the tabs** — open the jump popup and the active
  space's tabs are **retagged with their codes** (`wl 1`, `wf 2`); any
  fired code (or `esc`, or a crash — the next open unwinds it) restores
  your original labels. This is the only surface that works: herdr 0.9.5
  has **no tab-metadata surface** (`tab.*` is create/list/get/focus/
  rename/move/close — no `tab.report_metadata`), so the tab bar renders
  nothing but the tab's own label, and a polled `tab_bar_right` command
  segment (>=1 s interval) can never catch a popup that lives a few
  hundred milliseconds. Labels are saved to `tab-tags.json` before
  rewriting, so user-renamed tabs come back exactly as they were.
- **Priority order** — attention-first: agents that are `blocked`
  (approval/question dialog) or `done` (finished but not yet viewed) lead,
  both MRU-ordered; everything else — idle, working, unknown — interleaves
  in plain most-recently-used order. Recency comes from herdr's own
  `workspace.focused`/`pane.focused` event hooks (all navigation from any
  client is recorded, even while the picker is closed). `sort = "usage"` /
  `"natural"` restore the old orders. Status dots keep showing each state.
- **Theming** — Catppuccin latte-family palette; the detail border follows the
  selected item's identity accent.

## Requirements

- herdr `>= 0.9.3` (plugin pane + socket API, `report_metadata`, `[[startup]]` /
  `[[events]]` manifest hooks; verified against 0.9.3 and installed end-to-end
  on Ubuntu 24.04 with the upstream 0.9.5 binary)
- Go **only at install/build time**: any Go `>= 1.21` works — the build's
  `GOTOOLCHAIN=auto` fetches the go1.26 toolchain the module asks for (verified:
  stock Ubuntu Go 1.22 built the plugin). The produced binary is static
  (`CGO_ENABLED=0`), so nothing Go-related is needed at runtime.

## Install

From GitHub (builds automatically at install time — needs Go, see
[Requirements](#requirements)):

```sh
herdr plugin install MykolaSuprun/herdr-vimnav
```

On Nix — **preferred: [`nix-herdr-manager`](https://gitlab.com/mykolas-open/nix-herdr-manager)**,
the generic plugin manager. The plugin flake ships the contract it needs
(`passthru.herdrPluginId` + `share/herdr/plugins/<id>`), so the entry is just
the package:

```nix
{
  inputs.herdr-vimnav.url = "gitlab:mykolas-open/herdr-vimnav";
  inputs.nix-herdr-manager.url = "gitlab:mykolas-open/nix-herdr-manager";
  # …
  imports = [ inputs.nix-herdr-manager.homeManagerModules.default ];
  programs.herdr-hpm.plugins = [
    inputs.herdr-vimnav.packages.${pkgs.stdenv.hostPlatform.system}.default
  ];
}
```

The manager links plugin store paths at activation, prunes removed ones, and
puts each plugin's `bin` on `home.sessionPath` — so keybindings run
`herdr-preview-grid` by **name** (a store path would rot on every rebuild):

```toml
[[keys.command]]
key = "prefix+f"
type = "popup"
command = "herdr-preview-grid jump"
width = 46
height = 12
description = "jump to code"
```

Without the manager this flake also exports its own module
(`homeManagerModules.default`) for a single-plugin, no-extra-input install:

```nix
{
  inputs.herdr-vimnav.url = "gitlab:mykolas-open/herdr-vimnav"; # source of truth; github:MykolaSuprun/herdr-vimnav is the mirror
  imports = [ inputs.herdr-vimnav.homeManagerModules.default ];
  programs.herdrPreviewGrid = {
    enable = true;
    # optional — rendered into
    # ~/.config/herdr/plugins/config/preview-grid/config.toml
    # (empty = your file is left untouched)
    settings = {
      charset = "sdfjklerui";
      master_width = 0;
    };
  };
}
```

Both paths need no Go toolchain and no network on the host: the package is
built by Nix and `herdr plugin link` never runs the manifest `[[build]]`.

Without Home Manager at all, the package still works — it exposes
`share/herdr/plugins/preview-grid`:

```nix
home.activation.herdrPreviewGridLink = lib.hm.dag.entryAfter [ "linkGeneration" ] ''
  herdr plugin link ${inputs.herdr-vimnav.packages.${pkgs.system}.default}/share/herdr/plugins/preview-grid --enabled
'';
```

Verify the package and modules with `nix flake check` (builds the binary, runs
the Go tests, and evaluates the module against Home Manager).

Local development (no build step at link time; the plugin falls back to
`go run` if `./bin/herdr-preview-grid` is missing):

```sh
git clone https://github.com/MykolaSuprun/herdr-vimnav.git
cd herdr-vimnav
CGO_ENABLED=0 go build -o bin/herdr-preview-grid ./cmd/herdr-preview-grid
herdr plugin link "$PWD"
```

Then bind keys in your herdr config (plugin bindings live there, not in Nix):

```toml
[[keys.command]]
key = "prefix+w"
type = "plugin_action"
command = "preview-grid.spaces"
description = "switch space (preview grid)"

[[keys.command]]
key = "prefix+a"
type = "plugin_action"
command = "preview-grid.agents"
description = "switch agent (preview grid)"

# Sidebar jump codes (badges are maintained by the plugin's hooks; the
# sidebar renders them only after the one-time row-layout setup —
# see "Sidebar badges").
# The popup runs the built binary directly — see "Keybindings" for the path.
# 46x12 fits the live listing; 44x3 still works (one visible row).
[[keys.command]]
key = "prefix+f"
type = "popup"
command = "/path/to/herdr-preview-grid/bin/herdr-preview-grid jump"
width = 46
height = 12
description = "jump to code"
```

Reload with `prefix+shift+r` (or `herdr server reload-config`).

## Sidebar badges

The #1 stumbling block on a fresh install: **reporting and rendering are
two different things**. The plugin's hooks automatically report every
space's and agent's code as `$jump` metadata (source `user:jumpd`) — but
herdr only *renders* a metadata token in a sidebar row whose layout
mentions it. Herdr's default workspace/agent row layouts do not include
`$jump`, so a fully working, correctly linked install shows **no
badges** in the sidebar until the row layout says so. (The picker and the
jump popup show the same codes without any sidebar setup — this is about
the sidebar only.)

One-time setup — merge a `$jump` segment into the sidebar row layouts in
herdr's `config.toml`. Keep the segments you already have; if you use
agent-specific `rows_by_agent` overrides, those need the token too:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", { token = "$jump", bold = true, dim = false }, "workspace"],
  ["branch", "git_status"],
]

[ui.sidebar.agents]
rows = [
  ["state_icon", { token = "$jump", bold = true, dim = false }, "machine", "workspace", "tab"],
  ["agent"],
]
```

Keeping `$jump` before the long workspace label leaves the code visible
when the sidebar is narrow. The same thing through Home Manager's
`programs.herdr.settings`:

```nix
let
  jumpToken = { token = "$jump"; bold = true; dim = false; };
in {
  programs.herdr.settings.ui.sidebar = {
    spaces.rows = [
      [ "state_icon" jumpToken "workspace" ]
      [ "branch" "git_status" ]
    ];
    agents.rows = [
      [ "state_icon" jumpToken "machine" "workspace" "tab" ]
      [ "agent" ]
    ];
  };
}
```

Apply it with the **client's** reload: `prefix+shift+r` inside the running
herdr (or restart the client). `herdr server reload-config` reloads the
server only — a success response does **not** mean the attached client's
sidebar layout has refreshed; keybindings and `[ui]` settings are
client-side.

After this one-time setup everything is automatic — the `[[startup]]` +
layout `[[events]]` hooks re-report every code on every layout change
and after each server restart. Nothing else to touch.

No badges showing? Check in this order:

1. `herdr plugin list` — the plugin is present and enabled.
2. `herdr api snapshot` — workspaces/agents carry `tokens.jump` values
   (reporting works; the problem is rendering). Missing? Force a sync
   (`herdr-preview-grid sync-codes` inside the session) or touch the
   layout (create/close a workspace) and re-check.
3. Your merged row layouts actually contain the `$jump` segment, and
   `herdr config check` accepts the file.
4. The **client** actually reloaded (`prefix+shift+r` in-app) — not just
   `herdr server reload-config`.

## Keybindings

The plugin ships **no default keybinds** — herdr's plugin manifest has no key
schema (it only declares actions, panes, events, and the startup hook), so keys
always live in your herdr `config.toml`. "Changing a default" is editing the
`key =` line of the corresponding `[[keys.command]]` entry; nothing to
reinstall.

**The two picker actions** are bound as `plugin_action` commands named
`<plugin-id>.<action-id>` — `preview-grid.spaces` and `preview-grid.agents`
(list them any time with `herdr plugin action list --plugin preview-grid`):

```toml
[[keys.command]]
key = "prefix+w"                     # ← change this line (any herdr chord)
type = "plugin_action"
command = "preview-grid.spaces"      # ← keep the command, change the chord
description = "switch space (preview grid)"
```

**The jump popup** is a plain `popup` binding pointing at the built binary. Its
path depends on how the plugin was installed — find it instead of guessing:

```sh
find ~/.config/herdr/plugins/github-installations -name herdr-preview-grid 2>/dev/null | head -1
# local link/checkout: <checkout>/bin/herdr-preview-grid
```

```toml
[[keys.command]]
key = "prefix+f"
type = "popup"
command = "<that-path> jump"
width = 46
height = 12
description = "jump to code"
```

**One-keystroke variant (`ctrl+shift+e` = "shift+prefix").** herdr accepts
direct chords as bindings, so you can open the same popup with a single
press of prefix+shift:

```toml
# Direct chord — ctrl+e is this setup's [keys] prefix, so the jump chord is
# its SHIFT twin. herdr has no symbolic "shift+prefix" form (key strings
# are literal chords), so this line is a CONVENTION: if you ever change
# keys.prefix, re-derive this chord by hand. Requires a terminal that
# distinguishes ctrl+shift+e from ctrl+e (kitty-keyboard-protocol
# terminals like Ghostty do; where they collide, this binding is inert and
# the prefix keeps working).
[[keys.command]]
key = "ctrl+shift+e"
type = "popup"
command = "<that-path> jump"
width = 46
height = 12
description = "jump to code (direct)"
```

Tab/pane codes in the popup need a herdr binary with `tab.list` and
`pane.focus` (both are in the 0.9.5 schema, though the online method table
omits `pane.focus`); on older servers those sections drop out and the
popup degrades to spaces+agents.

**Tab codes appear on the tabs themselves.** herdr 0.9.5 exposes no
tab-metadata surface (`tab.*` has no `report_metadata`, and the tab bar
renders only the tab's label), so the only way a code can live on a tab is a
label rewrite. The jump popup does exactly that while it is open, and puts
your labels back on the way out:

- **open** (`prefix+f` or the direct `ctrl+shift+e`) → the active space's tabs
  are relabelled `wl 1`, `wf 2`, … — the codes the popup is listing;
- **any fired code** → labels restored, then focus moves;
- **`esc` / ctrl+c** → labels restored;
- **crash/kill** → the ledger survives, and the next open unwinds it before
  tagging again (no nesting, no lost user labels);
- **user renames are preserved** — originals are saved to `tab-tags.json`
  *before* the rename, and that is what comes back.

Nothing to configure: no `tab_bar_right` entry, no extra chord. The jump
chord is the trigger.

<!-- legacy: the polled tab_bar_right command segment was removed in 0.7.0
     (>=1s polling can never catch a sub-second popup) -->

**Apply and validate:** edit → `herdr config check` → reload (`prefix+shift+r`
or `herdr server reload-config`). One footgun: `herdr config reset-keys` backs
up and **deletes all custom keybindings** — a full reset, not a per-plugin one.

Keys never belong in the *plugin's* config (`herdr plugin config-dir
preview-grid`, which holds `charset`, `sort`, `master_width`, …) — that file
configures the plugin's behavior, not how you invoke it.

## Configuration

On first run, copy `config.example.toml` into the plugin config dir
(`herdr plugin config-dir preview-grid` → `config.toml`):

| Key | Default | Meaning |
|---|---|---|
| `charset` | `sdfjklerui` | Ten-letter pool for jump codes: 2-letter pairs first (90), then 3-letter doubled-first codes |
| `master_width` | `0` (auto) | Master list width in columns; `0` = about a third of the terminal, clamped to 28–42 |
| `preview` | `true` | Render the selected item's live pane snapshot in the detail panel |
| `refresh_ms` | `1000` | Snapshot refresh interval while open, floored to 100ms (`≤0` → default; selection moves re-read immediately on top) |
| `sort` | `priority` | `"priority"` (awaiting-input first, then MRU), `"usage"` (MRU), or `"natural"` (herdr order) |
| `theme` | `follow` | `"follow"` (mirror herdr's theme — see below), or pin `"latte"` / `"mocha"` |

Unknown keys are ignored, so tile-era leftovers (`preview_lines`,
`preview_scale`, `tile_min_width`, `code_timeout_ms`) from old configs are
harmless no-ops — nothing in the current code reads them.

## Theming

`theme = "follow"` (the default) makes the picker mirror herdr:

- **Palette** — the picker reads herdr's `~/.config/herdr/config.toml`
  `[theme]` tables: `name`, `auto_switch`, `light_name`/`dark_name`
  (Catppuccin Latte/Mocha are embedded; both halves of `auto_switch` pairs
  work), plus `[theme.custom]` (and its `.light`/`.dark` variants) tokens —
  `panel_bg`, `subtext0`/`overlay1`, `accent`, `red`, `green`, `blue`,
  `peach` — mapped onto the picker's colors.
- **Light/dark** — herdr keys `auto_switch` off the host terminal's
  appearance and sets the pane's color scheme accordingly; the picker asks
  the terminal (bubbletea's `BackgroundColorMsg`), so it flips in lockstep
  with herdr, mid-session included.
- **Accents stay yours** — per-item identity hues are hash-stable, and the
  light/dark palettes keep the same hue *family* per index: a space keeps
  its color across opens and appearance flips.
- Force one palette with `theme = "latte"` or `theme = "mocha"`.

Two honest limitations (upstream herdr exposes no theme env var and no theme
RPC on its socket — everything above is read at picker launch): non-catppuccin
named themes fall back to the appearance-matched Catppuccin pair, and unknown
or unparseable values are ignored rather than guessed — the picker never
renders unstyled.

## Keys

| Key | Action |
|---|---|
| `ctrl+j/k`, `↓`/`↑` | move selection |
| `pgup`/`pgdn`, `ctrl+u`/`ctrl+d` | page |
| `home` / `end` | first / last row |
| `/` | fuzzy search |
| `j k l f d s` (+ pairs) | select by jump code |
| `backspace` | edit code buffer (or search text) |
| `enter` | switch to the selected item |
| `esc` | clear search/filter, else close |
| `ctrl+c` | close |

Sidebar jump popup (bind e.g. `prefix+f` or a direct chord — see
[Keybindings](#keybindings) — to `…/bin/herdr-preview-grid jump` as a
`popup`): the popup lists every jumpable target — spaces, agents, tabs,
plain panes — with its code. A complete code fires instantly; `backspace`
edits; `esc` cancels. With a taller popup (~12 rows):

| Key | Action |
|---|---|
| `j k l f d s` (+ pairs, then triples) | jump code — fires the moment it completes |
| `/` | fuzzy-search labels (kind words like `tab`/`pane` work too) |
| `↑`/`↓`, `pgup`/`pgdn` | walk the search matches (scroll in code mode) |
| `enter` | commit the selected search match |
| `backspace` | edit the code buffer (or the search text) |
| `esc` | clear the search, else close |
| `ctrl+c` | close |

Spaces and agents use exactly their sidebar badge codes; tabs and panes
continue the same table. The badge sync + a full rebuild run at every
popup open, so codes always match the live layout.

## How it works

Standard herdr plugin anatomy:

- `herdr-plugin.toml` declares two **actions** (`spaces`, `agents`) — actions
  run without a TTY, so they delegate to the matching **pane** entrypoints via
  `herdr plugin pane open` — and two **panes** (overlay placement) that run the
  Go binary.
- The binary talks to the running herdr server over `$HERDR_SOCKET_PATH`
  (newline-delimited JSON, one request per connection) for state
  (`workspace.list`, `pane.list`, `agent.list`), previews
  (`pane.read` with `format = "ansi"`), and focus (`workspace.focus`,
  `agent.focus`).
- **The pickers assign canonical codes** (spaces start the shared
  sequence, agents continue after the workspace codes — exactly
  `jump.Entries`, which is also the badge table). Codes are assigned in
  the server's natural order before priority/MRU sorting, travel with
  their items through every sort/filter, and typed codes resolve against
  that table — a code means one target on every surface: sidebar, popup,
  and picker.
- MRU state is persisted to `$HERDR_PLUGIN_STATE_DIR/mru.json`; the sidebar
  badge table lives next to it in `jumpcodes.json`.
- `sync-codes` (the `[[startup]]` + layout `[[events]]` hook mode) re-lists
  the live **spaces+agents** layout, diffs against `jumpcodes.json`, and
  re-reports the plugin's `$jump` metadata tokens (source `user:jumpd`) —
  badges follow every create/close/move/rename/reorder automatically, with
  no daemon. Tabs and panes are deliberately NOT in this pass: they have no
  sidebar row to badge and no report to keep fresh.
- `jump` (the popup mode) rebuilds the full table live at every open —
  `jump.Collect` lists workspaces, agents, tabs (`tab.list`), and panes,
  re-asserts the badges along the way, then codes everything in herdr's
  order: spaces, agents (exactly their badge codes — the code sequence is
  prefix-stable, so the table only grows after the badge rows), then tabs,
  then plain panes. Codes resolve to `workspace.focus` / `agent.focus` /
  `tab.focus` / `pane.focus`, with `tab.focus` on the pane's tab as the
  fallback when an older server rejects `pane.focus`.

## Development

```sh
go build ./...
go vet ./...
go test ./...
CGO_ENABLED=0 go build -o bin/herdr-preview-grid ./cmd/herdr-preview-grid
```

Manual smoke test inside herdr: `herdr plugin action invoke preview-grid.spaces`.
Sidebar: add a throwaway space (`herdr workspace create --label tmp --cwd /tmp`)
and watch the badges re-space via the hooks, then close it; `herdr plugin log
list` shows the `sync-codes` runs.

UI work without a live session: `go run ./tools/demo-server /tmp/herdr-demo.sock`
serves a curated layout; point `HERDR_SOCKET_PATH` at the socket and run the
binary (or open the plugin linked from this checkout) against it.

Screenshots/GIF: `tools/ansi2html.py` converts a `tmux capture-pane -e`
dump into styled HTML with exact character-grid geometry (crop, never
distort); screenshot it headless (`chromium --headless
--force-device-scale-factor=3`) and assemble with ImageMagick (`-delay`
per frame, `-dither None` for flat terminal colors, `-layers Optimize`).
Drive a throwaway herdr server under isolated `XDG_CONFIG_HOME`/
`XDG_STATE_HOME` inside tmux (`herdr plugin link` the checkout, seed demo
workspaces so the hooks populate badges, `tmux send-keys` the tour,
capture each beat) — remember servers daemonize, so restart with
`herdr server stop`, and the popup jump mode renders only on attached
clients.

## License

MIT — see [LICENSE](LICENSE).
