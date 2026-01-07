# Project Structure Overview: BaSyx Go Components

This document explains the structure and purpose of each major component in the BaSyx Go Components repository. It is intended to help new developers understand how the codebase is organized and how the different layers interact.

---

## Top-Level Folders

- **cmd/**
  - Contains service entry points (main.go), configuration files, and Dockerfiles for each microservice (e.g., `aasregistryservice`, `submodelrepositoryservice`).
  - Each service folder includes its own config and healthcheck scripts.

- **api/**
  - Houses OpenAPI specifications (`openapi.yaml`) for each service, defining REST endpoints and data contracts.
  - Used for code generation and API documentation.

- **internal/**
  - Core business logic, persistence, and integration tests.
  - Organized by domain (e.g., `aasregistry`, `submodelrepository`, `discoveryservice`).
  - Each domain contains:
    - `api/`: Service layer, request/response handling, endpoint logic.
    - `persistence/`: Database access, repositories, file handlers.
    - `model/`: Data structures and types for the domain.
    - `benchmark_results/`, `integration_tests/`: Performance and integration test suites.
    - `security/`, `testenv/`: Security logic and test environments.

- **pkg/**
  - Shared libraries and API clients for use across services.
  - Contains reusable helpers, routers, and generated API code.

- **examples/**
  - Minimal working examples and sample setups, including Docker Compose files for quick local testing.

- **docu/**
  - Documentation, error explanations, and security notes.

- **basyx-database-wiki/**
  - In-depth documentation of the database schema, relationships, and usage notes.

- **sql_examples/**
  - Example SQL scripts for schema setup, sample data, and pagination.

---

## Key Components Explained

### Builders
- Located in `internal/*/builder/`
- Used for constructing complex domain objects (e.g., submodels, descriptors) from input data or configuration.
- Encapsulate creation logic to keep service and API layers clean.

### API Layer
- Found in `internal/*/api/` and `pkg/*/api.go`
- Implements REST endpoint logic, request validation, and response formatting.
- Connects HTTP requests to business logic and persistence.
- Uses OpenAPI specs for endpoint definitions.

### Model
- Defined in `internal/*/model/` and `pkg/*/model.go`
- Contains Go structs and types representing AAS, Submodels, File SME, Registry entries, etc.
- Used for serialization/deserialization and type safety across the codebase.

### Persistence
- Located in `internal/*/persistence/`
- Handles database operations, including CRUD for submodels, registries, and file attachments.
- Implements logic for PostgreSQL Large Object storage for File SME.

### Routers
- Found in `pkg/*/routers.go`
- Maps HTTP routes to handler functions, sets up middleware, and manages request lifecycle.
- Used by services to expose REST APIs.

### Security
- In `internal/*/security/` and `docu/security/`
- Implements authentication, authorization, and security best practices for API endpoints.

### Integration Tests
- In `internal/*/integration_tests/`
- End-to-end tests for service APIs, file attachment logic, and database interactions.
- Use Docker Compose for isolated test environments.

### Benchmarking
- In `internal/*/benchmark_results/`
- Performance tests and results for critical operations.

---

## How Components Interact

- **API Layer** receives HTTP requests, validates input, and calls **Builders** or **Persistence** logic.
- **Model** types are used throughout for data exchange and validation.
- **Persistence** stores and retrieves data, including large files for File SME.
- **Routers** connect endpoints to handlers and manage middleware.
- **Security** ensures only authorized access to sensitive operations.
- **Integration Tests** verify that all layers work together as expected.

---

## Example Workflow: File Attachment
1. API receives a PUT request to `/submodels/{id}/submodel-elements/{idShort}/attachment`.
2. Router maps the request to the handler in `internal/submodelrepository/api/`.
3. Handler validates input and calls the File SME builder/persistence logic.
4. File is stored in PostgreSQL using Large Object API.
5. GET requests to `/attachment` return the file, using model types for response formatting.

---

## Further Reading
- See `DeveloperDocumentation/README.md` for onboarding and godoc usage.
- Explore `basyx-database-wiki/` for database details.
- Review OpenAPI specs in `api/` for endpoint documentation.

For questions, open an issue or contact the maintainers.
