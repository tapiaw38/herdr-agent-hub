# Herdr integration research (2026-10-11)

Sources checked:

- [Plugin authoring](https://herdr.dev/docs/plugins/)
- [Marketplace](https://herdr.dev/docs/marketplace/)
- [CLI reference](https://herdr.dev/docs/cli-reference/)
- [Socket API](https://herdr.dev/docs/socket-api/)
- [Plugin implementation documentation in source](https://github.com/herdrdev/herdr/blob/master/docs/next/website/src/content/docs/plugins.mdx)
- [Herdr repository](https://github.com/herdrdev/herdr)
- [Manifest parser/validator source](https://github.com/herdrdev/herdr/blob/master/src/app/api/plugins/manifest.rs)
- [Plugin runtime environment source](https://github.com/herdrdev/herdr/blob/master/src/app/api/plugins/env.rs)
- [Plugin context construction source](https://github.com/herdrdev/herdr/blob/master/src/app/api/plugins/context.rs)
- [Plugin pane launcher source](https://github.com/herdrdev/herdr/blob/master/src/app/api/plugins/panes.rs)

## Confirmed contracts

Manifest requires `id`, `name`, `version`, and `min_herdr_version`; platforms are `linux`, `macos`, `windows`. Commands are argv arrays, not shell strings. `[[panes]]` accepts an id/title/placement/command; supported placements include `zoomed` and `tab`. Herdr's plugin v1 has no runtime UI or runtime action registration.

The live `herdr plugin link .` validation parsed this manifest and returned `plugin_linked` with `dashboard` entrypoint and `zoomed` placement. Source `manifest.rs` deserializes TOML into raw manifest structs, normalizes IDs/platforms/commands, rejects duplicate pane IDs, and checks `min_herdr_version` as semantic version against current binary.

Herdr plugin pane execution injects `HERDR_BIN_PATH`, `HERDR_SOCKET_PATH`, `HERDR_ENV`, plugin root/config/state dirs, `HERDR_PLUGIN_CONTEXT_JSON`, entrypoint ID, and available workspace/tab/pane IDs. The context JSON may hold workspace, tab, focused pane, worktree, agent, selected text, clicked URL, and link-handler data, depending on invocation. Plugin root is managed source and not for user data. Config/state dirs are created by Herdr; plugin owns formats and cleanup.

The dashboard queries documented `herdr api snapshot`, which emits the `session.snapshot` response containing workspace, tab, pane, and agent records. Refresh uses a cancellable CLI child process and timeout. Focus uses `herdr agent focus <agent-or-pane-id>`. Herdr agent state values include `idle`, `working`, `blocked`, `done`, and `unknown`; `done` is idle but not yet seen. Terminal reads exist as `agent read`/`pane read`, but no structured conversation-history contract is assumed.

On startup, the dashboard labels only its own `HERDR_PANE_ID` as `Agents` with documented `herdr pane rename <pane_id> <label>`. The manifest pane title also uses `Agents`. A rename failure does not prevent the dashboard from opening. Source: [Herdr CLI reference, Panes](https://herdr.dev/docs/cli-reference/).

The start/profile forms enumerate panes from that same snapshot. Pane options are labeled with snapshot workspace/tab labels, pane `cwd` or `foreground_cwd`, `title`/`terminal_title_stripped`, focus, and detected agent state. Before offering a pane for launch, Agent Hub calls documented `herdr pane process-info --pane ID`. It marks a pane ready only when `shell_pid` matches `foreground_process_group_id` and the only reported foreground process has that PID and a known shell name. Missing or ambiguous process data stays unavailable. It also excludes its own `HERDR_PANE_ID`, and it never falls back to the focused plugin pane when discovery fails. The stable `pane_id` remains internal. Herdr remains final authority and validates shell availability again at launch. Sources: [CLI reference, Panes](https://herdr.dev/docs/cli-reference/) and [Socket API, `pane.process_info`](https://herdr.dev/docs/socket-api/).

The picker can create a new shell tab using documented `herdr tab create --workspace ID --cwd PATH --label TEXT --no-focus`. It reads `.result.root_pane.pane_id`, then polls `pane process-info` until the shell reaches its prompt. If the shell does not become ready, the new tab remains intact and the UI asks the user to refresh. The UI never guesses a pane ID. `agent start` can return `agent_not_ready` or time out after launching a provider, so the UI sends only one start request and directs the user to check the live agent list before retrying. Source: [Agent automation](https://herdr.dev/docs/agent-automation/).

Agent launch uses Herdr's documented `agent start <name> --kind KIND --pane ID [-- ...]`. Herdr starts only canonical supported providers in an existing shell pane at its prompt. The UI validates the canonical kind and name, parses optional arguments as a JSON argv list, asks for confirmation, then lets Herdr validate shell availability. It never executes the profile's configurable executable field. This starts an installed CLI agent; it does not provision a model or create a new pane.

Agent Hub records its current `HERDR_PANE_ID` in `HERDR_PLUGIN_STATE_DIR`. The declared `focus-dashboard` action calls documented `herdr plugin pane focus <pane-id>`, letting a user return after focusing an agent pane via `herdr plugin action invoke community.herdr-agent-hub.focus-dashboard`. If the plugin pane is moved, reopen Agent Hub once to refresh the saved pane identity.

Herdr's plugin pane launcher defaults its process cwd to `plugin.plugin_root` when `--cwd` is absent. Agent Hub's manifest command is `./agent-hub`, which must start from that directory. The open action now passes the invoker's `focused_pane_id` as `--target-pane` for the `zoomed` placement, then the Go process changes its cwd to the invoker's workspace directory after startup. Herdr's `new_cwd = "follow"` policy can then use Agent Hub's foreground cwd when users create ordinary shell panes beside it. Agent Hub stores a dashboard pane ID per workspace, so invoking the action in a different workspace opens there rather than focusing the prior workspace's dashboard. Sources: [plugin pane launcher source](https://github.com/herdrdev/herdr/blob/master/src/app/api/plugins/panes.rs), [CLI reference](https://herdr.dev/docs/cli-reference/).

`HERDR_PLUGIN_CONTEXT_JSON` uses documented invocation fields. Agent Hub reads `workspace_id`, `workspace_label`, `workspace_cwd`, `tab_id`, `tab_label`, `focused_pane_id`, `focused_pane_cwd`, and `focused_pane_agent`. This context is fixed when the plugin pane opens, so reusing Agent Hub after navigating Herdr can leave its original directory stale. The `open-dashboard` and `focus-dashboard` actions receive a fresh context at invocation. Before focusing the existing Hub pane, they atomically store the origin workspace ID and directory in `HERDR_PLUGIN_STATE_DIR`, scoped to the server socket. Creating a new profile or starting a manual agent reads that latest origin and uses it as default. Invoking the action from inside Hub does not overwrite it. Direct pane opens without a known origin require explicit selection. Agent Hub lists current workspaces through `herdr api snapshot`, lets users choose one, and passes its documented `workspace_id` and `cwd` to `herdr tab create`. Linking an existing pane copies that pane's workspace ID and foreground directory. See [Herdr CLI reference](https://herdr.dev/docs/cli-reference/) and [Socket API](https://herdr.dev/docs/socket-api/).

Detected-agent favorites are persisted as Herdr-server-scoped pane IDs in schema version 2, because pane IDs can collide across sessions. The scope is a SHA-256-derived key from `HERDR_SOCKET_PATH`; the socket path itself is not stored. Loading schema version 1 migrates favorite profiles into the new ID list. The dashboard viewport follows the selected item, and the detail view supports bounded scrolling through Herdr's recent terminal read.

Marketplace indexing is automatic for public repositories tagged `herdr-plugin` when manifest required metadata parses. `herdr plugin install owner/repo[/subdir]` installs from GitHub. Local development uses `herdr plugin link PATH`; pane opens via `herdr plugin pane open --plugin ID --entrypoint ID`.

## Compatibility choices

- `min_herdr_version = "0.9.1"` covers the declared agent kinds plus plugin pane focus and managed agent startup; Letta Code and other newer kinds are absent from older CLI registries.
- CLI is first integration surface for cross-platform transport. Raw socket uses Unix sockets on Unix and named pipes on Windows, so no raw socket client is needed here.
- Only Linux and macOS are declared. Windows support remains unclaimed until Bubble Tea console/Herdr plugin pane behavior is verified.
- Agent statuses are mapped from Herdr state fields; absent state maps to `UNKNOWN`, and disconnected local profiles show `OFFLINE`.
