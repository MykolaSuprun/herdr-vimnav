# Repository Guidelines

## Project Overview

`herdr-preview-grid` is a [herdr](https://herdr.dev) plugin: a full-screen
**master-detail switcher** for herdr workspaces ("spaces") and agents — a left
list (code badge, status dot, label) with the selected item's live ANSI pane
preview in the right panel. Navigation is `ctrl+j/k` (plus arrows), fuzzy
search is `/`, and typing a row's code selects it (select-only — `enter`
commits the switch). It also maintains **sidebar jump codes**: `$jump` badges
on the native sidebar's spaces/agents rows (auto-synced by manifest hooks on
every layout change) and a `jump` popup (`prefix+f`) that focuses whatever
code you type. Implemented in Go with Bubble Tea v2 / Lip Gloss v2. MIT
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
  → socket: workspace.list / agent.list → diff vs jumpcodes.json
  → workspace.report_metadata / pane.report_metadata (source user:jumpd, token jump=<code>)
  → sidebar rows render $jump badges

prefix+f ([[keys.command]] popup)
  → popup runs ./bin/herdr-preview-grid jump   (TTY)
  → type code → workspace.focus | agent.focus → exit
```

- `cmd/herdr-preview-grid/{main,jump}.go` — one binary, four modes: picker
  (`spaces`/`agents`), `record` (MRU event hook), `sync-codes` (badge sync),
  `jump` (popup). `main.go` resolves the mode from argv or
  `HERDR_PLUGIN_ENTRYPOINT_ID`, wires config/state/socket; `jump.go` is the
  popup model (type code → focus, esc cancels).
- `internal/herdr/` — socket client (one request per connection) + typed API.
- `internal/app/` — the Bubble Tea model: data load, filter, codes
  (select-only), MRU, navigation; `view.go` holds the master-detail
  layout/rendering (pure helpers + lipgloss). Preview resolution skips the
  picker's own pane (`HERDR_PANE_ID`) and falls back to the launch-context
  pane (`HERDR_PLUGIN_CONTEXT_JSON.focused_pane_id`) — the overlay itself is
  the host space's focused pane, so without this the host space would preview
  the picker itself.
- `internal/codes/` — prefix-free jump-code table + resolver (six singles,
  then pairs; no code is a prefix of another, so codes fire on completion —
  invariant-tested).
- `internal/mru/` — usage ledger persisted to `$HERDR_PLUGIN_STATE_DIR/mru.json`.
- `internal/jump/` — sidebar badge engine: canonical space/agent → code
  assignment, `jumpcodes.json` state, diff-based `Sync` (reports only changed
  tokens), prefix `Match` for the popup.
- `internal/config/` — flat `config.toml` parser (top-level keys only).
- `tools/demo-server/` — fake herdr socket server serving a curated layout
  (singles + pair codes, 4-digit number, duplicate labels, attention items,
  ANSI previews) for screenshots/demos and UI work without a live session.
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
list`) — then close it and confirm they re-space back; `prefix+f` + a code
focuses its target, Esc cancels.

## Conventions

- 2-space TOML/JSON-ish; gofmt everywhere (tabs).
- Pure helpers separated from model logic; no globals except styles.
- Keep both columns exact-height: the list pads to `bodyH` rows and the detail
  panel renders `bodyH` lines (2 border + 1 title + `bodyH-3` preview), with
  border/padding arithmetic centralized in `layout()` (view.go).
