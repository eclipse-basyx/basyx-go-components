# BaSyx Go Components - Developer Documentation

Welcome to the BaSyx Go Components project! This guide is designed to help new developers onboard quickly, understand the architecture, and contribute effectively.

## Table of Contents
1. [Project Overview](#project-overview)
2. [Architecture Overview](#architecture-overview)
3. [Setup & Installation](#setup--installation)
4. [Environment Variables & Configuration](#environment-variables--configuration)
5. [Code Style & Conventions](#code-style--conventions)
6. [Module Structure](#module-structure)
7. [Common Workflows](#common-workflows)
8. [API Usage](#api-usage)
9. [Contribution Guidelines](#contribution-guidelines)
10. [Repository Automation](#repository-automation)
11. [Troubleshooting & Error Reference](#troubleshooting--error-reference)
12. [Glossary of Terms & Abbreviations](#glossary-of-terms--abbreviations)

---

## 1. Project Overview

BaSyx Go Components is an open-source implementation of the Eclipse BaSyx framework in Go, providing Asset Administration Shell (AAS) services, registries, repositories, and more. The project is modular, scalable, and designed for industrial digital twin scenarios.

## 2. Architecture Overview

The system is composed of microservices for AAS registry, submodel repository, discovery, and file server. Each service exposes REST APIs and interacts with a PostgreSQL database. Security is enforced via OIDC and ABAC middleware. See [docu/security/DISCOVERY_AUTHORIZATION_README.md](../docu/security/DISCOVERY_AUTHORIZATION_README.md) for a detailed flow and architecture diagram.

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
	 go run ./cmd/submodelrepositoryservice/main.go -config ./cmd/submodelrepositoryservice/config.yaml -databaseSchema ./basyxschema.sql
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

Configuration is managed via YAML files in `cmd/<service>/config.yaml` and environment variables. Key variables include database connection settings (see [docu/errors.md](../docu/errors.md) for troubleshooting):

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

## 5. Code Style & Conventions

- Use Go modules (`go.mod`) for dependency management
- Follow GoDoc conventions ([godoc_tips.md](godoc_tips.md))
- Run the linter before submitting PRs:
	```sh
	./lint.sh
	```
- Use `//nolint:<linter>` comments sparingly and explain why
- Auto-generated model files may not conform to all linter rules

## 6. Module Structure

- `cmd/` - Service entry points, configs, Dockerfiles
- `api/` - OpenAPI specs and API layer
- `internal/` - Core business logic, persistence, integration tests
- `pkg/` - Shared libraries and API clients
- `examples/` - Minimal working examples, Docker Compose setups
- `docu/` - Documentation, error explanations, security notes
- `basyx-database-wiki/` - Database schema documentation

See [structure.md](structure.md) and related files for details on each module.

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

- OpenAPI specs are in `api/<service>/openapi.yaml`
- Use generated API clients in `pkg/`
- Example endpoint: `/submodels/{id}/submodel-elements/{idShort}/attachment`
- See [structure_api.md](structure_api.md) for details

## 9. Contribution Guidelines

- Fork the repository and create a feature branch
- Write clear commit messages and PR descriptions
- Add/update GoDoc comments for all exported code
- Run tests and linter before submitting
- Cover new features with integration tests
- Document public APIs in OpenAPI YAML files

## 10. Repository Automation

- CI/CD pipelines run tests and linter on each PR
- Ensure your local environment matches CI versions (see [linter.md](linter.md))
- Use provided tasks in VSCode or CLI for build/test/lint automation

## 11. Troubleshooting & Error Reference

- See [docu/errors.md](../docu/errors.md) for common error scenarios and solutions
- For security and authorization, see [docu/security/DISCOVERY_AUTHORIZATION_README.md](../docu/security/DISCOVERY_AUTHORIZATION_README.md)

## 12. Glossary of Terms & Abbreviations

- **AAS**: Asset Administration Shell – digital representation of assets
- **Submodel**: Modular part of an AAS
- **SME**: Submodel Element
- **OIDC**: OpenID Connect – authentication protocol
- **ABAC**: Attribute-Based Access Control
- **OpenAPI**: Specification for RESTful APIs

---

For further details, see module-specific docs in this folder and links above. If you encounter issues, please open an issue on GitHub or consult the error documentation.

## Database Schema

See `basyx-database-wiki/` and `sql_examples/` for details on tables, relationships, and large object handling.

## Frequently Asked Questions

**Q: How do I add a new submodel type?**
- Implement the submodel logic in `internal/submodelrepository/`
- Update OpenAPI specs in `api/submodelrepository/openapi.yaml`
- Add tests in `integration_tests/`

**Q: How do I handle file attachments?**
- Use the File SME logic in `internal/submodelrepository/persistence/Submodel/submodelElements/FileHandler.go`
- See integration tests for upload/download examples

**Q: Where do I find API documentation?**
- OpenAPI YAML files in `api/`
- GoDoc for package-level documentation

## Further Reading

- [Eclipse BaSyx Documentation](https://www.eclipse.org/basyx/)
- [IDTA AAS Specification](https://industrialdigitaltwin.org/en/content-hub/aasspecifications)

---

For any questions, open an issue or contact the maintainers.
