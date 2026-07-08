# BaSyx Go Components: Structure Overview

This file provides a high-level map of the repository and links to in-depth documentation for each major component. Use this as your starting point for exploring the codebase.

## Main Components

- [cmd/](structure_cmd.md): Service entry points, configuration, and OpenAPI contracts (`cmd/*/openapi.yaml`)
- [internal/](structure_internal.md): Core business logic, persistence, and tests
- [pkg/](structure_pkg.md): Generated Go server stubs and reusable packages
- [examples/](structure_examples.md): Sample setups and minimal examples
- [docu/](structure_docu.md): Documentation and security notes
- [AAS API v3.2 user guide](../user/aas_api_v3_2.md): User-visible v3.2 endpoints, history behavior, recent changes, signed reads, and operational notes
- [basyx-database-wiki/](../basyx-database-wiki/): Database schema documentation
- [sql_examples/](structure_sqlexamples.md): Example SQL scripts
- [AAS API v3.2 runtime notes](aas_v3_2_runtime.md): History, recent changes, migration behavior, and scalability notes

---

For onboarding, see [README.md](../../README.md). For GoDoc tips, see [godoc_tips.md](godoc_tips.md).
