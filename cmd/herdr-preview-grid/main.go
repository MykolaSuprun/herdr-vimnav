// Command herdr-preview-grid is a herdr plugin: a preview-grid picker for
// workspaces (spaces) and agents with hjkl navigation, fuzzy search, and
// short jump codes.
//
// Modes:
//   - "spaces" / "agents" (default "spaces"): the interactive grid TUI.
//   - "record": event-hook mode — ingest HERDR_PLUGIN_EVENT_JSON from herdr's
//     [[events]] hooks and record focus activity into the MRU ledger, then
//     exit. This keeps the ledger fed by ALL navigation (any client, any
//     method: keybindings, sidebar, clicks), not just grid jumps — which is
//     what makes the priority sort's "recently active" real.
//   - "sync-codes": one-shot sync of the sidebar $jump badges with the live
//     layout (wired to the [[startup]] hook and layout-change events).
//   - "jump": tiny popup — type a sidebar code, focus moves, Esc cancels.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/app"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/mru"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-preview-grid:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "record":
			return recordMode()
		case "sync-codes":
			return syncCodesMode()
		case "jump":
			return jumpMode()
		}
	}

	mode := app.ModeSpaces
	switch arg := entrypoint(); arg {
	case "agents":
		mode = app.ModeAgents
	case "spaces", "":
	default:
		return fmt.Errorf("unknown entrypoint %q", arg)
	}

	socket := os.Getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return herdr.ErrNotInHerdr
	}

	cfg := config.Load(configDir())
	stDir := stateDir()
	client := herdr.NewClient(socket)
	ledger := mru.Load(stDir)

	selfPane := os.Getenv("HERDR_PANE_ID")
	launchPane := app.LaunchFocusedPane(os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"))
	model := app.New(mode, cfg, client, ledger, stDir, selfPane, launchPane)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		return err
	}
	return nil
}

// recordMode ingests one herdr event (from HERDR_PLUGIN_EVENT_JSON) and
// touches the MRU ledger with the matching item key.
func recordMode() error {
	raw := os.Getenv("HERDR_PLUGIN_EVENT_JSON")
	if raw == "" {
		return nil // hook fired without payload; nothing to record
	}
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if stateDir == "" {
		return nil
	}
	key, ok := app.ParseEventKey(raw)
	if !ok {
		return nil // unrelated event; not an error
	}
	mru.Load(stateDir).Touch(key)
	return nil
}

// entrypoint resolves which grid to open: an argv override, else the
// plugin entrypoint env herdr injects for panes.
func entrypoint() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	if v := os.Getenv("HERDR_PLUGIN_ENTRYPOINT_ID"); v != "" {
		return v
	}
	return ""
}

// syncCodesMode re-asserts the sidebar $jump badges against the live layout.
// Idempotent and diff-based, so it is safe to run on every layout event.
func syncCodesMode() error {
	socket := os.Getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return herdr.ErrNotInHerdr
	}
	client := herdr.NewClient(socket)
	cfg := config.Load(configDir())
	_, err := jump.Sync(client, jump.LoadState(stateDir()), cfg.Charset)
	return err
}

// configDir resolves the plugin config dir: HERDR_PLUGIN_CONFIG_DIR inside
// herdr (hooks/panes), an XDG fallback for CLI runs.
func configDir() string {
	if d := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); d != "" {
		return d
	}
	return config.DefaultDir()
}

// stateDir resolves the plugin state dir: HERDR_PLUGIN_STATE_DIR inside
// herdr (hooks/panes), an XDG fallback for CLI runs.
func stateDir() string {
	if d := os.Getenv("HERDR_PLUGIN_STATE_DIR"); d != "" {
		return d
	}
	return jump.DefaultStateDir()
}
