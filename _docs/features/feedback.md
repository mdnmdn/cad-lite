---
id: feat-feedback
title: Feedback
kind: feature
status: current
summary: An application feedback dialog with rendered screenshots, reversible annotations, runtime metadata and a private delivery outbox.
read_when: you are changing feedback capture, screenshot annotation, intake delivery or build credentials
updated: 2026-10-17
verified: 2026-10-17
code_anchors: [feedback/client.go, feedback/outbox.go, feedback/image.go, feedback/metadata.go, appenv/system.go, feedback/client_test.go, feedback/image_test.go, ui/feedback.go, ui/feedback_view.go, ui/slider.go, ui/feedback_test.go, ui/titlebar.go, ui/workspace.go, cmd/cad/main.go, packaging/lib.sh, packaging/linux/bundle.sh, packaging/macos/bundle.sh, packaging/windows/bundle.sh, .github/workflows/ci.yml, .github/workflows/release.yml]
depends_on: [tech-ui-framework, tech-architecture, tech-operations]
review_cycle: quarterly
---

# Feedback

## Purpose

Users report problems and suggestions from inside Cad-lite, with an optional screenshot and enough
application and system information to reproduce them. An editor annotates the screenshot without
changing the drawing or its undo history.

## Behaviour

- A speech-balloon button beside the agent and settings buttons opens the feedback dialog. The
  background is a fixed copy of the last rendered application frame; drawing interactions are absent
  while the dialog or editor is open. Close and Escape retain the draft, including an explicit
  choice to remove its screenshot. Closing releases the background image.
- The dialog starts with an application screenshot. Capture application restores that capture;
  Attach image imports a PNG or JPEG using the native file dialog; Remove omits the attachment.
  Imported images are limited to 16 MB and 32 megapixels and all attachments fit within 2048 pixels
  on their longest side. The preview opens the editor.
- The form contains a required title (1–300 characters), a category and a description (up to 20,000
  characters). Test connection checks authentication without creating feedback and refreshes the
  service's accepted categories. Bug, Improvement and Note are the initial choices.
- The feedback dialog and editor use the same opposite light/dark palette from the application:
  dark, classic and high-contrast applications get light feedback chrome; a light application gets
  dark chrome. The
  dialog card, fields, preview surround, toolbar, buttons, focus rings and brush slider share that
  local palette. Captured screenshot colours and the application theme remain unchanged when entering or leaving the editor.
- The editor offers a pen, an eraser that restores original screenshot pixels, nine colours, a
  1–50 pixel width slider and stroke undo (also Ctrl/Cmd+Z). Save replaces the attachment; Cancel
  and Escape discard the edits. Drawing geometry and document undo are never touched.
- Send includes the application identifier and name, release version, build revision when available,
  OS, OS version, architecture, Go runtime, language, viewport and client timestamp. Drawing files,
  user names, paths and intake credentials are not included in metadata. The user reviews the image
  and description, which can contain information visible in the application.
- Before transmission, a private local outbox saves the report. Every retry reuses its ULID and
  payload. Successful delivery removes the copy and shows the public feedback reference, without
  exposing the administrative link. Send again retries the unchanged failed draft; changing the
  draft creates a new ID and replaces its failed copy. Clear explicitly discards that draft's copy.
- Startup and Retry saved attempt a batch of at most three queued reports. Delivery tries at most
  three times with cancellable exponential backoff; network errors, 429 and 500/502/503/504 retry,
  while other HTTP failures stop. Pending reports survive application exit and remain separate from
  agent history. Closing the dialog lets an authorized send continue; application shutdown cancels it.
- With missing or invalid build configuration the dialog and editor remain available, and submission,
  testing and retries are inactive. Browser builds receive no service credentials.

## Contract

Package `feedback`: `Report`, `Receipt`, `Info`, `Error`, `Client`, `Outbox`, `NewID`, `Metadata`,
`MaxImageSide`, `PrepareImage`, `DecodeImage`, `EncodeScreenshot`, `Point`, `Editor`, `NewEditor`.
Workspace: `ConfigureFeedback`, `ShutdownFeedback`. No CAD command, document transaction or
settings key is added. The outbox is `feedback-outbox` beside the settings file, honoring
`CADLITE_CONFIG_DIR`.

## Implementation

`ui/titlebar.go` opens `ui/feedback.go`; `Window.Snapshot` copies rendered pixels before the modal
is mounted. `ui/feedback_view.go` paints the frozen background, form and full-window image editor.
`feedbackTheme` selects the local contrasting palette for both dialog and editor; `newSliderWithTheme` applies it to
the brush control without changing the shared application theme. Only the UI goroutine changes
form and editor state. PNG encoding, metadata queries, native image
import and network/outbox work run outside rendering and return through buffered channels polled
by window timers. The modal owns keyboard focus and excludes the drawing's action path.

`feedback/image.go` copies, bounds and encodes images and implements pen/eraser strokes. The
`feedback` package has no UI dependency. `Client.Test` posts `{}` to `/api/feedback/test` and reads
accepted kinds; `Client.Deliver` writes the private outbox before posting JSON to `/api/feedback`
with `X-Api-Key`. Fields follow the supplied intake specification. Request bodies are bounded to
15 MB; metadata to 16,384 bytes and screenshot base64 to 14,000,000 characters. Redirects are refused
so keys never follow a redirect to another service. Responses and errors never echo credentials.

`packaging/lib.sh` encodes optional `FEEDBACK_URL` and `FEEDBACK_KEY` for linker flags, decoded by
`cmd/cad/main.go` only for desktop interactive startup. CI and release desktop steps take the URL
from repository variable `FEEDBACK_URL` and the key from secret `FEEDBACK_KEY`; missing values
produce a working build with submission disabled. Compiled desktop credentials remain extractable
from the executable and must be scoped to feedback intake.

## Failure

Invalid fields and unsupported images leave the draft intact. Cancelling the native image picker
keeps the previous attachment without reporting an error. Missing storage prevents submission.
A failed delivery retains its copy and the draft for retry; permanent service rejections need
configuration or draft correction. Outbox reads skip corrupt and non-regular entries. Shared `appenv.SystemMetadata` OS version
queries time out after two seconds and use `unknown` if unavailable. Shutdown cancellation leaves
unacknowledged outbox entries for the next start.

## Related docs

- [UI framework and theme](../tech/ui-framework.md)
- [Operations](../tech/operations.md)
- [Testing and performance](../tech/testing.md)
- [Settings](settings.md)
