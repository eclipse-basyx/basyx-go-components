# AAS Repository API Setup Notes

This document summarizes the current repository-service pattern used by the AAS Repository and related services.

## Architecture

- OpenAPI contracts live in `cmd/<service>/openapi.yaml`.
- Generated Go server stubs live under `pkg/<component>api` or `pkg/<component>api/go`.
- Hand-written service implementations live under `internal/<component>/api`.
- Service entrypoints live in `cmd/<component>service/main.go`.
- Shared AAS DTOs and SDK aliases are reused instead of keeping a full copy of generated `model_*.go` files per service.

For the AAS Repository:

- contract: `cmd/aasrepositoryservice/openapi.yaml`
- generated stubs: `pkg/aasrepositoryapi/go`
- implementation: `internal/aasrepository/api`
- entrypoint: `cmd/aasrepositoryservice/main.go`

## Generate Server Stubs

Generate into a temporary folder first:

```bash
podman run --rm -v "${PWD}:/local" openapitools/openapi-generator-cli generate \
  -i /local/cmd/aasrepositoryservice/openapi.yaml \
  -g go-server \
  -o /local/regen-temp \
  --additional-properties=packageName=openapi,isGoSubmodule=true,enumClassPrefix=true,router=chi
```

Compare the generated output against `pkg/aasrepositoryapi/go` and copy only the curated server files that the package already keeps, such as `api*.go`, `routers.go`, `helpers.go`, `impl.go`, `logger.go`, and `error.go`.

Do not blindly copy generated docs, clients, or model files. The project commonly aliases generated API types to shared SDK/model types.

## Wire A Service

A DB-backed repository service should follow the existing entrypoint pattern:

1. Load YAML/env configuration.
2. Open the PostgreSQL connection.
3. Validate the schema version/state.
4. Build the component service implementation from `internal/<component>/api`.
5. Create the generated router from `pkg/<component>api`.
6. Apply OIDC/ABAC setup through the shared security helper when the service supports security.
7. Start the HTTP server with configured timeouts and graceful shutdown.

Use the existing `cmd/aasrepositoryservice/main.go` and `cmd/submodelrepositoryservice/main.go` as references for repository-like services.

## Verify Changes

```bash
go fmt ./pkg/aasrepositoryapi/... ./internal/aasrepository/... ./cmd/aasrepositoryservice/...
go test -v ./...
```
