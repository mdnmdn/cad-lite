---
id: feat-settings
title: Settings
kind: feature
status: current
summary: The settings panel (theme, UI scale, drawing defaults, mouse wheel, default agent) and the TOML file in the user config folder that keeps them.
read_when: you are changing a setting, the settings file, the themes, the UI scale or the panel
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [settings/settings.go, settings/file.go, settings/recent.go, ui/settings.go, ui/settings_view.go, ui/agent_binary.go, ui/discovery.go, ui/discovery_view.go, ui/terminal.go, ui/agent_install.go, ui/update_view.go, ui/slider.go, ui/themes.go, ui/theme.go, ui/workspace.go, ui/layout.go, cmd/cad/main.go]
depends_on: [tech-ui-framework, tech-operations, feat-canvas, feat-updates]
review_cycle: monthly
---

# Settings

## Purpose

The user picks how Cad-lite looks and what a new session starts with, and it is remembered. A panel
over the window offers a light, dark, classic or high-contrast theme, a UI scale that enlarges or
reduces the whole interface including text, the drawing aids that start on, the mouse wheel
behaviour and the default agent and where agents are found. Changes show at once and are saved to a TOML file.

## Behaviour

- **Opening.** The gear `Settings` button at the right end of the title bar, **Ctrl+,** (**Cmd+,** on
  macOS) and the `OPTIONS` command (alias `OP`) open and close it. The panel is a card centred over a dimmed backdrop;
  **Escape**, the `Close` button and a click on the backdrop close it. While it is open the keyboard
  belongs to it and nothing under the backdrop receives pointer input.
- **Appearance.** Four theme cards, each a miniature in the theme's own colours; clicking one applies
  it. The UI scale runs from 75% to 200% in 5% steps, set with the `-` and `+` buttons, the slider (click the
  track, drag the thumb, or Left/Right, Home and End when it is focused) or `100%` to reset. Buttons
  and keys apply immediately; a drag shows the value while it moves and applies it on release,
  because rescaling the UI under the pointer would move the slider away from it. The themes and the
  scale mechanism are in [UI framework](../tech/ui-framework.md).
- **Language.** `Auto`, `English` or `Italiano`. `Auto` follows the system language (`LC_ALL`,
  `LC_MESSAGES`, `LANG`, in that order; English when none names a supported language). The workspace
  stores the choice and calls the hook installed with `Workspace.SetLanguageHandler` (on
  registration, on load and on every change); `cmd/cad` installs a hook that calls
  `i18n.Set(i18n.FromSetting(lang))`, and `Workspace.change` sets the language itself before the
  hook runs, so prompts, history lines, keywords, panel labels, tooltips and the settings warnings
  switch language with the next frame. Command names stay valid in English whatever the language, Italian
  names are extra aliases (see [Command line](command-line.md)), and an AI agent driving the
  drawing always gets English. Warnings produced while the file is read are in the system language,
  because the stored choice is not known yet.
- **Drawing.** Grid, object snap and ortho switches set what starts on and apply to every open
  document tab and each new one too (toggling with F7, F3 or F8 afterwards does not change the stored default). The mouse
  wheel is `auto` or `zoom` (the wheel zooms, Shift pans) or `pan` (the wheel pans, Ctrl or Cmd zooms);
  `Workspace.WheelMode` reports it to the canvas.
- **Panel layout.** The title bar (Settings, Close) and the footer (where the file is, Reset to
  defaults) stay in place; only the body between them scrolls, with a thin scrollbar. The card is at most the
  window height minus a 24-pixel margin, so on a short window nothing falls below the screen edge.
  The wheel or trackpad over the panel or its backdrop scrolls the body and never reaches the canvas. The
  body returns to the top each time the panel opens.
- **AI agent.** The default harness (`auto`, `opencode`, `claude`, `codex`); `auto` takes the first one
  found, in that order. Below it, **Executables**: one row per agent with the program Cad-lite starts for
  it — the file the user chose (`chosen by you`, with the version it reported), else the one on the PATH
  (`found on the PATH`), else `not found` — a **Browse…** button and, for a chosen file, **Reset**.
  Browse opens the native open dialog (the workspace's file dialog, as for Open) titled for that agent,
  starting in the folder of the executable in use, else the home folder; on Windows it lists `.exe`,
  `.cmd`, `.bat` and `.com` programs first, elsewhere every file. The chosen file must exist, not be a
  folder and be executable (Windows: one of those extensions); it is then asked for `--version` (3 s at
  most, silence is accepted). A refused file leaves the setting alone and shows why under the row; a
  cancelled dialog does nothing; without a dialog backend the row says to edit `agent.binaries` in the
  file. A chosen file wins over the PATH for discovery, for the agent panel's chips and for every
  session start; Reset goes back to the PATH. A chosen file that has since disappeared is flagged in
  red. After a change the discovery probe runs again if it ran before.
  found, in that order. The default model per harness has no control here: the agent panel's model menu sets
  it (*Make default for …*, see [Agent panel](agent-panel.md)) and the file keeps it as `agent.model_<harness>`.
- **Agent discovery.** Which of OpenCode, Claude and Codex were found, each with its full path and the
  version its `--version` reports (marked when it is the executable chosen under AI agent), or `not found`; a line saying where the PATH came from (login shell,
  Windows registry, or the app's own when the probe failed, with the reason) and a `Show PATH` expander
  that lists the folders the login shell added. **Check** runs the probe again (the button shows
  `Checking…` meanwhile); **Open terminal** opens the user's terminal in the project folder, to install or
  log in to an agent by hand, and reports when none could be started. The probe runs once in the background
  at startup, so `auto` works without a click. Nothing is persisted. While OpenCode is not found, or an
  installation is under way, the section ends with the *Install OpenCode* card described in the
  [agent panel](agent-panel.md). How the PATH is found and used is in
  [Agent integration](../tech/agent-integration.md).
- **About and updates.** The version, the time of the last check, the automatic-check switch, a status
  line, *Check for updates* and, when a release waits, *Update*. Everything about it is in [Updates](updates.md).
- **Saving.** Every change is written about half a second after the last one, and on close, with the
  window's size. The panel footer shows where the file is, or that settings are kept in memory.
  `Reset to defaults` restores every value but keeps unknown keys, the stored window size, the panel layout, the recent lists and the last update check.
- **Loading.** The file is read at startup. A missing file gives the defaults. A value of the wrong
  type or out of range is replaced (the UI scale is clamped to 0.75 to 2.0 and snapped to 0.05, window
  sides to 320 to 16384, panel sizes to 0 to 4096) and one warning per value shows in the panel footer. An unknown theme name falls back to `dark`.
  Keys this version does not know are kept and written back. A file without the `recent` table gives empty recent
  lists; recent entries that are not strings, empty or duplicated are dropped, and each list is cut to 10, without a warning.
- **Recent drawings.** The `recent` table is written by the app, not the panel: the drawings opened or saved
  lately and their folders, which the Open menu and the start panel list. What records an entry and what a
  click does are in [DXF files](dxf-files.md).

## Contract

The file is `cad-lite/settings.toml` in the OS user config folder (copied once from the old `go-draw/` folder when only that exists), or `$CADLITE_CONFIG_DIR/settings.toml`
([Operations](../tech/operations.md)). Tables and keys:

| Key | Values | Default |
|---|---|---|
| `appearance.theme` | `dark`, `light`, `classic`, `high-contrast` | `dark` |
| `appearance.language` | `auto`, `en`, `it` | `auto` |
| `appearance.ui_scale` | 0.75 to 2.0, step 0.05 | 1.0 |
| `drawing.grid`, `drawing.osnap`, `drawing.ortho` | boolean | true, true, false |
| `drawing.wheel` | `auto`, `zoom`, `pan` | `auto` |
| `agent.default` | harness name, or empty | empty |
| `agent.binaries` | inline table of harness name to executable path, e.g. `{ claude = "/opt/claude/bin/claude" }`; empty paths are dropped, unknown names kept | `{}` |
| `agent.model_<harness>` (`model_opencode`, `model_claude`, `model_codex`) | model id new tabs on that harness start with; absent for the harness's own default | absent |
| `update.auto_check` | boolean | true |
| `update.last_check` | timestamp, written by the app | unset |
| `window.width`, `window.height` | logical pixels, 0 for unset | 1280, 800 |
| `layout.side_width`, `layout.agent_width`, `layout.layers_height`, `layout.history_height` | logical pixels, 0 for automatic, at most 4096 | 0 |
| `recent.files`, `recent.folders` | arrays of absolute paths, newest first, at most 10 each, written by the app | empty |
| `layout.side_collapsed`, `layout.layers_collapsed`, `layout.properties_collapsed`, `layout.history_collapsed`, `layout.ribbon_collapsed` | boolean | false |

The `layout` table is the arrangement of the workspace panels, written by the app when the user drags a
splitter or folds a panel (see [Drawing canvas](canvas.md)); a size of 0 is automatic, and a saved size is
kept even when a smaller window cannot show it. The file starts with a comment header and a comment per table; comments a user adds are not kept.
API: package `settings` (`Settings`, `Default`, `Path`, `Load`, `LoadWarn`, `Parse`, `Marshal`, `Save`,
`ClampUIScale`, `Recent` with `AddFile`, `AddFolder`, `RemoveFile`, `RemoveFolder`, `Clear`, `CleanPath`);
`Workspace.RecentFiles`, `RecentFolders`, `ClearRecent`, `OpenMenuShown`, `StartPanelShown`; `Workspace.EnableAgentDiscovery`, `AgentDiscovery`, `SetAgentDiscoveryProbe`, `SetTerminalOpener` (`ui.TerminalOpener`), `ui.OpenTerminal`, `ui.OpenTerminalCommand`; `settings.Layout`; `Workspace.LoadSettings`, `LoadSettingsFrom`, `Settings`, `SetSettings`,
`SaveSettings`, `SetLanguageHandler`, `Language`, `OpenSettings`, `CloseSettings`, `ToggleSettings`, `WheelMode`; theme registry
`ui.Themes`, `ThemeByName`, `ApplyTheme`; element `ui.NewSlider` with `SliderState`; action
`cad-settings`.

## Implementation

`settings/settings.go` defines the values and `Normalize`; `settings/file.go` holds `Path`, `Parse`
(into `Settings`, unknown keys into `Extra`), `Marshal` and the atomic `Save`; `settings/recent.go` holds `Recent`
and its list rules (temporary file in the
same directory, mode 0600, directory 0700, then rename). `ui/settings.go` keeps the workspace's
settings state, `applySettings` (theme, scale, drawing defaults, agent and its executables), the debounced save through
`Window.After` and the hook `withSettings`, which `Workspace.Render` calls to mount the panel and the
action; `ui/settings_view.go` renders the gear button and the panel; `ui/slider.go` is the reusable
`Slider`. `ui/agent_binary.go` holds the executables rows: `applyAgentBinaries` hands `Settings.AgentBinaries` to `harnesses.SetBinary` (on load and on every change) and reprobes the agent panel; `browseAgentBinary` runs the workspace's `FileDialog.Open` and `harnesses.Check` on a goroutine, and `pollBinary` adopts the answer on the UI goroutine (tests install a fake dialog with `SetFileDialog`). `ui/discovery.go` holds the discovery state (a worker goroutine hands its result to the UI goroutine through a poll timer, as the update check does), `ui/discovery_view.go` the section and `ui/terminal.go` `OpenTerminal` (macOS `open -a Terminal`, Windows `cmd /c start powershell`, elsewhere `$TERMINAL`, x-terminal-emulator, gnome-terminal, konsole or xterm, the first installed) and `OpenTerminalCommand`, which runs a program in that terminal and keeps the window open afterwards (macOS `osascript` with Terminal's `do script`, Windows a new PowerShell `-NoExit` console or `cmd /k`, elsewhere `sh -c` waiting for Enter). Hosts call `Workspace.EnableAgentDiscovery` to start the background probe (`cmd/cad` does, except for `-png`); tests use `SetAgentDiscoveryProbe` and `SetTerminalOpener`. The panel body scrolls through a `gpui.ScrollHandle`, and `scrollSettings` consumes every wheel event over the overlay. `cmd/cad/main.go` sets the system language, calls `Workspace.LoadSettings` before the window opens,
applies the stored language with `i18n.Set` and takes the window size from it; a loaded UI scale
applies on the first frame. The wording of the warnings is in `i18n/locales` (`settings.warn.*`).

## Failure

- No config folder (the browser build, or no home directory) or an unreadable or malformed file:
  the defaults apply, settings stay in memory, and the panel footer shows the error.
- A save that fails (read-only folder, full disk) shows its error in the footer; the running
  application keeps the settings, and the next change tries again. The previous file is never left
  half-written.

## Related docs

- [UI framework and theme](../tech/ui-framework.md) — the themes, tokens and the UI scale
- [Operations](../tech/operations.md) — where the file lives and `CADLITE_CONFIG_DIR`
- [Drawing canvas](canvas.md) — the wheel, grid and the status-bar toggles
- [Agent panel](agent-panel.md) — the harness the default agent selects
- [Updates](updates.md) — the About and updates section

## Next steps

- Persist the agent panel width and the side panel visibility.
- A key-binding editor.
