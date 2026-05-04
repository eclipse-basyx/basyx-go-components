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
  - `admin` / `pwd`: full read + write access (can upload AASX files)
  - `usera` / `pwd`: read-only access to all submodels
  - not logged in (anonymous): can read Nameplate and CarbonFootprint only (DPP data)

## Test Scenario

Use the provided AASX file for this walkthrough:

- [`aas/IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx)

The access rules in this example are aligned to the IDs contained in that file.

### 1) Admin Login + Upload (Required Setup)

1. Open UI at [http://localhost:3000](http://localhost:3000)
2. Log in as `admin` (see credentials above)
3. Upload [`aas/IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx)

Expected behavior:

- Upload succeeds
- AAS **IESEDriveMotorDM3000** is visible with all 5 submodels:
  Nameplate, TechnicalData, HandoverDocumentation, ContactInformations, CarbonFootprint

### 2) Read-Only User Check (`usera`)

Log in as:

- User: `usera`
- Password: `pwd`

Expected behavior:

- `usera` can read the AAS and all 5 submodels
- Create/update/delete operations are denied

### 3) Logout Check (Expected: Limited Visibility)

1. Log out

Expected behavior:

- Anonymous users can see the AAS shell and read **Nameplate** and **CarbonFootprint** (public Digital Product Passport data)
- **TechnicalData**, **ContactInformations**, and **HandoverDocumentation** are not visible (require authentication)

### 4) Anonymous Upload Attempt (Expected: Fail)

1. Without logging in, try to upload the AASX file

Expected behavior:

- Write is denied by security rules
- Current UI may show a success message, but data is **not** persisted

## Access Rules Explained

The behavior above is configured in:

- [`security_env/access-rules.json`](security_env/access-rules.json)

Rule model reference (ACLs, formulas, object groups, and rule wiring):

- [IDTA-01004 Access Rule Model (v3.0.2)](https://industrialdigitaltwin.io/aas-specifications/IDTA-01004/v3.0.2/access-rule-model.html)

This file defines three access levels:

1. **Anonymous** (not logged in)

   - ACL `anonymous_read` with `READ` rights
   - Object group `public_product_info` grants access to the AAS shell, Nameplate, and CarbonFootprint (Digital Product Passport)
   - Other submodels (TechnicalData, ContactInformations, HandoverDocumentation) require authentication

2. **Viewer** (`usera`, role = `viewer`)

   - ACL `viewer_read` with `READ` rights
   - Formula `is_viewer` checks `role = viewer`
   - Object group `all_api` grants read access to all API resources and submodels

3. **Admin** (`admin`, role = `admin`)

   - ACL `admin_full` with `ALL` rights
   - Formula `is_admin` checks `role = admin`
   - Object group `all_api` grants full CRUD access

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
