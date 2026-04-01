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

- AAS UI: [http://localhost:3000](http://localhost:3000)
- Keycloak (direct): [http://keycloak.localhost:8080](http://keycloak.localhost:8080)

## Credentials

- Keycloak Admin Console:
  - Username: `admin`
  - Password: `admin`
- AAS UI test users:
  - `admin` / `pwd`: read + write access (can upload AASX files)
  - `usera` / `pwd`: read-only access to Demo AAS and the second AAS
  - not logged in (anonymous): can read Demo AAS only

## Test Scenario

You can try out this **extremly simplified** test scenario to get a feeling for the security features of basyx.
Use this AASX file for upload tests:

- [`aas/ExampleV3.aasx`](aas/ExampleV3.aasx)

Only [`aas/ExampleV3.aasx`](aas/ExampleV3.aasx) is supported in this example scenario, because the predefined access-rule objects and expected IDs in this walkthrough are aligned to the content of that file.

### 1) Admin Login + Upload (Required Setup)

1. Open UI at [http://localhost:3000](http://localhost:3000)
2. Log in as `admin` (see credentials in the section above)
3. Upload [`aas/ExampleV3.aasx`](aas/ExampleV3.aasx)

Expected behavior:

- Upload succeeds for `admin`
- Demo AAS and the second AAS are visible in UI

### 2) Read-Only User Check (`usera`)

Login in UI with:

- User: `usera`
- Password: `pwd`

Expected behavior:

- `usera` can read both AAS entries (Demo AAS + second AAS)
- Create/update/delete operations are denied (write not allowed)

### 3) Logout Check (Expected: Demo AAS only)

1. Log out

Expected behavior:

- Demo AAS remains visible for anonymous users
- The second AAS is hidden for anonymous users

### 4) Anonymous Upload Attempt (Expected: Fail)

1. Without logging in, try to upload [`aas/ExampleV3.aasx`](aas/ExampleV3.aasx)

Expected behavior:

- Write is denied by security rules
- Current UI may show a success message, but data is **not** persisted

## Access Rules Explained

The behavior above is configured in:

- [`security_env/access-rules.json`](security_env/access-rules.json)

Rule model reference (ACLs, formulas, object groups, and rule wiring):

- [IDTA-01004 Access Rule Model (v3.0.2)](https://industrialdigitaltwin.io/aas-specifications/IDTA-01004/v3.0.2/access-rule-model.html)

This file defines three access types:

1. Anonymous (not logged in)

- Rule uses ACL `anonymous_read` with `READ` rights
- It is limited to objects `public_description` and `public_aas`
- `public_aas` points to the Demo AAS identifier, so anonymous users can only see Demo AAS

2. Viewer user (`usera`)

- Rule uses ACL `user` with `READ` rights
- Formula `is_user` checks `role = viewer`
- Objects `all_api` allow read access to all API resources, so `usera` can read both AAS entries
- No write rights are granted

3. Admin user (`admin`)

- Rule uses ACL `admin` with `ALL` rights
- Formula `is_admin` checks `role = admin`
- Objects `all_api` allow full API access, including upload/write operations

To change who can see or edit what, update [`security_env/access-rules.json`](security_env/access-rules.json) (ACLs, formulas, and object groups).

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

- Security rules are configured in [`security_env/access-rules.json`](security_env/access-rules.json).
- Trusted OIDC issuer/audience config is in `security_env/trustlist.json`.
