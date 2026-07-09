# Regenerating OpenAPI Server Stubs

Service OpenAPI contracts live in `cmd/<service>/openapi.yaml`. The generated Go server stubs used by the services live under `pkg/<component>api` or `pkg/<component>api/go`, depending on the package.

Generate into a temporary directory first:

```sh
podman run --rm -v "${PWD}:/local" openapitools/openapi-generator-cli generate \
  -i /local/cmd/<service>/openapi.yaml \
  -g go-server \
  -o /local/regen-temp \
  --additional-properties=packageName=openapi,isGoSubmodule=true,enumClassPrefix=true,router=chi
```

Then compare the generated files with the existing package and copy only the curated server files that the component already keeps, such as `api*.go`, `routers.go`, `helpers.go`, `impl.go`, `logger.go`, and `error.go`.

Do not blindly commit generated docs, clients, or model files. This repository commonly aliases generated API types to shared SDK/model types instead of keeping a separate OpenAPI model copy.

After updating generated stubs:

```sh
go fmt ./pkg/<component>api/...
go test -v ./...
```
