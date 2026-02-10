# BaSyx Digital Twin Registry Example

> Status: **Work in progress** – not all filters and access rules from the reference implementation are implemented yet.

This example shows how to run a BaSyx-based Digital Twin Registry that mirrors the concepts and behavior of the Tractus-X Digital Twin Registry as described in the Cross-Cutting Concepts document:

- Tractus-X DTR Cross-Cutting Concepts: https://github.com/eclipse-tractusx/sldt-digital-twin-registry/blob/main/docs/architecture/6-crosscutting-concepts.md

The goal of this example is to provide a local, docker-compose-based setup that closely follows that document while still being easy to experiment with and adapt.

## What is implemented

- Digital Twin Registry service running on `http://localhost:5004` with base path `/api/v3` (can be configured).
- PostgreSQL database for persistence, started automatically via docker-compose.
- ABAC-based access rules and trust list wiring (model and trust list loaded from `security_env`).
- Keycloak identity provider running on `http://localhost:8080` for configuring realms, clients, and users.

**Important:** This is an **in-progress** implementation:
- Not all filters and access rules from the Tractus-X Cross-Cutting Concepts are implemented yet.
- `externalSubjectId` handling is currently simplified (see below) and will be aligned with the reference document in a later iteration.

## Prerequisites

- Docker
- Docker Compose (v2 or compatible)

Run all commands from the directory of this example:

- `examples/BaSyxDigitalTwinRegistryExample`

## How to start the example

From this example directory, start all services via:

```bash
docker compose up -d
```

This will start:
- PostgreSQL database (port `5432`).
- Digital Twin Registry (port `5004`).
- Keycloak (port `8080`).

To stop and remove containers:

```bash
docker compose down
```

## Services and endpoints

### Digital Twin Registry API

- Base URL: `http://localhost:5004/api/v3`
- OpenAPI / Swagger UI: `http://localhost:5004/api/v3/swagger`

The main endpoints relevant for this example are:

- `POST /shell-descriptors`
  - Use this to create (register) shell descriptors in the registry.
  - In this example, **no access token is required** by default.
  - In a real setup, a claim check in the access rules would enforce appropriate tokens.
  - Optionally, you can send the BPN of the caller in the HTTP header `Edc-Bpn`. This value is injected as a claim into the access rules engine and can be used there (for example, to match against `externalSubjectId`).

- `GET /shell-descriptors`
  - List shell descriptors from the registry.
  - Shells are filtered based on `externalSubjectId` according to the configured access rules.
  - Currently, `externalSubjectId` values are **still exposed in the response** for this endpoint; this is an in-progress behavior and will later be aligned with the Cross-Cutting Concepts document.
  - As with `POST /shell-descriptors`, you can optionally send the BPN of the caller in the HTTP header `Edc-Bpn`, which is injected as a claim into the access rules engine and used to filter visible shells.

- `GET /lookup/shellsByAssetLink`
  - Lookup shell descriptors by asset link.
  - Queries can include `externalSubjectId`. In this example, such IDs are **queried** but **not exposed in responses** (see below).
  - Optionally, you can send the BPN of the caller in the HTTP header `Edc-Bpn`. This value is injected as a claim into the access rules engine and is used to filter which data is returned.

- `GET /lookup/shells/{id}`
  - Lookup a specific shell descriptor by its ID.
  - As with the previous endpoint, `externalSubjectId` information is currently filtered out from responses.
  - Optionally, you can send the BPN of the caller in the HTTP header `Edc-Bpn`. This value is injected as a claim into the access rules engine and is used to filter which data is returned.

#### `externalSubjectId` behavior (temporary)

Currently, this example:
- Supports queries involving `externalSubjectId`.
- For most endpoints, **filters out `externalSubjectId` values from responses**.
- For `GET /shell-descriptors`, `externalSubjectId` is currently still exposed in the response so that the filtering behavior can be inspected; this will be aligned with the reference behavior in a later iteration.

The injected `Edc-Bpn` claim from the HTTP header is used in the access rules to filter data based on the `externalSubjectId`:
- Data whose `externalSubjectId` matches the caller’s BPN (from `Edc-Bpn`) can be made visible.
- Data that is marked as publicly readable can also be made visible.
- Other data is filtered out according to the configured access rules.

This is an intentional, temporary simplification. The long-term goal is to align the behavior exactly with the Tractus-X Cross-Cutting Concepts specification (see link above), where `externalSubjectId` is treated similarly to other fields, subject to fine-grained access rules rather than complete filtering.

## Security and access rules

Security for the Digital Twin Registry is configured via ABAC access rules and trust lists:

- Access rules model path: `/security_env/access-rules.json`
- Trust list path: `/security_env/trustlist.json`

These files are mounted into the `digitaltwinregistry` container via the `security_env` directory in this example. You can adjust them to:
- Enforce token-based access.
- Add claim checks for specific operations (for example, requiring certain claims for `POST /shell-descriptors`).
- Refine which data is visible to which subjects.

In this example configuration:
- The `POST /shell-descriptors` endpoint is usable without a token.
- You can extend or tighten access rules by editing the JSON models in `security_env` and restarting the containers.

## Keycloak configuration

Keycloak is available at:

- `http://localhost:8080`

From there you can:
- Configure realms, clients, and users.
- Adapt the identity setup to your organization.

The concrete Keycloak configuration (realm import, admin credentials, etc.) is defined in:

- This example directory: see `keycloak/` and the `keycloak` service section in `docker-compose.yml`.

## Configuration via docker-compose

The core behavior of this example is configured via environment variables in:

- `examples/BaSyxDigitalTwinRegistryExample/docker-compose.yml`

Notable options include:
- `SERVER_PORT` and `SERVER_CONTEXTPATH` for the registry’s HTTP port and base path.
- PostgreSQL connection parameters (`POSTGRES_*`).
- ABAC and trust list paths (`ABAC_MODELPATH`, `OIDC_TRUSTLISTPATH`).
- Flags such as `ABAC_ENABLED`, `GENERAL_ENABLEIMPLICITCASTS`, `GENERAL_ENABLEDESCRIPTORDEBUG`.

You can adapt these settings to your environment, then restart the stack with `docker compose up -d`.

## Relation to Tractus-X DTR

This example is designed to **mirror the implementation** of the Tractus-X Digital Twin Registry’s cross-cutting concepts:

- Reference document: https://github.com/eclipse-tractusx/sldt-digital-twin-registry/blob/main/docs/architecture/6-crosscutting-concepts.md

When you experiment with this example, you can read that document side by side with this README to understand how:
- Access rules are modeled and enforced.
- Fields like `externalSubjectId` are meant to be handled.
- Security and trust configuration are intended to work in a production-grade setup.

Because this repository is still evolving, this example **does not yet cover all filters and access rules** described in the document, but it is structured to match that architecture and can be extended to reach full parity over time.