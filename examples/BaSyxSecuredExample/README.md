# BaSyx Secured Example (Go Components + Keycloak)

This example shows a minimal secured BaSyx setup with:

- Keycloak authentication
- ABAC-based authorization
- BaSyx Web UI
- Shared PostgreSQL database

## Prerequisites

- Docker + Docker Compose
- Free ports: `3000`, `8080`, `8082`, `8083`, `8084`, `8090`, `8091`, `8092`

## Start The Example

From this folder:

```bash
docker compose up -d
```

The setup includes a one-shot DB schema init container (`db-schema-init`).  
Backends start only after:

1. Postgres is ready
2. Keycloak is ready
3. Schema init finished successfully

## Open The UI

- AAS UI: `http://localhost:3000`
- Keycloak (direct): `http://keycloak.localhost:8080`

## Credentials

- Keycloak Admin Console:
  - Username: `admin`
  - Password: `admin`
- AAS UI test users:
  - `admin` / `pwd` (read + write)
  - `usera` / `pwd` (read-only)

## Test Scenario

Use the included file:

- `example_data/ProductionPlanSFKL.aasx`

### 1) Anonymous Upload Attempt (Expected: Fail)

1. Open UI at `http://localhost:3000`
2. Try to upload `ProductionPlanSFKL.aasx` without logging in

Expected behavior:

- Write is denied by security rules
- Current UI may show a success message, but data is **not** persisted

### 2) Admin Upload (Expected: Success)

Login in UI with:

- User: `admin`
- Password: `pwd`

Upload the same AASX again.

Expected behavior:

- Upload succeeds
- Data is written and visible in UI

### 3) Logout Check (Expected: Data Hidden)

1. Log out
2. Refresh / reopen UI

Expected behavior:

- Previously uploaded data is no longer visible for anonymous user

### 4) Read-Only User Check (`usera`)

Login in UI with:

- User: `usera`
- Password: `pwd`

Expected behavior:

- Existing data is visible (read allowed)
- Create/update/delete operations are denied (write not allowed)

## Stop / Clean Up

Stop containers:

```bash
docker compose down
```

Stop and remove volumes:

```bash
docker compose down -v
```

## Notes

- Security rules are configured in `security_env/access-rules.json`.
- Trusted OIDC issuer/audience config is in `security_env/trustlist.json`.
