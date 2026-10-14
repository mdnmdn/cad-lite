# AGENTS.md

Guidance for AI agents working in this repo.

## What this is

go-draw is an AutoCAD-style 2D drafting application in Go. The UI framework under it is an internal
port of Zed's GPUI at `internal/gpui/`, which has its own docs and `AGENTS.md`.

Start at `_docs/INDEX.md`: the map, reading paths by task, and which document owns which fact.

## The duty

**A change that touches code, behaviour or a contract updates the documents it touched, in the same
commit. Read `_docs/_meta/authoring.md` first; it is short and self-sufficient.**
`go run ./tools/docs touched` names the documents your diff touches.

## Rules

1. **Dependency boundaries.** Core packages (`geom`, `doc`, `spatial`, `fileio`, `render`, `cmdsys`)
   never import `internal/gpui` or ebiten. Only `render/gpuirender`, `ui` and `cmd/` import gpui;
   only `cmd/` imports `internal/gpui/ebitenplatform`. Details: `_docs/tech/architecture.md`.
2. **Model space is `float64`.** Narrow to `float32` only at the gpui boundary, after subtracting
   the view centre.
3. **Transactions only.** The document changes inside a `doc.Txn`; a command is one undo step.
4. **Contract first for gpui.** Work inside `internal/gpui` follows `internal/gpui/AGENTS.md`; edit
   `internal/gpui/_docs/api-contract.md` before changing a signature there.
5. **Style.** A doc comment on every exported identifier; comments elsewhere only when the reason
   is non-obvious.
6. **Verification before done.** `gofmt -l .` prints nothing, `go vet ./...`, `go test ./...` and
   `go run ./tools/docs lint` pass. Run `go mod tidy` after adding an import.
7. **Bug fixes get a regression test** that fails first.
8. **Rendering changes are checked visually.** Render with `go run ./cmd/cad -png _tmp/x.png`
   (see `_docs/tech/operations.md` for flags) before and after, and look at the images.

## Working in this environment

- **No window.** Agent shells have no display or GPU, and ebiten fails at startup. Do not try to fix
  that. Verify with headless tests and `-png` renders, and say which interactive behaviour was not
  run.
- **Parallel agents.** Stay inside the files your brief names. If a fix belongs in someone else's
  file, stop and report it. A transient build break from another agent's edit usually clears in a
  minute or two.
- **Scratch.** Experiments, renders and binaries go in `_tmp/` (gitignored). `*.png` is gitignored
  everywhere.
- **Commits.** Small, descriptive messages, no AI attribution trailer. Do not commit unless asked.

## Layout

`geom`, `doc`, `spatial`, `fileio` (+`dxf`), `render` (+`gpuirender`), `cmdsys` (+`commands`), `ui`,
`cmd/cad`, `tools/docs` and `internal/gpui`. Each package's purpose is in `_docs/tech/code-map.md`;
build, test and run commands are in `_docs/tech/operations.md`.
