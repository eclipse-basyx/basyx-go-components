# BaSyx AASX File Server Example

This example is intended for manual Swagger UI tryout.

It uses matching host and container port mapping for the service:

- 5004:5004

This avoids Swagger Try it out issues caused by mismatched external and internal ports.

## Prerequisites

- Docker
- Docker Compose
- Free port 5004

## Start

Run from this folder:

docker compose up --build -d

## Endpoints

- Health: http://localhost:5004/health
- Swagger UI: http://localhost:5004/swagger
- OpenAPI YAML: http://localhost:5004/api-docs/openapi.yaml
- API base path: http://localhost:5004

## Quick Tryout Flow

1. Open Swagger UI and execute GET /description.
2. Use POST /packages to upload an AASX file via multipart form field named file.
3. Use GET /packages to list package descriptions.
4. Use GET /packages/{packageId} to download one package.

Optional sample file in this repository:

internal/aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx

## Stop

docker compose down

## Notes

The integration test compose at internal/aasxfileserver/integration_tests/docker_compose/docker_compose.yml intentionally uses port mapping 6004:5004 for test isolation and should remain unchanged.
