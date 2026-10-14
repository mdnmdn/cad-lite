# go-draw tasks. Each recipe is a plain command, so `just` is optional.

# gofmt, vet and test everything
check:
    test -z "$(gofmt -l .)"
    go vet ./...
    go test ./...

# run the desktop app
run *args:
    go run ./cmd/cad {{args}}

# render the demo drawing headlessly
png path="_tmp/cad.png":
    go run ./cmd/cad -png {{path}}

# build the browser (js/wasm) binary
wasm:
    GOOS=js GOARCH=wasm go build -o _tmp/cad.wasm ./cmd/cad

# lint _docs/ against _docs/_meta/librarian.md
docs-lint:
    go run ./tools/docs lint

# documents anchored to the given paths (default: the working diff)
docs-touched *paths:
    go run ./tools/docs touched {{paths}}

# documents whose anchors changed after they were verified
docs-drift:
    go run ./tools/docs drift

# regenerate the INDEX.md catalogue and tech/code-map.md
docs-index:
    go run ./tools/docs index

# lint the icon registry and SVGs, and fail if names_gen.go is stale
icons-check:
    uv run tools/icons/icons.py check

# regenerate assets/icons/names_gen.go from the registry
icons-gen:
    uv run tools/icons/icons.py codegen

# a review sheet: `just icons-sheet draw-line`, or `just icons-sheet --category draw`
icons-sheet *ARGS:
    uv run tools/icons/icons.py sheet {{ARGS}}

# copy Lucide icons in as `adopted:` rows: `just icons-adopt` (all unfilled) or by name
icons-adopt *ARGS:
    uv run tools/icons/icons.py adopt {{ARGS}}

# compare the competing takes in assets/icons/variants/: `just icons-variants --category draw`
icons-variants *ARGS:
    uv run tools/icons/icons.py variants {{ARGS}}

# one contact sheet per category: which icon breaks the family?
icons-audit *ARGS:
    uv run tools/icons/icons.py audit {{ARGS}}

# pairs of icons that look alike
icons-dupes:
    uv run tools/icons/icons.py dupes
