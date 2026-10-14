---
id: feat-settings
title: Settings
kind: feature
status: current
summary: The settings panel (theme, UI scale, drawing defaults, mouse wheel, default agent) and the TOML file in the user config folder that keeps them.
read_when: you are changing a setting, the settings file, the themes, the UI scale or the panel
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [settings/settings.go, settings/file.go, ui/settings.go, ui/settings_view.go, ui/update_view.go, ui/slider.go, ui/themes.go, ui/theme.go, ui/workspace.go, cmd/cad/main.go]
depends_on: [tech-ui-framework, tech-operations, feat-canvas, feat-updates]
review_cycle: monthly
---

# Settings

## Purpose

The user picks how Cad-lite looks and what a new session starts with, and it is remembered. A panel
over the window offers a light, dark, classic or high-contrast theme, a UI scale that enlarges or
reduces the whole interface including text, the drawing aids that start on, the mouse wheel
behaviour and the default agent. Changes show at once and are saved to a TOML file.

## Behaviour

- **Opening.** The gear `Settings` button at the right end of the toolbar, **Ctrl+,** (**Cmd+,** on
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
- **Drawing.** Grid, object snap and ortho switches set what starts on and apply to the current
  session too (toggling with F7, F3 or F8 afterwards does not change the stored default). The mouse
  wheel is `auto` or `zoom` (the wheel zooms, Shift pans) or `pan` (the wheel pans, Ctrl or Cmd zooms);
  `Workspace.WheelMode` reports it to the canvas.
- **Agent.** The default harness (`auto`, `opencode`, `claude`, `codex`); `auto` leaves the choice to
  the first one found on `PATH`.
- **About and updates.** The version, the time of the last check, the automatic-check switch, a status
  line, *Check for updates* and, when a release waits, *Update*. Everything about it is in [Updates](updates.md).
- **Saving.** Every change is written about half a second after the last one, and on close, with the
  window's size. The panel footer shows where the file is, or that settings are kept in memory.
  `Reset to defaults` restores every value but keeps unknown keys, the stored window size and the last update check.
- **Loading.** The file is read at startup. A missing file gives the defaults. A value of the wrong
  type or out of range is replaced (the UI scale is clamped to 0.75 to 2.0 and snapped to 0.05, window
  sides to 320 to 16384) and one warning per value shows in the panel footer. An unknown theme name falls back to `dark`.
  Keys this version does not know are kept and written back.

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
| `update.auto_check` | boolean | true |
| `update.last_check` | timestamp, written by the app | unset |
| `window.width`, `window.height` | logical pixels, 0 for unset | 1280, 800 |

The file starts with a comment header and a comment per table; comments a user adds are not kept.
API: package `settings` (`Settings`, `Default`, `Path`, `Load`, `LoadWarn`, `Parse`, `Marshal`, `Save`,
`ClampUIScale`); `Workspace.LoadSettings`, `LoadSettingsFrom`, `Settings`, `SetSettings`,
`SaveSettings`, `SetLanguageHandler`, `Language`, `OpenSettings`, `CloseSettings`, `ToggleSettings`, `WheelMode`; theme registry
`ui.Themes`, `ThemeByName`, `ApplyTheme`; element `ui.NewSlider` with `SliderState`; action
`cad-settings`.

## Implementation

`settings/settings.go` defines the values and `Normalize`; `settings/file.go` holds `Path`, `Parse`
(into `Settings`, unknown keys into `Extra`), `Marshal` and the atomic `Save` (temporary file in the
same directory, mode 0600, directory 0700, then rename). `ui/settings.go` keeps the workspace's
settings state, `applySettings` (theme, scale, drawing defaults, agent), the debounced save through
`Window.After` and the hook `withSettings`, which `Workspace.Render` calls to mount the panel and the
action; `ui/settings_view.go` renders the gear button and the panel; `ui/slider.go` is the reusable
`Slider`. `cmd/cad/main.go` sets the system language, calls `Workspace.LoadSettings` before the window opens,
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
