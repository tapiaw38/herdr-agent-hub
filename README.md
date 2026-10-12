# Herdr Agent Hub

A native terminal dashboard for agents already running in Herdr, plus reusable local agent templates. Built with Go, Bubble Tea v2, Lip Gloss v2, and Bubbles v2. Current plugin version: 3.12.0.

> Agent Hub manages Herdr sessions; it does not create AI models or fabricate chat history. Herdr currently provides terminal-backed agent inspection and focus, not a structured conversation store.

## Features

- Detect live agent state from Herdr's documented `session.snapshot` CLI bootstrap.
- Focus selected live agent through `herdr agent focus` and relay interactive approval keys through `herdr agent send-keys`.
- Observe an agent's live ANSI terminal screen inside the chat inbox and conversation view, with a safe virtual terminal renderer.
- Type into the embedded terminal and answer approval prompts with ordinary keys through Herdr's terminal controller.
- Store reusable templates and launched-session identity atomically in Herdr's plugin config directory.
- Periodic refresh, dashboard/detail/help views, keyboard navigation, search, favorites, and create/delete profile flow.
- Provider-specific and selectable 8×8 pixel avatars rendered with Unicode half-blocks, with custom icon and color support.
- Scrollable agent list, persistent favorites for detected agents, terminal output viewport, and confirmed starts for Herdr-supported agent kinds.
- Plugin action to focus the last opened Agent Hub pane after focusing another agent.
- A manifest action that opens or returns focus to the dashboard, so Herdr keybindings can launch it.
- Label its own Herdr pane `Agents` when the dashboard starts.

## Install

Requires Go 1.24+ and Herdr 0.9.1+. GitHub Marketplace installation builds the binary using Go:

```sh
herdr plugin install tapiaw38/herdr-agent-hub
herdr plugin action invoke community.herdr-agent-hub.open-dashboard
```

After changing a local checkout, rebuild with `go build -o agent-hub ./cmd/agent-hub`, run `herdr plugin link /path/to/herdr-agent-hub`, then close the old Agent Hub pane and open a new one. Existing panes keep their already running process.

For local development:

```sh
go build -o agent-hub ./cmd/agent-hub
herdr plugin link .
herdr plugin action invoke community.herdr-agent-hub.open-dashboard
```

Invoke the action from the workspace where you want Agent Hub. The action opens a dashboard pane in that workspace and reuses that workspace's existing dashboard pane later. The dashboard process adopts that workspace directory after startup, so new Herdr shell panes opened beside it follow the project rather than the plugin checkout. A direct `herdr plugin pane open` uses the plugin root as its initial directory; the action is the recommended entrypoint.

`HERDR_BIN_PATH` is preferred; outside Herdr, the executable falls back to `herdr` on `PATH`. Config uses `HERDR_PLUGIN_CONFIG_DIR/agents.json`; `HERDR_PLUGIN_STATE_DIR` stores only runtime pane identity. No secrets are stored.

Create and edit reusable templates with **N** and **E**. Templates hold identity, instructions, provider, and avatar; they do not reserve a workspace or pane. Choose pixel avatar with arrows or `j`/`k`, or press 1–8. **Ctrl+K** opens provider picker. **Ctrl+P** can associate an existing agent for reference, but does not bind template to its workspace. **Tab** advances, **Shift+Tab** goes back, and **Ctrl+S** saves. Start a template with **A**; choose workspace and shell pane during launch. After Herdr confirms startup, Agent Hub saves a session-to-template association so each launched session keeps its chosen avatar, even when template is reused.

Dashboard separates **AGENT SESSIONS** (live Herdr panes) from **AGENT TEMPLATES** (saved profiles not running). Start with **A**. Choose template, then workspace and shell pane. Review and confirm with **Y** or **Enter**. Identity instructions are sent as a normal first `herdr agent prompt` after startup; they are not provider system prompts and may not persist after restart. Sessions retain template avatar and display identity by Herdr pane and agent name. A timeout or `agent_not_ready` may mean provider started but Herdr could not confirm readiness; refresh before retrying. After focusing an agent, return to Agent Hub with `herdr plugin action invoke community.herdr-agent-hub.focus-dashboard` from a Herdr terminal.

Press **C** on dashboard for chat inbox. Agent list stays on left; selected agent's live Herdr terminal screen appears on right. Use arrows or `j`/`k` to switch. Press **Enter** to open that terminal. Type, paste, answer approvals, and press **Enter** directly in the embedded terminal. In normal terminal mode, mouse wheel and **PgUp/PgDn** scroll captured terminal history; **End** returns to live output. In alternate-screen apps, wheel sends **PgUp/PgDn** to the app. **Esc** goes to the agent; **Ctrl+G** returns to the chat list. From inbox, **Esc** or **B** returns to dashboard. Agent Hub uses Herdr's terminal controller and does not fabricate chat messages. Offline profiles stay on dashboard and do not appear as live chats. **D** on dashboard or inbox deletes local profile after **Y** confirmation, without closing its Herdr pane.

### Keyboard shortcut to open Agent Hub

Herdr keybindings can invoke plugin actions, but cannot open plugin panes directly. Agent Hub exposes an `open-dashboard` action for that purpose. Herdr normally assigns `prefix+h` to pane-left navigation, so move that action to `prefix+left` when assigning `prefix+h` to Agent Hub. Add this to Herdr's `config.toml` (usually `~/.config/herdr/config.toml`):

```toml
[keys]
focus_pane_left = "prefix+left"

[[keys.command]]
key = "prefix+h"
type = "plugin_action"
command = "community.herdr-agent-hub.open-dashboard"
description = "Open Agent Hub"
```

Reload keybindings with `herdr server reload-config` from a Herdr-managed pane. Herdr's default prefix is `Ctrl+B`, so press **Ctrl+B**, release, then **H**. If you changed the prefix, press your configured prefix first. `prefix+?` shows active keybindings. `prefix+left` still navigates to the pane on the left.

Config example (`agents.json`):

```json
{
  "version": 4,
  "favorites": ["0123456789abcdef|w3:p8"],
  "profiles": [
    {
      "id": "sales-helper",
      "name": "Sales Outbound",
      "description": "Outbound research",
      "instructions": "You are a sales research assistant. Draft outreach for review; never send messages.",
      "icon": "✦",
      "color": "#8B5CF6",
      "provider": "claude",
      "executable": "claude",
      "args": ["--continue"],
      "workdir": "/workspace/sales",
      "workspace_id": "w3",
      "pane_id": "w3:p8",
      "favorite": true
    }
  ]
}
```

## Keys

| Key | Action |
|---|---|
| ↑/↓, j/k | Select agent |
| C | Open chat inbox with agent list and live terminal |
| Enter in inbox | Open selected agent terminal |
| Type in conversation | Send keys directly to agent terminal |
| Mouse wheel in terminal | Scroll captured output; send PgUp/PgDn to alternate-screen apps |
| PgUp / PgDn / End in conversation | Scroll terminal history / return to live output |
| Esc in conversation | Send Esc to agent terminal |
| Ctrl+G in conversation | Return to chat list or dashboard |
| B / Esc in inbox | Return to dashboard |
| PgUp / PgDn on dashboard | Scroll agent list by page |
| Home / End | Select first / last agent |
| A | Start a supported Herdr agent in a ready shell pane |
| Ctrl+P, then N | Create a fresh shell tab for the selected agent |
| Ctrl+W in start or profile editor | Select Herdr workspace and its directory |
| N / E | New / edit local profile |
| Ctrl+K in profile editor | Choose Herdr provider |
| Ctrl+P in profile editor | Link live agent pane or ready shell |
| Ctrl+A in profile editor | Show/hide advanced metadata |
| Ctrl+S in profile editor | Save profile from any field |
| D, then Y | Delete selected local profile; keep live Herdr pane |
| F | Persist favorite for any detected agent |
| ↑/↓, j/k or 1–8 in Avatar field | Choose predefined pixel avatar |
| / | Search |
| R | Refresh |
| Esc | Back |
| ? | Help |
| Q | Quit |

## Development

```sh
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/agent-hub
```

Go cross-compilation example: `GOOS=linux GOARCH=amd64 go build ./cmd/agent-hub`; for macOS use `GOOS=darwin GOARCH=arm64`. The manifest currently advertises Linux and macOS only. Windows is not declared because this release has not verified console behavior.

## Architecture

`internal/herdr` isolates the CLI integration behind `HerdrClient`-style `Client`; `internal/agents` maps semantic states and joins local profiles to stable live IDs; `internal/storage` owns schema validation and atomic JSON writes; `internal/ui` renders and routes Bubble Tea messages. CLI calls use argv directly with timeouts, never a shell.

## Herdr API research

See [docs/herdr-api.md](docs/herdr-api.md) for confirmed interfaces, limitations, and design choices. Herdr owns plugin pane lifecycle, context env injection, and plugin config/state directories. Agent Hub only uses documented CLI wrappers, avoiding OS-specific direct socket transport.

## Release

Add GitHub topic `herdr-plugin` on public repository. Marketplace scans public default branches every 30 minutes. Install through `herdr plugin install tapiaw38/herdr-agent-hub`.
