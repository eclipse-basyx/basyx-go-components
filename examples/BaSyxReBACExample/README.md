# BaSyx ReBAC Example (experimental)

This example runs the AAS Environment with ABAC **and** the experimental
relationship-based access control (ReBAC) backed by OpenFGA. Owners share
Submodels, Shells, SubmodelElements and Concept Descriptions with users or
groups without changing the ABAC policy. See
[docu/security/REBAC.md](../../docu/security/REBAC.md) for the full
documentation, in particular the section on how ReBAC and ABAC interplay.

## Services

| Service | Purpose |
| --- | --- |
| `aas-environment` | BaSyx AAS Environment on <http://localhost:8082> with `ABAC_ENABLED=true` and `REBAC_ENABLED=true` |
| `basyx_configuration` | Applies the BaSyx schema and provisions the OpenFGA store and model (`REBAC_PROVISION_MODEL=true`) |
| `openfga-migrate` | One-shot `openfga migrate` of OpenFGA's own schema |
| `openfga` | OpenFGA 1.18 with a pre-shared key; not published on the host |
| `keycloak` | Identity provider on <http://keycloak.localhost:8080> (realm `basyx`) |
| `db` | PostgreSQL with the BaSyx database and a separate `openfga` database and user |

Users (password `pwd`): `alice`, `bob`, `carol` (group `engineering`),
`dave` (group `operators`, configured ReBAC administrator), `eve`, and
`admin` (full ABAC rights).

## Run

```bash
docker compose up -d
./smoke.sh
```

The smoke script walks through the basic flow:

1. `dave` (administrator) lets `alice` create Submodels by granting
   `creator` on the `submodel` repository.
2. `alice` creates a Submodel and becomes its owner. `bob` gets `403`.
3. `alice` shares the Submodel with `bob` as `viewer`. `bob` gets `200`.
4. `alice` revokes the grant. `bob` gets `403` again.

## Manual walkthrough

```bash
token() {
  curl -s -d "grant_type=password&client_id=basyx-ui&username=$1&password=pwd" \
    http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])'
}
ALICE=$(token alice)

# Inspect grants and the access revision (ETag) of a Submodel
curl -i -H "Authorization: Bearer $ALICE" \
  "http://localhost:8082/submodels/<base64url id>/\$access"

# Replace grants (If-Match is required)
curl -X PUT -H "Authorization: Bearer $ALICE" -H 'If-Match: "1"' \
  -H 'Content-Type: application/json' \
  "http://localhost:8082/submodels/<base64url id>/\$access/grants" \
  -d '{"grants":[
        {"relation":"owner","subjectType":"user","issuer":"http://keycloak.localhost:8080/realms/basyx","subject":"<alice sub>"},
        {"relation":"viewer","subjectType":"group","issuer":"http://keycloak.localhost:8080/realms/basyx","subject":"engineering"}]}'

# Invite someone with a one-time link valid for one day
curl -X POST -H "Authorization: Bearer $ALICE" -H 'Content-Type: application/json' \
  "http://localhost:8082/submodels/<base64url id>/\$access/invitations" \
  -d "{\"relation\":\"viewer\",\"expiresAt\":\"$(date -u -v+1d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d tomorrow +%Y-%m-%dT%H:%M:%SZ)\"}"
```

## Notes

- The OpenFGA pre-shared key in this example is for demonstration only.
  Use a secret in real deployments and never publish the OpenFGA port.
- All BaSyx services of one PostgreSQL database share one ReBAC scope
  (`REBAC_SCOPE`) and one OpenFGA store.
