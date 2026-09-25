# BaSyx ReBAC Example (experimental)

This example runs the AAS Environment with ABAC **and** the experimental
relationship-based access control (ReBAC). Owners share Shells, Submodels,
SubmodelElements, Concept Descriptions, descriptors and discovery entries
with users or groups without changing the ABAC policy. Relationships are
stored and evaluated in the BaSyx PostgreSQL database; no extra service is
needed. See [docu/security/REBAC.md](../../docu/security/REBAC.md) for the
full documentation, in particular the section on how ReBAC and ABAC
interplay.

## Services

| Service | Purpose |
| --- | --- |
| `aas-environment` | BaSyx AAS Environment on <http://localhost:8082> with `ABAC_ENABLED=true`, `REBAC_ENABLED=true` and registry synchronization |
| `basyx_configuration` | Applies the BaSyx database schema |
| `keycloak` | Identity provider on <http://keycloak.localhost:8080> (realm `basyx`) |
| `db` | PostgreSQL with the BaSyx database |

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
2. `alice` creates a Submodel and becomes its owner. `bob` gets `403` for
   the Submodel and for its synchronized registry descriptor.
3. `alice` shares the Submodel with `bob` as `viewer`. `bob` gets `200` for
   the Submodel and for the descriptor, which follows its Submodel.
4. `alice` revokes the grant. `bob` gets `403` again.

## Manual walkthrough

```bash
token() {
  curl -s -d "grant_type=password&client_id=basyx-ui&username=$1&password=pwd" \
    http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])'
}
ALICE=$(token alice)
DAVE=$(token dave)

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

# Review and verify the audit trail of access changes (administrators)
curl -H "Authorization: Bearer $DAVE" "http://localhost:8082/security/rebac/admin/audit?limit=20"
curl -H "Authorization: Bearer $DAVE" "http://localhost:8082/security/rebac/admin/audit/verify"
```

## Notes

- All BaSyx services of one PostgreSQL database share the same
  relationships. Enable ReBAC consistently in all of them.
- The Keycloak realm and credentials of this example are for demonstration
  only.
