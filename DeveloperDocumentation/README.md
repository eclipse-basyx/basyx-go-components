# BaSyx Go Components - Developer Documentation

Welcome to the BaSyx Go Components project! This documentation is designed to help new developers quickly understand the codebase, its architecture, and best practices for contributing.

## Table of Contents
- [Project Overview](#project-overview)
- [Repository Structure](#repository-structure)
- [Getting Started](#getting-started)
- [Key Concepts](#key-concepts)
- [Code Organization & Best Practices](#code-organization--best-practices)
- [How to Contribute](#how-to-contribute)
- [Godoc Usage](#godoc-usage)
- [Example: Service Startup](#example-service-startup)
- [Example: Running Integration Tests](#example-running-integration-tests)
- [Database Schema](#database-schema)
- [Frequently Asked Questions](#frequently-asked-questions)

## Project Overview

BaSyx Go Components is an open-source implementation of the Eclipse BaSyx framework in Go, providing Asset Administration Shell (AAS) services, registries, repositories, and more. The project is modular, scalable, and designed for industrial digital twin scenarios.

## Repository Structure

- `cmd/` - Service entry points (main.go, Dockerfiles, configs)
- `api/` - OpenAPI specs and API layer
- `internal/` - Core business logic, persistence, and integration tests
- `pkg/` - Shared libraries and API clients
- `examples/` - Minimal working examples and sample setups
- `docu/` - Documentation and security notes
- `basyx-database-wiki/` - In-depth database schema documentation

## Getting Started

1. **Clone the repository**
2. **Install Go (>=1.20)**
3. **Run integration tests**: `go test -v ./internal/...`
4. **Start services**: Use Docker Compose files in `integration_tests/docker_compose/`

## Key Concepts

- **Asset Administration Shell (AAS)**: Digital representation of assets
- **Submodel**: Modular part of an AAS, e.g., file attachments, operations
- **Registry/Repository**: Services for storing and discovering AAS and submodels
- **File SME**: Submodel element for file attachments, supports large object storage

## Code Organization & Best Practices

- Use Go modules (`go.mod`) for dependency management
- Follow GoDoc conventions for all exported functions and types
- Write integration and unit tests for new features
- Use the provided linter (`lint.sh`) before submitting PRs
- Document public APIs in OpenAPI YAML files

## How to Contribute

- Fork the repository and create a feature branch
- Write clear commit messages and PR descriptions
- Add or update GoDoc comments for all exported code
- Run tests and ensure they pass before submitting

## Godoc Usage

All packages are documented using GoDoc. To view documentation locally:

```sh
godoc -http=:6060
```
Then visit [http://localhost:6060/pkg/](http://localhost:6060/pkg/) in your browser.

## Example: Service Startup

```sh
cd cmd/submodelrepositoryservice
GO111MODULE=on go run main.go -config config.yaml
```

## Example: Running Integration Tests

```sh
go test -v ./internal/submodelrepository/integration_tests
```

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
