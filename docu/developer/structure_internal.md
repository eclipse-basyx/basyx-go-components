# structure_internal.md: Core Logic (internal/)

## Purpose

`internal/` contains service implementations, shared runtime helpers, persistence, security, and test support that should not be imported by external modules.

## Common Subfolders

Component folders vary by service, but common patterns include:

- `api/`: service-specific HTTP handlers and generated-interface implementations
- `persistence/`: database access, repositories, descriptors, and file handlers
- `model/`: component-local data structures when shared SDK types are not enough
- `builder/`: construction and mapping logic for complex AAS objects
- `integration_tests/`, `query_integration_tests/`, `security_tests/`, `migration_integration_tests/`: package-specific test suites
- `benchmark/`: package-specific benchmark code and results where present

Shared infrastructure lives mostly under `internal/common`, including `model`, `builder`, `history`, `jws`, `queries`, `security`, and `testenv`.

## Example: submodelrepository

- `api/`: handles REST requests, validates input, and calls persistence
- `persistence/Submodel/submodelElements/FileHandler.go`: manages File SME metadata and PostgreSQL Large Object content
- `integration_tests/`: covers upload, download, delete, query, and history behavior through the API

## How To Extend

Add new behavior in the component package that owns it, reuse `internal/common` helpers when a behavior is already shared, and add tests in the closest existing test package.
