# TinyGo Build Issue: Vela v0.1.0 + TinyGo 0.35.0

## Summary

Building a Vela WASM app locally against `vela v0.1.0` and `vela-common-go v0.1.0` fails with TinyGo due to a Go version mismatch. This document explains the root cause, all error manifestations, and the working fix.

---

## Root Cause

There is a fundamental version gap between the Vela v0.1.0 Go dependencies and TinyGo:

| Component | Go version |
|-----------|-----------|
| `vela v0.1.0` + `vela-common-go v0.1.0` | Require **Go >= 1.24** |
| `tinygo/tinygo:0.35.0` | Bundles **Go 1.23.4** internally |

`vela-common-go v0.1.0` also injects a `toolchain go1.24.13` directive into go.mod when `go mod tidy` runs.

TinyGo 0.35.0 rejects any go.mod with `go >= 1.24` or a `toolchain` directive. This is not a bug in the voting app code — it is a gap between when the Vela team bumped their minimum Go version (v0.1.0) and when TinyGo ships a release bundling Go 1.24.

> **Why didn't the payment app hit this?** The `payment_app.wasm` binary is downloaded pre-built from GitHub releases. Nobody compiles it locally. The voting app is the first app in the tutorial where users build WASM from source, which is why this surfaced here.

---

## Error Chain

Each fix revealed a new layer of the same underlying mismatch.

### Error 1 — `missing go.sum entry`

```
verifying github.com/HorizenOfficial/vela@v0.1.0: missing go.sum entry
```

**Cause:** `go.sum` did not exist — `go mod tidy` had never been run.

**Fix:** Run `go mod tidy` inside the build container before compiling.

---

### Error 2 — `requires go version 1.19 through 1.23, got go1.24` (TinyGo reading go.mod)

```
requires go version 1.19 through 1.23, got go1.24
```

**Cause:** `go mod tidy` wrote `go 1.24.0` into go.mod (to satisfy the dependency requirement). TinyGo 0.34.0 / 0.35.0 read this directive and rejected the build.

**Attempted fix:** Upgrade TinyGo image from `0.34.0` → `0.35.0`. Still failed — TinyGo 0.35.0 also bundles Go 1.23.4.

---

### Error 3 — `requires go@1.24.0, but 1.23 is requested`

```
go: github.com/HorizenOfficial/vela@v0.1.0 requires go@1.24.0, but 1.23 is requested
```

**Cause:** Added `-go=1.23` flag to `go mod tidy` to prevent the version bump. The dependencies hard-require `>= 1.24`, so tidy refused.

**Fix:** Removed the `-go=1.23` pin — it conflicts with the dependency requirements.

---

### Error 4 — `go.mod requires go >= 1.24.0 (running go 1.23.4; GOTOOLCHAIN=local)`

```
go: go.mod requires go >= 1.24.0 (running go 1.23.4; GOTOOLCHAIN=local)
```

**Cause:** Used `GOTOOLCHAIN=local` to prevent toolchain download. Revealed that TinyGo 0.35.0's bundled Go is 1.23.4 — it cannot satisfy the `>= 1.24` requirement in go.mod.

---

### Error 5 — `toolchain go1.24.13` in go.mod

```
requires go version 1.19 through 1.23, got go1.24
```

**Cause:** `go mod tidy` (run via `golang:1.24` container) wrote both `go 1.24.0` and `toolchain go1.24.13` into go.mod. The `sed` command only patched the `go` line, leaving the `toolchain` line — TinyGo still rejected it.

**Fix:** Extended `sed` to also remove the `toolchain` line:
```bash
sed -i 's/^go 1\.24.*/go 1.23.0/; /^toolchain /d' go.mod
```

---

### Error 6 — `vela-common-go requires go >= 1.24.0 (running go 1.23.4)`

```
go: github.com/HorizenOfficial/vela-common-go@v0.1.0 requires go >= 1.24.0 (running go 1.23.4)
```

**Cause:** TinyGo's container (Go 1.23.4) downloaded the dependencies itself during `tinygo build`. Go's module system checked `vela-common-go`'s own go.mod and refused to use it with Go 1.23.4.

**Fix:** Vendor the dependencies in stage 1 (with `golang:1.24`) so TinyGo never downloads them. With `-mod=vendor`, TinyGo reads source from disk and skips all version checks.

---

## Working Fix

Two-stage Makefile — `golang:1.24` handles deps, TinyGo handles the WASM compile:

```makefile
build:
    # Stage 1: download deps with real Go 1.24, vendor them, strip Go 1.24 directives
    docker run --rm --platform linux/amd64 \
      -v $(shell pwd):/app -w /app \
      golang:1.24 \
      sh -c "go mod tidy && go mod vendor && sed -i 's/^go 1\.24.*/go 1.23.0/; /^toolchain /d' go.mod"
    # Stage 2: TinyGo builds from vendor/, never touches the network or checks dep versions
    docker run --rm --platform linux/amd64 \
      -v $(shell pwd):/app -w /app \
      tinygo/tinygo:0.35.0 \
      tinygo build -target=wasi -mod=vendor -o voting_app.wasm .
```

What the `sed` does in one pass:
1. `s/^go 1\.24.*/go 1.23.0/` — downgrades the `go` directive to 1.23.0
2. `/^toolchain /d` — removes the `toolchain go1.24.13` line

After patching, go.mod is:
```
module github.com/example/vela-voting-app

go 1.23.0

require (
    github.com/HorizenOfficial/vela v0.1.0
    github.com/HorizenOfficial/vela-common-go v0.1.0
)
```

TinyGo 0.35.0 accepts `go 1.23.0`, reads all source from `vendor/`, and compiles successfully.

---

## Long-term Fix

This workaround will no longer be needed once TinyGo ships a release that bundles Go 1.24+. At that point, the Makefile can revert to a single-stage build:

```makefile
build:
    docker run --rm --platform linux/amd64 \
      -v $(shell pwd):/app -w /app \
      tinygo/tinygo:<version-with-go1.24> \
      sh -c "go mod tidy && tinygo build -target=wasi -o voting_app.wasm ."
```

Monitor TinyGo releases at https://github.com/tinygo-org/tinygo/releases for a version that lists Go 1.24 support.
