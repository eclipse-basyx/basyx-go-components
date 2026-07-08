# structure_cmd.md: Service Entry Points (cmd/)

## Purpose

`cmd/` contains executable entry points, configs, Dockerfiles, OpenAPI contracts, and service-specific resources.

## Runtime Services

Current REST service entry points include:

- `aasenvironmentservice`
- `aasregistryservice`
- `aasrepositoryservice`
- `aasxfileserverservice`
- `companylookupservice`
- `conceptdescriptionrepositoryservice`
- `digitaltwinregistryservice`
- `discoveryservice`
- `dppapiservice`
- `submodelregistryservice`
- `submodelrepositoryservice`

## Tools

- `basyxconfigurationservice`: initializes `database/base.sql`, applies `database/patches/`, and records schema state/version.
- `historyevidenceverifier`: verifies stored history evidence artifacts and manifests.

## Typical Contents

- `main.go`: service startup, configuration loading, router setup, and graceful shutdown
- `config.yaml`: service-specific defaults
- `openapi.yaml`: API contract for generated stubs and Swagger/OpenAPI serving
- `Dockerfile`: container build instructions and health checks
- `resources/` or `config/`: service-specific static resources, trust lists, and access rules

## How To Extend

Add a new command only when the behavior is an independently deployable service or operational tool. Keep startup, logging, schema validation, security setup, and graceful shutdown aligned with equivalent services.
