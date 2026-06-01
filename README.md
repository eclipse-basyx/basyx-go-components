[![Go Report Card](https://goreportcard.com/badge/github.com/eclipse-basyx/basyx-go-components)](https://goreportcard.com/report/github.com/eclipse-basyx/basyx-go-components)
![Docker Pulls](https://img.shields.io/endpoint?url=https%3A%2F%2Fdocker-stats.basyx.org%2Fgo.json)

<div align="center">
  <img src="docu/assets/Logo_Go_light.svg" alt="BaSyx Go Logo" width="400"/>
</div>

# BaSyx Go Components

Welcome to the BaSyx Go project! This guide is designed to help new developers onboard quickly, understand the architecture, and contribute effectively.

> [!NOTE]
> We also provide a wiki at https://wiki.basyx.org with a extensive user and developer documentation.

## Table of Contents

- [BaSyx Go Components](#basyx-go-components)
  - [Table of Contents](#table-of-contents)
  - [1. Project Overview](#1-project-overview)
  - [2. Architecture Overview](#2-architecture-overview)
  - [3. Setup \& Installation](#3-setup--installation)
    - [Prerequisites](#prerequisites)
    - [Steps](#steps)
  - [4. Environment Variables \& Configuration](#4-environment-variables--configuration)
    - [Database Initialization](#database-initialization)
  - [5. Code Style \& Conventions](#5-code-style--conventions)
  - [6. Module Structure](#6-module-structure)
  - [7. Common Workflows](#7-common-workflows)
    - [Build \& Run](#build--run)
    - [Test](#test)
    - [Lint](#lint)
  - [8. API Usage](#8-api-usage)
  - [9. Contribution Guidelines](#9-contribution-guidelines)
  - [10. Repository Automation](#10-repository-automation)
  - [11. Security \& Supply Chain Verification](#11-security--supply-chain-verification)
  - [12. Troubleshooting \& Error Reference](#12-troubleshooting--error-reference)
  - [13. Glossary of Terms \& Abbreviations](#13-glossary-of-terms--abbreviations)
  - [Database Schema](#database-schema)
  - [Frequently Asked Questions](#frequently-asked-questions)
  - [Further Reading](#further-reading)

---

## 1. Project Overview

BaSyx Go Components is an open-source implementation of the Eclipse BaSyx framework in Go, providing Asset Administration Shell (AAS) API components like registries, repositories, and more. The project is modular, scalable, and designed for industrial digital twin scenarios.

## 2. Architecture Overview

The project is composed of microservices for AAS and Submodel registries, AAS and Submodel repositories, the AAS discovery service, and the AASX file server. Each service exposes REST APIs and interacts with a PostgreSQL database. Security is enforced via OIDC and ABAC middleware. See [docu/security/README.md](docu/security/README.md) for a detailed flow and architecture diagram.

## 3. Setup & Installation

### Prerequisites

- Go >= 1.20
- Docker & Docker Compose
- PostgreSQL (for local development)

### Steps

1. Clone the repository:
     ```sh
     git clone https://github.com/eclipse-basyx/basyx-go-components.git
     cd basyx-go-components
     ```
2. Install dependencies:
     ```sh
     go mod tidy
     ```
3. Start services (example):
     ```sh
     go run ./cmd/basyxconfigurationservice/main.go -config ./cmd/basyxconfigurationservice/config.yaml -databaseSchema ./database/base.sql -customPatchPath ./database/patches
     go run ./cmd/submodelrepositoryservice/main.go -config ./cmd/submodelrepositoryservice/config.yaml
     ```
4. Run integration tests:
     ```sh
     go test -v ./internal/submodelrepository/integration_tests
     ```
5. Use Docker Compose for multi-service setup:
     ```sh
     docker-compose -f examples/BaSyxMinimalExample/docker-compose.yml up
     ```

## 4. Environment Variables & Configuration

### Database Initialization

DB-backed BaSyx services expect the shared PostgreSQL schema to be initialized before startup. Run `basyxconfigurationservice` once against the target database before starting repository, registry, discovery, or environment services. The configuration service loads `database/base.sql`, applies versioned patches from `database/patches`, and records the schema version and schema state in `basyxsystem`. Runtime services validate that the schema state is `clean` and that the version matches during startup; they fail fast if the configuration service has not completed successfully. If a schema patch fails during execution, the database is marked `dirty` until the configuration service completes a compatible patch run successfully.

For operator-facing setup guidance, see the [BaSyx wiki](https://wiki.basyx.org)

The `basyxconfigurationservice` image should use the same BaSyx version or build revision as the DB-backed runtime components in the same setup. This is especially important when using mutable image tags. The `latest` tag tracks the newest release, and the `SNAPSHOT` tag tracks the current main-branch snapshot; both tags may point to different image digests over time. For reproducible deployments, pin a concrete version tag, commit/SNAPSHOT tag, or image digest instead.

If a setup uses mutable tags and pulls images on every start or restart, include `basyxconfigurationservice` in the deployment and run it before the DB-backed components. Otherwise a freshly pulled runtime component may expect a newer schema version than the database currently contains, causing startup validation to fail.

Configuration is managed via YAML files in `cmd/<service>/config.yaml` and environment variables. Key variables include database connection settings (see [docu/errors.md](docu/errors.md) for troubleshooting):

```yaml
maxOpenConnections: 500
maxIdleConnections: 500
connMaxLifetimeMinutes: 5
```

Or via `.env`:

```env
POSTGRES_MAXOPENCONNECTIONS=500
POSTGRES_MAXIDLECONNECTIONS=500
POSTGRES_CONNMAXLIFETIMEMINUTES=5
```

For `aasenvironmentservice`, `aasrepositoryservice`, and `submodelrepositoryservice`, uploads are additionally bounded by:

```yaml
general:
    uploadMaxSizeBytes: 134217728
```

This value limits the maximum accepted request body size for upload endpoints:

- `POST /upload`
- `PUT /shells/{aasIdentifier}/thumbnail`
- `PUT /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment`

For `aasenvironmentservice`, startup preconfiguration can import AAS files automatically:

```yaml
general:
    aasPreconfigPaths:
        - ./examples/BaSyxMinimalExample/aas
        - ./myDevice.aasx
```

Or via environment variable:

```env
GENERAL_AAS_PRECONFIG_PATHS=./examples/BaSyxMinimalExample/aas,./myDevice.aasx
```

Each configured source can be a file or directory. Directories are scanned recursively for `.aasx`, `.json`, and `.xml` files.

Upload and startup preconfiguration use the AAS 3.1 parsing stack. For backward compatibility, XML payloads with lower or equal AAS v3 namespace versions (for example `https://admin-shell.io/aas/3/0`) are adapted to the current namespace before parsing, and a warning is logged.

## 5. Code Style & Conventions

- Use Go modules (`go.mod`) for dependency management
- Follow GoDoc conventions ([godoc_tips.md](docu/developer/godoc_tips.md))
- Run the linter before submitting PRs:
    ```sh
    ./lint.sh
    ```
- Use `//nolint:<linter>` comments sparingly and explain why
- Auto-generated model files may not conform to all linter rules

## 6. Module Structure

- `cmd/` - Service entry points, configs, Dockerfiles
- `cmd/*/openapi.yaml` - Service OpenAPI specifications and API contracts
- `internal/` - Core business logic, persistence, integration tests
- `pkg/` - Autogenerated API Files
- `examples/` - Minimal working examples, Docker Compose setups
- `docu/` - Documentation, error explanations, security notes
- `basyx-database-wiki/` - Database schema documentation, including `basyxconfigurationservice` schema-version and clean/dirty state behavior

See [structure.md](docu/developer/structure.md) and related files for details on each module.

## 7. Common Workflows

### Build & Run

- Build and run services:
    ```sh
    go run ./cmd/<service>/main.go -config ./cmd/<service>/config.yaml
    ```
- Use VSCode launch scripts in `.vscode/launch.json` for debugging

### Test

- Run all tests:
    ```sh
    go test -v ./internal/...
    ```
- Run integration tests for a component:
    ```sh
    go test -v ./internal/<component>/integration_tests
    ```

### Lint

- Run linter:
    ```sh
    ./lint.sh
    ```

## 8. API Usage

- OpenAPI specs are in `cmd/<service>/openapi.yaml` (generated copies in `pkg/*/api/openapi.yaml`)
- Use generated API clients in `pkg/`
- Example endpoint: `/submodels/{id}/submodel-elements/{idShort}/attachment`
- AAS environment import endpoint: `/upload` (multipart/form-data with file part `file`)
- Supported upload media types: `application/aasx+xml`, `application/aasx+json`, `application/asset-administration-shell+xml`, `application/asset-administration-shell+json`, `application/json`, `application/xml`, `text/xml`
- AAS v3.2 history and recent changes: [user guide](docu/user/aas_api_v3_2.md) and [runtime notes](docu/developer/aas_v3_2_runtime.md)
- See [structure_cmd.md](docu/developer/structure_cmd.md) for details

## 9. Contribution Guidelines

- Fork the repository and create a feature branch
- Write clear commit messages and PR descriptions
- Add/update GoDoc comments for all exported code
- Run tests and linter before submitting
- Cover new features with integration tests
- Document public APIs in OpenAPI YAML files

## 10. Repository Automation

- CI/CD pipelines run tests and linter on each PR
- Ensure your local environment matches CI versions (see [linter.md](docu/developer/linter.md))
- Use provided tasks in VSCode or CLI for build/test/lint automation

## 11. Security & Supply Chain Verification

- Security reporting policy: [Eclipse BaSyx SECURITY.md](https://github.com/eclipse-basyx/.github/blob/main/SECURITY.md)
- Supply-chain trust model, signing, attestations, and SBOM verification: [docu/security/SUPPLY_CHAIN_SECURITY.md](docu/security/SUPPLY_CHAIN_SECURITY.md)
- Runtime security architecture (OIDC + ABAC): [docu/security/README.md](docu/security/README.md)

Release and snapshot images are signed with Cosign keyless identity and include provenance and SBOM attestations. For full verification commands, see [docu/security/SUPPLY_CHAIN_SECURITY.md](docu/security/SUPPLY_CHAIN_SECURITY.md).

## 12. Troubleshooting & Error Reference

- See [docu/errors.md](docu/errors.md) for common error scenarios and solutions
- For security and authorization, see [docu/security/REGISTRY_SECURITY.md](docu/security/REGISTRY_SECURITY.md)

## 13. Glossary of Terms & Abbreviations

- **AAS**: Asset Administration Shell – digital representation of assets
- **Submodel**: Modular part of an AAS
- **SME**: Submodel Element
- **OIDC**: OpenID Connect – authentication protocol
- **ABAC**: Attribute-Based Access Control
- **OpenAPI**: Specification for RESTful APIs

---

For further details, see the [docs folder](docu), the [BaSyx wiki](https://wiki.basyx.org), and links above. If you encounter issues, please open an issue on GitHub or consult the [error documentation](docu/errors.md).

## Database Schema

See [basyx-database-wiki](docu/basyx-database-wiki/) and [sql_examples](sql_examples/) for details on tables, relationships, and large object handling.

## Frequently Asked Questions

**Q: How do I add a new component?**

- Add main.go in `cmd/<COMPONENT_NAME>/main.go`
- Implement the logic in `internal/<COMPONENT_NAME>/`
- Save and use OpenAPI specs in `cmd/<COMPONENT_NAME>/openapi.yaml`
- Add tests in `internal/<COMPONENT_NAME>/integration_tests/`

**Q: How do I handle file attachments?**

- Use the File SME logic in `internal/submodelrepository/persistence/Submodel/submodelElements/FileHandler.go`
- See integration tests for upload/download examples

**Q: Where do I find API documentation?**

- OpenAPI YAML files in `cmd/*/openapi.yaml` (generated copies in `pkg/*/api/openapi.yaml`)
- GoDoc for package-level documentation

## Further Reading

- [Eclipse BaSyx Documentation](https://www.eclipse.org/basyx/)
- [IDTA AAS Specification](https://industrialdigitaltwin.org/en/content-hub/aasspecifications)

---

For any questions, open an issue or contact the maintainers.
