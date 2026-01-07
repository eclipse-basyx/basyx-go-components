# structure_internal.md: Core Logic (internal/)

## Purpose
Contains the main business logic, persistence, and integration tests for each domain (e.g., registry, submodel repository, discovery).

## Subfolders
- `api/`: Service layer, endpoint handlers, request/response logic
- `persistence/`: Database access, repositories, file handlers
- `model/`: Domain data structures and types
- `builder/`: Object construction logic
- `integration_tests/`: End-to-end and scenario tests
- `security/`: Authentication and authorization logic
- `benchmark_results/`: Performance test results
- `testenv/`: Test environment setup

## Example: submodelrepository
- `api/`: Handles REST requests, validates input, calls persistence
- `persistence/Submodel/submodelElements/FileHandler.go`: Manages file attachments using PostgreSQL Large Objects
- `model/`: Defines Go structs for submodels, files, etc.
- `integration_tests/`: Tests upload/download/delete of file attachments

## How to Extend
- Add new domain folders for new features
- Implement business logic in `api/` and `persistence/`
- Add tests in `integration_tests/`
