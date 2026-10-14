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
