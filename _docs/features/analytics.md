---
id: feat-analytics
title: Analytics
kind: feature
status: current
summary: Anonymous application activity events through a nonblocking recorder, stdout dummy provider and optional build-configured PostHog ingestion.
read_when: you are adding activity events, changing analytics providers or configuring ingestion in desktop builds
updated: 2026-10-17
verified: 2026-10-17
code_anchors: [analytics/analytics.go, analytics/sinks.go, analytics/analytics_test.go, appenv/system.go, appenv/system_windows.go, appenv/system_other.go, cmdsys/analytics.go, cmdsys/context.go, cmdsys/session.go, cmdsys/commands/analytics_test.go, ui/analytics.go, ui/analytics_test.go, ui/doctabs.go, ui/agent.go, ui/agent_store.go, ui/update.go, cmd/cad/analytics.go, cmd/cad/analytics_test.go, cmd/cad/main.go, packaging/lib.sh, packaging/macos/bundle.sh, packaging/windows/bundle.sh, packaging/linux/bundle.sh, .github/workflows/ci.yml, .github/workflows/release.yml]
depends_on: [tech-architecture, tech-command-system, tech-operations, feat-updates]
review_cycle: quarterly
---

# Analytics

## Purpose

Cad-lite registers application activity through one event interface. A stdout implementation makes
integration visible during development; configured desktop builds send anonymous events to PostHog.
Analytics cannot prevent a drawing operation or present a delivery error to the user.

## Behaviour

- Interactive desktop launches register `app_started` and a best-effort `app_stopped` on exit.
  Startup events include application name/version, OS, OS version and architecture. System metadata
  is collected on the background worker, with a two-second OS-version query deadline and `unknown`
  fallback. Windows queries hide their subprocess window. The same metadata accompanies subsequent
  events.
- `command_used` records a registered command's canonical name and whether its caller is a user or
  agent. `drawing_opened` and `drawing_saved` contain only a format category (`dxf`, `dwg`, `other`).
  A successful startup `-open` also registers the open. Rendering with `-png`, printing `-version`,
  browser builds and workspaces without an installed recorder emit nothing.
- `agent_message` records user messages and the first nonempty assistant delta for each message ID
  in a turn, up to 128 IDs; chunks do not inflate message counts. User events include attachment
  count. `agent_tool_used` records tool kind and, for the CAD server, its structured tool name.
  `agent_session_started` and `agent_turn_finished` record backend lifecycle activity.
- `error` records categories and fixed codes for unknown commands, invalid input, recovered command
  panics, command failure messages, unsupported files, agent failures, tool failures, update failures,
  and host startup/settings failures. It contains no raw exception or stderr text.
- Events never include prompt/response text, tool arguments/results, drawing contents, full paths,
  user names or provider credentials. Each client uses a fresh random identifier for its run;
  there is no persistent installation identity. PostHog person-profile processing is disabled.
- A bounded queue accepts up to 128 events without waiting for HTTP or stdout. Queue overflow,
  invalid properties, provider failures and provider panics are silently discarded. Properties are
  copied as at most 32 scalar values, with bounded names and strings. Delivery has a three-second
  deadline. There are no persistent analytics files and no delivery retries.
- On normal exit the host allows at most 500 ms to flush, then cancels delivery. Missing or invalid
  PostHog configuration selects stdout. `CADLITE_ANALYTICS=stdout` explicitly chooses the dummy;
  `CADLITE_ANALYTICS=off` disables both providers. Legacy `GODRAW_ANALYTICS` is also recognized.

## Contract

Package `analytics`: `Properties`, `Recorder.Track`, `Event`, `Sink.Send`, `Client`, `New`,
`Record`, `Client.Track`, `Client.Close`, `Stdout`, `PostHog`. `Record` isolates recorder panics;
custom recorder implementations must honor the nonblocking interface contract. Only the host
owns client shutdown. `Workspace.SetAnalytics` installs the recorder for existing and future
sessions; `Session.SetAnalytics` allows headless hosts to install one directly.

## Implementation

`cmd/cad/analytics.go` selects a provider and registers startup before opening drawings. `New`
starts the worker; `appenv.SystemMetadata` supplies system information without a UI dependency.
`cmdsys/analytics.go` records command starts and classifies stable command-history message keys;
`Context.Say` passes through the session to keep translation and event classification together.
The UI records agent activity when folding backend events, independently of transcript storage.
No analytics delivery runs on the UI thread or inside a document transaction.

`Stdout.Send` writes one JSON line. `PostHog.Send` posts JSON with `api_key`, `event`, `distinct_id`,
`timestamp` and `properties` to `/i/v0/e/`, using the public project capture token. The URL can be
an ingestion host or that endpoint, including a self-hosted path prefix. Redirects are refused and
returned errors never echo keys, URLs or response bodies. See the official
[PostHog API](https://posthog.com/docs/api) and
[anonymous event capture](https://posthog.com/docs/data/anonymous-vs-identified-events).

`resolve_analytics` in `packaging/lib.sh` base64 encodes optional `POSTHOG_URL` and `POSTHOG_KEY`
for the `main.posthogURLBase64` and `main.posthogKeyBase64` linker values. CI and release desktop
steps read repository variable `POSTHOG_URL` and secret `POSTHOG_KEY`. Both are required for remote
capture. Browser builds receive neither. The key is a project ingestion token, which remains
extractable from a desktop binary; it is never a personal administration key.

## Failure

Failures remain in the background and never change command results, drawing history or agent
transcripts. A closed/full queue drops new events. A canceled shutdown drops remaining events.
An unavailable endpoint or failing stdout writer loses that event without opening a dialog or
recursively producing analytics errors. Custom sinks must honor context cancellation; the host's
flush deadline still bounds waiting for a sink that fails to do so.

## Related docs

- [Operations](../tech/operations.md)
- [Architecture](../tech/architecture.md)
- [Command system](../tech/command-system.md)
- [Updates](updates.md)
