# Integration Tests

## Purpose

Integration tests exercise a component through its HTTP API and the real database-backed persistence path. Most packages start an isolated compose stack in `TestMain` and then run Go tests against the service endpoints.

## Location

- `internal/<component>/integration_tests/`
- related packages such as `query_integration_tests`, `security_tests`, `security_*_tests`, `migration_integration_tests`, and `preconfiguration_integration_tests`

## Common Test Assets

Packages choose the fixture names that fit their test runner. Current variants include:

- `it_config.json`: JSON-suite step definitions for `internal/common/testenv.RunJSONSuite`
- `postBody/`, `bodies/`, `expected/`, and `expectedResponse/`: request bodies and expected JSON responses
- `testdata/`: package-specific files such as AASX, JSON, XML, or policy fixtures
- `docker_compose/`: compose files for the package test stack

## How To Run

From the repository root, run the package you are working on:

```sh
go test -v ./internal/<component>/integration_tests
```

The mandatory Submodel Repository integration suite is:

```sh
go clean -testcache
go test -v ./internal/submodelrepository/integration_tests
```

Some VS Code tasks and action buttons exist for selected packages in `.vscode/tasks.json` and `.vscode/settings.json`; their names are package-specific, not one generic "Run Integration Tests" task.

## Adding Tests

When adding service behavior, add integration coverage in the relevant package. Prefer the shared helpers in `internal/common/testenv` for compose setup, dynamic ports, token retrieval, and JSON-suite execution.

Keep fixtures realistic and deterministic, cover both success and error cases, and clean up state through the API or the package's existing test helper pattern.
