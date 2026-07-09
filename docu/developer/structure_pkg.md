# structure_pkg.md: Generated Stubs And Reusable Packages (pkg/)

## Purpose

`pkg/` contains generated Go server stubs from service OpenAPI contracts plus reusable packages that are intended to be imported by multiple services.

## Typical Contents

- `api.go`, `api_*.go`, `routers.go`: generated server interfaces and chi routing glue
- `helpers.go`, `impl.go`, `logger.go`, `error.go`: generated or curated support code required by the stubs
- reusable service packages that are not tied to one `internal/` component

The generated packages are server-side API glue, not public client SDKs. Many model types are aliases to shared BaSyx SDK/model types instead of separate generated `model_*.go` files.

## How To Extend

Regenerate stubs from `cmd/<service>/openapi.yaml` into a temporary folder, copy only the curated files used by the existing package, then run `go fmt` and tests.
