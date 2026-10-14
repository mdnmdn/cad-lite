---
id: meta-review-log
title: Review log
kind: meta
status: current
summary: Append-only log of documentation maintenance passes, one line per pass.
read_when: you are starting a maintenance pass and need to know what the last ones did
updated: 2026-10-14
verified: 2026-10-14
depends_on: [meta-librarian]
review_cycle: quarterly
---

# Review log

One line per pass, newest last: date · scope · checks run · counts. Never a silent pass.

| date | scope | checks | result |
|---|---|---|---|
| 2026-10-14 | Founding pass: `_cad-doc/` adopted as the main `_docs/`. `draft-specs.md` filed verbatim in `inbox/completed/` and distilled into `product/`, `features/`, `tech/`, `backlog.md` and the decision register; `_meta/` adapted from the Ubiq rulebook to go-draw (Go paths, `tools/docs` for the `just docs-*` checks, `internal/gpui` as the separate library) | L1, L2, L4, L5, L7, L8, L9, L10 via `just docs-lint`; L3 via `just docs-drift` | all documents pass; 0 drifted anchors; 1 proposal opened (`P1`) |

## Related docs

- [`librarian.md`](./librarian.md) — what a pass checks and reports
- [`feedback.md`](./feedback.md) — the proposals passes open
