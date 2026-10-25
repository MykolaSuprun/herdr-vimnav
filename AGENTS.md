# Repository Guidelines

## Project Overview

`herdr-preview-grid` is a [herdr](https://herdr.dev) plugin: a full-screen
**master-detail switcher** for herdr workspaces ("spaces") and agents — a left
list (code badge, status dot, label) with the selected item's live ANSI pane
preview in the right panel. Navigation is `ctrl+j/k` (plus arrows), fuzzy
search is `/`, and typing a row's code selects it (select-only — `enter`
commits the switch). It also maintains **jump codes**: `$jump` badges
on the native sidebar's spaces/agents rows (auto-synced by manifest hooks on
every layout change) and a `jump` popup (bound to `prefix+f` or a direct
chord) that focuses whatever code you type — with codes covering not just
spaces and agents but every tab and plain pane as well. Implemented in Go
with Bubble Tea v2 / Lip Gloss v2. MIT
licensed.

## Architecture & Data Flow

```
herdr keybinding ([[keys.command]] plugin_action)
  → action "preview-grid.spaces"          (no TTY — delegates)
  → herdr plugin pane open --entrypoint spaces
  → pane (overlay) runs ./bin/herdr-preview-grid spaces   (TTY)
  → socket: workspace.list / tab.list / pane.list / agent.list
  → master list + pane.read (format=ansi) preview of the selected item
  → user picks → workspace.focus | agent.focus → exit

layout change or server start
  → [[startup]] / [[events]] hook runs ./bin/herdr-preview-grid sync-codes   (no TTY)
  → socket: workspace.list / agent.list → canonical codes (Entries, no state)
  → workspace.report_metadata / pane.report_metadata (source user:jumpd, token jump=<code>)
  → sidebar rows render $jump badges
  NOTE: reports are UNCONDITIONAL on every pass (no diff vs jumpcodes.json —
  herdr does not persist plugin metadata across server restarts while the
  state file survives them; a diff masked exactly the stable workspace ids
  and spaces lost their badges on every restart until 0.4.1; pinned by
  TestSyncReReportsEverythingAfterServerRestart). jumpcodes.json is only an
  audit mirror (plus code-stability debugging aid); Entries() derives codes
  deterministically from the layout.

prefix+f or a direct chord ([[keys.command]] popup)
  → popup runs ./bin/herdr-preview-grid jump   (TTY)
  → jump.Collect: workspace.list / agent.list / tab.list / pane.list
    (spaces/agents errors fatal; tab/pane errors degrade the table)
  → same pass re-asserts badges (unconditional reports, self-heal)
  → type code (or '/' fuzzy-search + enter) → workspace.focus |
    agent.focus | tab.focus | pane.focus (fallback: tab.focus of the
    pane's tab) → confirmation beat → exit
```

- `cmd/herdr-preview-grid/{main,jump,tabtags}.go` — one binary, four modes:
  picker (`spaces`/`agents`), `record` (MRU event hook), `sync-codes` (badge
  sync), `jump` (popup + tab tagging). `main.go` resolves the mode from argv
  or `HERDR_PLUGIN_ENTRYPOINT_ID`, wires config/state/socket; `jump.go` is
  the popup model: live listing of all coded targets, complete codes fire
  instantly, '/' fuzzy-searches labels, pane.focus with tab fallback;
  `tabtags.go` retags the active space's tabs with their codes while the
  popup is open and restores the originals on fire/esc/crash (herdr has no
  tab-metadata surface, so a label rewrite is the only surface that works —
  see Key facts).
- `internal/herdr/` — socket client (one request per connection) + typed API.
- `internal/app/` — the Bubble Tea model: data load, filter, codes
  (select-only), MRU, navigation; `view.go` holds the master-detail
  layout/rendering (pure helpers + lipgloss). Preview resolution skips the
  picker's own pane (`HERDR_PANE_ID`) and falls back to the launch-context
  pane (`HERDR_PLUGIN_CONTEXT_JSON.focused_pane_id`) — the overlay itself is
  the host space's focused pane, so without this the host space would preview
  the picker itself.
- `internal/codes/` — prefix-free jump-code table + resolver. Ten-letter
  pool (`sdfjklerui`): every code is an ordered pair of DISTINCT letters
  (10×9 = 90), then doubled-first triples (`ssd`) past 90 — so no code is a
  prefix of another and codes fire on completion. No single-letter codes
  exist. Invariant-tested across the tier boundary.
- `internal/mru/` — usage ledger persisted to `$HERDR_PLUGIN_STATE_DIR/mru.json`.
- `internal/jump/` — jump-code engine. `Entries()` = the badge table
  (spaces, then agents); `EntriesFor()` extends it with tabs and plain
  non-agent panes continuing the SAME prefix-stable sequence (badge codes
  provably unchanged — pinned by test, so popup codes always agree with
  sidebar badges); `Sync` = hook-side badge pass — UNCONDITIONAL reports
  (herdr wipes plugin metadata across restarts while `jumpcodes.json`
  survives them; a diff masked exactly the stable space ids — pinned by
  TestSyncReReportsEverythingAfterServerRestart); `Collect` = popup-side
  full-table rebuild + badge self-heal in one pass (tab/pane list errors
  degrade those sections instead of failing the open); prefix `Match` for
  the popup; entries carry workspace/tab context for compound labels and
  the pane→tab focus fallback.
- `internal/config/` — flat `config.toml` parser (top-level keys only).
- `tools/demo-server/` — fake herdr socket server serving a curated layout
  (pair codes + the triple tier, a 4-digit number, duplicate labels, two multi-tab
  spaces so tabs/panes get their own codes, attention items, ANSI previews)
  for screenshots/demos and UI work without a live session.
- `docs/screenshots/` — committed PNGs rendered from the demo server
  (tmux `capture-pane -e` → ANSI→HTML → headless Chromium).

## Key Facts

- **Actions have no TTY.** They must delegate to panes (`herdr plugin pane
  open`), exactly like simple-switcher does; running the TUI in an action
  fails with `could not open TTY`.
- **Pane placement**: `overlay` (default). Switch to `popup` in the manifest if
  a modal popup is preferred; both receive keys including Esc.
- **Pane entrypoints env**: panes receive `HERDR_PLUGIN_ENTRYPOINT_ID`; argv is
  preferred (the manifest passes `spaces`/`agents` explicitly).
- **Sidebar badges** are `$jump` metadata tokens reported under source
  `user:jumpd`; re-reporting a source REPLACES its tokens. A server restart
  wipes reported tokens — the `[[startup]]` hook repopulates them.
- **Tabs cannot carry badges** (2026-10-22, verified against the bundled
  0.9.5 schema): `tab.*` is create/list/get/focus/rename/move/close only —
  there is no `tab.report_metadata`, and the tab bar renders tab labels
  only (no `$token` slots). Tab codes therefore surface via the native
  `[ui] tab_bar_right = [{ type = "command", command = "<bin> tab-codes" }]`
  segment (active workspace only, local 1..n numbering — `tab.number` is a
  session-global counter that would not match the bar). `tab-codes`
  computes `EntriesFor(spaces, agents, tabs, nil, charset)` — panes sort
  AFTER tabs, so omitting them cannot shift a tab code; failures print
  nothing so herdr clears the segment.
- **`pane.focus` exists but is undocumented**: `PaneTarget{pane_id}` in the
  0.9.5 bundled schema, absent from the online method table (the table is
  known non-exhaustive — `pane.input.set` is likewise omitted). Pane codes
  fire it; the popup degrades to `tab.focus` on the pane's tab. Verified
  end-to-end in an isolated session (focus moved pane-to-pane).
- **Tab codes are label rewrites** (2026-10-24, supersedes the tab_bar_right
  experiment): herdr 0.9.5 has no tab-metadata surface, and a polled
  `[ui] tab_bar_right` command (floor ~1 s) can never catch a popup that
  lives a few hundred ms — it was removed in 0.7.0. The popup now renames
  the **active space's** tab labels to `"<code> <label>"` on open and
  restores them on fire/esc/crash. `tab-tags.json` is written BEFORE each
  rename (durability: a kill between the two steps is still unwindable);
  the next open clears stale tags first and re-collects entries so a
  previous open's tag is never compounded (`wl 1` must not become
  `wl wl 1`). User renames survive because the saved original is what is
  restored.
- **`prefix+ctrl+e` is NOT bindable** (live `herdr config check`): herdr
  reserves prefix+<the prefix key> as a literal-prefix passthrough and
  disables such bindings — "pressing the prefix twice sends a literal
  prefix key". "shift+prefix" is the DIRECT chord `ctrl+shift+e` (Ghostty's
  default `new_split:down` must be unbound in the ghostty module for it to
  reach herdr).
- **`herdr server reload-config` does NOT reload client-side settings**
  (keybindings, `[ui]`, tab bar). Those need the in-app reload
  (`prefix+shift+r`) or a client restart. Verified: a running client log
  showed zero `tab_bar_right` mentions after a CLI reload.
- **A stale binary on PATH silently shadows the dev checkout**: the nix
  store had 0.4.1 on PATH, so `herdr-preview-grid jump || ./bin/… jump`
  ran the OLD popup and the `||` fallback never fired — popups appeared to
  work while showing 0.4.1 behavior. Bindings here point at the checkout
  path directly; only by-name after a rebuild that ships the same version.
- **Nested herdr is disabled by default** (`experimental.allow_nested`), so
  a TUI client cannot be attached to a test session from inside a herdr
  pane. E2E verification therefore drives the binary against the test
  socket directly + replicates herdr's command-entry env
  (`HERDR_ACTIVE_WORKSPACE_ID`), and never trusts `HERDR_SESSION` for CLI
  routing: `HERDR_SOCKET_PATH` wins over it and silently retargets calls
  at the LIVE session (this caused two throwaway spaces in the default
  session during 0.6.1 work — always `env -u HERDR_SOCKET_PATH` or set it
  explicitly to the test socket).
- **Hook events are layout-only** (workspace/pane created/closed/moved/
  renamed/reordered/exited, agent_detected). No `*.metadata_updated` events:
  a hook that reports tokens would re-trigger itself. Hooks run with cwd =
  plugin root and receive `HERDR_PLUGIN_{CONFIG,STATE}_DIR`.
- **Popup commands** run through a shell; `width`/`height` must be INTEGERS
  for cells (strings are percentages only). Popups only get
  `HERDR_SOCKET_PATH` (no plugin dirs) — CLI runs fall back to XDG paths
  (`config.DefaultDir()`, `jump.DefaultStateDir()`).
- **Socket protocol**: newline-delimited JSON `{"id","method","params"}` one
  request per Unix-socket connection; `pane.read` sources are
  `visible|recent|recent_unwrapped|detection`, formats `text|ansi`.
- **Go min 1.26** (bubbletea v2.0.10 requirement); static `CGO_ENABLED=0`
  builds. `charm.land/{bubbletea,lipgloss,bubbles}/v2` are the canonical v2
  import paths.
- **lipgloss `Width/Height` are minimum outer sizes** and never trim; keep line
  counts exact (the list pads to `bodyH` rows, the detail panel renders exactly
  `bodyH` lines: border 2 + title 1 + preview `bodyH-3`) and truncate with
  display-width awareness (`truncateANSI`).
- **Master-row columns**: marker(2) chips(4 — badge exactly code-width:
  singles 1 cell centered, pairs "uj" adjacent, triples "uuu"; column
  wash-padded) number(3-4, right, dim) label(flex)
  dim cwd, then a pinned rail (status dot + focused flag). Numbers: spaces use
  `ws.Number`; agents best-effort member-workspace number (blank on a failed
  fetch). Selected-row wash = `styles.washColor` (palette base blended 12%
  toward the item's accent); badge background = `styles.chipBg` (same blend
  at 55% toward base — badges keep their hue but read soft; full-intensity
  accent is glare). Header counts `AwaitingInput` items ("N attention").
- **Theming follows herdr** (`internal/theme`, resolved in `app.New` +
  re-resolved on `tea.BackgroundColorMsg`): palette precedence = plugin
  `theme` key ("follow" default | "latte" | "mocha") → herdr config.toml
  `[theme]` (`name`/`auto_switch` `dark_name`/`light_name`; Unknown
  appearance keeps the startup `name`, mirroring herdr) → `[theme.custom]`
  (+ `.light`/`.dark` when `auto_switch`, mapped tokens: `panel_bg`
  `subtext0`/`overlay1` `accent` `red` `green` `blue` `peach`). Only
  Catppuccin Latte/Mocha are embedded (hue-aligned accent lists — index N
  = same hue family in both, so item identities survive flips); other named
  themes fall back to the appearance-matched pair. herdr's config is parsed
  by a bounded TOML-subset reader (NO new Go deps — vendor hash must stay
  untouched); invalid values are ignored, never brick the UI. Every render
  hex flows from `Model.s()` (zero-Model falls back to latte) — view tests
  pin the latte values via a package-wide `loadHerdrSpec` stub.

## Nix packaging

The repo is also a flake: `packages.<system>.default` (buildGoModule, static,
version read from `herdr-plugin.toml`), `checks` (`package` = build + `go test`,
`module-eval` = a real `homeManagerConfiguration` eval), `devShells.default`,
and `homeManagerModules.default` (`nix/home-manager-module.nix`).

Two facts make this work and must not regress:

- **Layout serves both install paths.** Store root = `herdr-plugin.toml` +
  `bin/herdr-preview-grid` (so `herdr plugin link <store path>` works; link
  never runs `[[build]]`), plus `share/herdr/plugins/<id> -> ../../..` (THREE
  ups — four would overshoot to /nix/store; live-caught in the container
  E2E, pinned by checks.layout) for
  generic Nix modules. `postInstall` must NOT re-install the binary —
  buildGoModule already put it in `$out/bin` (`test -x` guards it).
- **Nix is the only Go-free install path.** `herdr plugin install` needs git +
  a Go toolchain on PATH (it runs the manifest `[[build]]`); the store-path
  link needs neither. Same for `env.CGO_ENABLED = "0"` (buildGoModule reads
  `args.env`, nixpkgs' Go defaults it to 1) — the Nix binary must stay static
  like the manifest build.

Module quirks worth remembering: activation dag entries carry `data` (not
`text`); `lib.sortAttrs` is absent from HM's lib (use `lib.sort` +
`builtins.attrNames`); nested `''` inside an antiquotation is a parser error;
`addToPath` exists because store paths change per rebuild, so bindings should
use `herdr-preview-grid jump` by name.

## Development Commands

```sh
go build ./...            # compile
go vet ./...              # static checks
go test ./...             # unit tests (app, codes, config, herdr, jump, mru, cmd)
CGO_ENABLED=0 go build -o bin/herdr-preview-grid ./cmd/herdr-preview-grid
herdr plugin link "$PWD"  # register with the running herdr (no build step in link)
herdr plugin action invoke preview-grid.spaces   # live smoke test
HERDR_SOCKET_PATH="$HOME/.config/herdr/herdr.sock" ./bin/herdr-preview-grid sync-codes
herdr plugin log list     # hook runs + exit codes (sync-codes evidence)
nix flake check           # Nix package (build + go test) AND HM module eval
nix develop               # go + gopls shell (optional)
# popup smoke (reliable harness: a detached tmux pane. Do NOT use script(1) —
# its pty mode leaves this Bubble Tea v2 TUI waiting on capability queries and
# no frame renders. Use the wip session's socket so smoke focus-jumps stay out
# of the default session):
tmux new-session -d -s pg-smoke -x 100 -y 6 \
  "env HERDR_SOCKET_PATH=$HOME/.config/herdr/sessions/wip/herdr.sock $PWD/bin/herdr-preview-grid jump"
tmux capture-pane -p -t pg-smoke   # frame: "jump ▸ " + hint line
tmux send-keys -t pg-smoke j       # a complete code → focus moves + settles 180ms
tmux capture-pane -p -t pg-smoke   # "jump ▸ j → <target label>"
tmux kill-session -t pg-smoke
```

Manual QA inside herdr: open each mode, walk the list (`ctrl+j/k`), fuzzy
search (`/n1`), select via a code (preview must follow; Enter commits), verify
the target focus + `mru.json` update, Esc clears/closes. Sidebar: create a throwaway space (`herdr workspace create
--label tmp --cwd /tmp`) — badges re-space via the hooks (`herdr plugin log
list`) — then close it and confirm they re-space back. Jump popup: `prefix+f`
(or a direct chord) opens the listing — a complete code fires instantly
(verify space/agent codes match their sidebar badges, and that a TAB or plain
PANE code moves the real focus: tall popups show the table; `/tab` lists the
tab rows), Esc cancels.

## Conventions

- 2-space TOML/JSON-ish; gofmt everywhere (tabs).
- Pure helpers separated from model logic; no globals except styles.
- Keep both columns exact-height: the list pads to `bodyH` rows and the detail
  panel renders `bodyH` lines (2 border + 1 title + `bodyH-3` preview), with
  border/padding arithmetic centralized in `layout()` (view.go).
