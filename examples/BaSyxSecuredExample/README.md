# BaSyx Secured Example (Go Components + Keycloak)

This example shows a minimal secured BaSyx setup with:

- Keycloak authentication
- ReBAC-first authorization for every data component
- The existing ABAC policy as a migration fallback
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

The Go services are built from the current checkout so every component uses the same database schema and security implementation. The first start can therefore take a few minutes.

The setup includes a one-shot DB schema init container (`basyx_configuration`).
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
- The creating admin becomes an owner of the created AAS, Submodels, and Submodel Elements

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

## Authorization Setup

The AAS Repository, Submodel Repository, AAS Registry, Submodel Registry, Discovery Service, and Concept Description Repository run in `resource-bound-first` mode. Their startup configuration is in [`docker-compose.yml`](docker-compose.yml). Collection access is defined by ABAC in [`security_env/access-rules.json`](security_env/access-rules.json).

The imported Keycloak user `admin` is the bootstrap owner. Its issuer and stable Keycloak user ID are configured as:

```text
issuer:  http://keycloak.localhost:8080/realms/basyx
subject: 32103f0d-1d0e-4d94-b720-bcd22e1c7709
```

The ABAC admin rule grants this identity access to collection operations. Resource creation assigns the authenticated creator as an owner, and ReBAC administration is available on concrete resources through `/$access`.

All requests not granted by ReBAC are evaluated against the existing ABAC policy. This preserves the viewer and anonymous behavior described above while the example demonstrates an incremental ABAC-to-ReBAC migration.

Registry and Discovery entries automatically inherit the policy of an AAS or Submodel with the same identifier. Entries without a matching repository resource use their own ownership or policy plus ABAC fallback. An embedded Submodel Descriptor also requires access to its containing AAS Descriptor.

### Let A User Create An AAS

Creation on `/shells` is an ABAC decision. In this example, the `viewer` ACL grants `CREATE` and `READ` on the collection routes, so `usera` can create an AAS without a ReBAC collection grant. After creation, `usera` becomes its direct owner and receives implicit `ALL` on that AAS. Other users' AAS resources remain hidden unless an effective resource policy or object-based ABAC rule grants access.

Top-level collection `/$access` endpoints do not exist. Change collection rights in the ABAC policy and use `/$access` only on concrete AAS, Submodels, Submodel Elements, descriptors, Concept Descriptions, and Discovery records.

### Verify Resource Ownership

After uploading the example AASX as `admin`, obtain an access token:

```bash
TOKEN=$(curl -fsS \
  -X POST http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d grant_type=password \
  -d client_id=basyx-ui \
  -d username=admin \
  -d password=pwd | jq -r .access_token)
```

Then read the AAS access overview:

```bash
curl -i \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8090/shells/dXJuOmZyYXVuaG9mZXI6aWVzZTpkdGU6YWFzOmRyaXZlbW90b3ItZG0zMDAwOjAwMQ/\$access
```

The response contains the owners, managers, local grants, effective policy, and an `ETag`. Any access mutation must send that current value in `If-Match`.

## ABAC Fallback Rules Explained

The behavior above is configured in:

- [`security_env/access-rules.json`](security_env/access-rules.json)

Rule model reference (ACLs, formulas, object groups, and rule wiring):

- [IDTA-01004 Access Rule Model (v3.0.2)](https://industrialdigitaltwin.io/aas-specifications/IDTA-01004/v3.0.2/access-rule-model.html)

This file defines three fallback access levels:

1. **Anonymous** (not logged in)

   - ACL `anonymous_read` with `READ` rights
   - Object group `public_product_info` grants access to the AAS shell, Nameplate, and CarbonFootprint (Digital Product Passport)
   - Other submodels (TechnicalData, ContactInformations, HandoverDocumentation) require authentication

2. **Viewer** (`usera`, role = `viewer`)

   - ACL `viewer_read` with `CREATE` and `READ` rights
   - Formula `is_viewer` checks `role = viewer`
   - Object group `all_api` admits collection reads and creation; list results still require ownership, ReBAC, or object-based ABAC

3. **Admin** (`admin`, role = `admin`)

   - ACL `admin_full` with `ALL` rights
   - Formula `is_admin` checks `role = admin`
   - Object groups `all_api` and `all_resources` grant collection access, full resource access, and access to `/verify`

To change fallback access, update [`security_env/access-rules.json`](security_env/access-rules.json) (ACLs, formulas, and object groups). To change access for one AAS, Submodel, or Submodel Element, use that resource's `/$access` API.

ReBAC is evaluated first, but the fallback is still allowed to grant a request that ReBAC did not grant. When migrating a resource to exclusive ReBAC control, remove any ABAC object rule that can still authorize it.

By default, services import this file with `ABAC_POLICY_FILE_IMPORT=if_missing`: the file is used only when no active policy exists yet for the service scope. If you want the JSON file to stay the source of truth and overwrite the stored policy on every restart, set `ABAC_POLICY_FILE_IMPORT=always` for the affected service containers. For a local reset, `docker compose down -v` removes the persisted policy state.

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
- Use concrete-resource `/$access` endpoints for ReBAC changes, or run `docker compose down -v` for a complete local reset.
