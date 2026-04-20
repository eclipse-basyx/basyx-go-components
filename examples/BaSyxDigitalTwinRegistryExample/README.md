# BaSyx Digital Twin Registry Example

## What This DTR Is And What Makes It Special

This example runs a BaSyx Digital Twin Registry (DTR) with ABAC-based access control that follows the Tractus-X cross-cutting concept style.

What is special in this setup:
- Access decisions are not only endpoint-based, they are also data-fragment-based. A caller can receive only parts of a shell descriptor depending on claims.
- The `Edc-Bpn` HTTP header is injected as a claim and used in ABAC formulas (`bpn_or_public`, `bpn_or_public_with_header`, `bpn_match`) to filter data visibility.
- `specificAssetIds[].externalSubjectId.keys[].value` drives tenant-specific visibility. `PUBLIC_READABLE` data is visible across tenants.
- `PUT /shell-descriptors/{aasIdentifier}` has create-or-update semantics and authorization depends on whether the descriptor already exists.

Reference concept:
- https://github.com/eclipse-tractusx/sldt-digital-twin-registry/blob/main/docs/architecture/6-crosscutting-concepts.md

## Prerequisites

- Docker
- Docker Compose
- curl
- jq

Run commands from:
- `examples/BaSyxDigitalTwinRegistryExample`

Start services:

```bash
docker compose up -d
```

Services:
- DTR API: `http://localhost:5004/api/v3`
- Swagger: `http://localhost:5004/api/v3/swagger`
- Keycloak: `http://localhost:8080`

Stop services:

```bash
docker compose down
```

## Test-Aligned Actor Setup

The walkthrough below is aligned with the DTR JSON integration suite under `internal/digitaltwinregistry/integration_tests`.

Set base URL:

```bash
export DTR_BASE="http://localhost:5004/api/v3"
```

Get an admin access token (password grant, client `basyx-ui`, user `admin`, password `pwd`):

```bash
export KC_TOKEN_ENDPOINT="http://localhost:8080/realms/basyx/protocol/openid-connect/token"
export ADMIN_TOKEN=$(curl -s -X POST "$KC_TOKEN_ENDPOINT" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password" \
  -d "client_id=basyx-ui" \
  -d "username=admin" \
  -d "password=pwd" | jq -r .access_token)
```

Header profiles used in the examples:

```bash
export HDR_BPN1="Edc-Bpn: BPN_COMPANY_001"
export HDR_BPN2="Edc-Bpn: BPN_COMPANY_002"
export HDR_BPN_UNKNOWN="Edc-Bpn: BPNL00000000015G"
```

Important:
- The header examples above are for local testing/integration tests only.
- In a secured environment, end users must not be able to set or override `Edc-Bpn` directly.
- `Edc-Bpn` controls data visibility and must come from trusted identity/network infrastructure.

## Payloads (Copy-Paste)

Create `shell-descriptor.json`:

```json
{
  "idShort": "idShortExample",
  "id": "e1eba3d7-91f0-4dac-a730-eaa1d35e035c-2",
  "description": [
    {
      "language": "en",
      "text": "Example of human readable description of digital twin."
    }
  ],
  "specificAssetIds": [
    {
      "name": "partInstanceId",
      "value": "24975539203421"
    },
    {
      "name": "customerPartId",
      "value": "231982",
      "externalSubjectId": {
        "type": "ExternalReference",
        "keys": [
          {
            "type": "GlobalReference",
            "value": "BPN_COMPANY_001"
          }
        ]
      }
    },
    {
      "name": "manufacturerId",
      "value": "123829238",
      "externalSubjectId": {
        "type": "ExternalReference",
        "keys": [
          {
            "type": "GlobalReference",
            "value": "BPN_COMPANY_001"
          },
          {
            "type": "GlobalReference",
            "value": "BPN_COMPANY_002"
          }
        ]
      }
    },
    {
      "name": "manufacturerPartId",
      "value": "231982",
      "externalSubjectId": {
        "type": "ExternalReference",
        "keys": [
          {
            "type": "GlobalReference",
            "value": "PUBLIC_READABLE"
          }
        ]
      }
    }
  ],
  "submodelDescriptors": [
    {
      "endpoints": [
        {
          "interface": "SUBMODEL-3.0",
          "protocolInformation": {
            "href": "https://edc.data.plane/mypath/submodel",
            "endpointProtocol": "HTTP",
            "endpointProtocolVersion": [
              "1.1"
            ],
            "subprotocol": "DSP",
            "subprotocolBody": "body with information required by subprotocol",
            "subprotocolBodyEncoding": "plain",
            "securityAttributes": [
              {
                "type": "NONE",
                "key": "NONE",
                "value": "NONE"
              }
            ]
          }
        }
      ],
      "idShort": "idShortExample",
      "id": "cd47615b-daf3-4036-8670-d2f89349d388-2",
      "semanticId": {
        "type": "ExternalReference",
        "keys": [
          {
            "type": "Submodel",
            "value": "urn:bamm:io.catenax.serial_part_typization:1.1.0#SerialPartTypization"
          }
        ]
      },
      "description": [
        {
          "language": "de",
          "text": "Beispiel einer lesbaren Beschreibung des Submodels."
        },
        {
          "language": "en",
          "text": "Example of human readable description of submodel"
        }
      ]
    }
  ]
}
```

Create `assetlink-num1.json`:

```json
[
  {
    "name": "partInstanceId",
    "value": "24975539203421"
  }
]
```

Create `assetlink-num2.json`:

```json
[
  {
    "name": "customerPartId",
    "value": "231982"
  }
]
```

Create `assetlink-num3.json`:

```json
[
  {
    "name": "manufacturerId",
    "value": "123829238"
  }
]
```

Create `assetlink-num4.json`:

```json
[
  {
    "name": "manufacturerPartId",
    "value": "231982"
  }
]
```

## Cross-Cutting Usage Walkthrough

Use this encoded shell id in path calls:

```bash
export AAS_ID_B64="ZTFlYmEzZDctOTFmMC00ZGFjLWE3MzAtZWFhMWQzNWUwMzVjLTI"
```

1. Create without token and without `Edc-Bpn`.

```bash
curl -i -X POST "$DTR_BASE/shell-descriptors" \
  -H "Content-Type: application/json" \
  --data-binary @shell-descriptor.json
```

Expected result:
- HTTP `403`.
- Why: write requires create rights formula (`add_digital_twin`) from token claims.

2. Create without token but with non-matching BPN header.

```bash
curl -i -X POST "$DTR_BASE/shell-descriptors" \
  -H "Content-Type: application/json" \
  -H "$HDR_BPN_UNKNOWN" \
  --data-binary @shell-descriptor.json
```

Expected result:
- HTTP `403`.
- Why: `Edc-Bpn` only influences data filtering rules, not missing create permission claims.

3. Create with admin token.

```bash
curl -i -X POST "$DTR_BASE/shell-descriptors" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @shell-descriptor.json
```

Expected result:
- HTTP `201`.
- Body contains created descriptor.
- Why: token provides the required create claim.

4. Read descriptor by id without token/header.

```bash
curl -s -X GET "$DTR_BASE/shell-descriptors/$AAS_ID_B64" | jq
```

Expected result:
- HTTP `200`.
- `specificAssetIds` is not present.
- `id` and `submodelDescriptors` are present.
- Why: route-level read (`bpn_or_public`) still allows access because the descriptor contains `PUBLIC_READABLE`, but fragment filtering for `specificAssetIds` uses `bpn_or_public_with_header`, which requires a non-`<nil>` `Edc-Bpn` header.

5. Read descriptor by id with non-matching BPN header (`BPNL00000000015G`).

```bash
curl -s -X GET "$DTR_BASE/shell-descriptors/$AAS_ID_B64" \
  -H "$HDR_BPN_UNKNOWN" | jq
```

Expected result:
- HTTP `200`.
- Visible `specificAssetIds`: only `manufacturerPartId` (`PUBLIC_READABLE`).
- Why: header is present but does not match tenant-specific keys, so only public fragment passes.

6. Read descriptor by id as tenant `BPN_COMPANY_001`.

```bash
curl -s -X GET "$DTR_BASE/shell-descriptors/$AAS_ID_B64" \
  -H "$HDR_BPN1" | jq
```

Expected result:
- HTTP `200`.
- Visible `specificAssetIds`: `customerPartId`, `manufacturerId` (with BPN1 key), and `manufacturerPartId`.
- Why: BPN1 matches tenant-protected entries plus `PUBLIC_READABLE`.

7. Read descriptor by id as tenant `BPN_COMPANY_002`.

```bash
curl -s -X GET "$DTR_BASE/shell-descriptors/$AAS_ID_B64" \
  -H "$HDR_BPN2" | jq
```

Expected result:
- HTTP `200`.
- Visible `specificAssetIds`: `manufacturerId` (with BPN2 key) and `manufacturerPartId`.
- Why: BPN2 can only see matching fragments and public fragments.

8. Read descriptor by id with admin token.

```bash
curl -s -X GET "$DTR_BASE/shell-descriptors/$AAS_ID_B64" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq
```

Expected result:
- HTTP `200`.
- Full descriptor is visible.
- Why: token-based read formula grants full read for this user in the integration-test-aligned setup.

9. Lookup by asset link with no token and no header (`customerPartId`).

```bash
curl -s -X POST "$DTR_BASE/lookup/shellsByAssetLink" \
  -H "Content-Type: application/json" \
  --data-binary @assetlink-num2.json | jq
```

Expected result:
- HTTP `200`.
- Empty result:

```json
{
  "paging_metadata": {}
}
```

- Why: query value is not `PUBLIC_READABLE` and no matching `Edc-Bpn` claim is present.

Public lookup still works without header when the queried asset link itself is `PUBLIC_READABLE` (`manufacturerPartId`):

```bash
curl -s -X POST "$DTR_BASE/lookup/shellsByAssetLink" \
  -H "Content-Type: application/json" \
  --data-binary @assetlink-num4.json | jq
```

Expected result:
- HTTP `200`.
- Shell id is returned.
- Why: the asset-link rule uses `bpn_or_public` (no header-required guard), so `PUBLIC_READABLE` link values remain discoverable.

10. Lookup by asset link as BPN1 (`customerPartId`).

```bash
curl -s -X POST "$DTR_BASE/lookup/shellsByAssetLink" \
  -H "Content-Type: application/json" \
  -H "$HDR_BPN1" \
  --data-binary @assetlink-num2.json | jq
```

Expected result:
- HTTP `200`.
- Shell id returned:

```json
{
  "paging_metadata": {},
  "result": [
    "e1eba3d7-91f0-4dac-a730-eaa1d35e035c-2"
  ]
}
```

- Why: `customerPartId` carries `externalSubjectId = BPN_COMPANY_001`, which matches header claim.

11. Lookup by asset link as BPN2 (`customerPartId`).

```bash
curl -s -X POST "$DTR_BASE/lookup/shellsByAssetLink" \
  -H "Content-Type: application/json" \
  -H "$HDR_BPN2" \
  --data-binary @assetlink-num2.json | jq
```

Expected result:
- HTTP `200`.
- Empty result.
- Why: `customerPartId` is protected for BPN1, not BPN2.

12. Direct shell lookup endpoint with and without token.

```bash
curl -i -X GET "$DTR_BASE/lookup/shells/$AAS_ID_B64"
```

Expected result:
- HTTP `403`.
- Why: this route requires token-based read access in the integration suite.

```bash
curl -i -X GET "$DTR_BASE/lookup/shells/$AAS_ID_B64" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "$HDR_BPN_UNKNOWN"
```

Expected result:
- HTTP `200`.
- Why: token grants route-level read permission.

## Precise PUT Behavior (`/shell-descriptors/{aasIdentifier}`)

Important: endpoint is plural `shell-descriptors`.

The DTR routes `PUT` authorization by descriptor existence:
- If descriptor does not exist: `PUT` is treated as create.
- If descriptor exists: `PUT` is treated as update.

Expected status matrix:

| Permission profile | Descriptor missing | Descriptor exists |
|---|---:|---:|
| create + update | `201 Created` | `204 No Content` |
| create only | `201 Created` | `403 Forbidden` |
| update only | `403 Forbidden` | `204 No Content` |
| neither | `403 Forbidden` | `403 Forbidden` |

Why this happens:
- For missing descriptor, backend uses insert path and applies create formula.
- For existing descriptor, backend uses replace path and applies update formula.

Example `PUT` call:

```bash
curl -i -X PUT "$DTR_BASE/shell-descriptors/$AAS_ID_B64" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @shell-descriptor.json
```

Expected result in normal sequence after creation:
- HTTP `204`.

## Production Security Hardening (`Edc-Bpn`)

To avoid header spoofing in real deployments:

1. Put the BPN value into the access token as claim `Edc-Bpn` (from trusted IdP/proxy mapping, not from client input).
2. Disable request-header-to-claim injection in DTR:

```bash
GENERAL_ENABLECUSTOMMIDDLEWAREHEADERINJECTION=false
```

3. Ensure your API gateway/ingress strips incoming `Edc-Bpn` from external client requests.

Why:
- ABAC formulas evaluate `CLAIM: "Edc-Bpn"` for tenant filtering.
- If untrusted callers can set `Edc-Bpn`, they can influence which fragments become visible.
- With token claim mapping plus disabled header injection, visibility is bound to signed token claims.

## Security Files Used By This Example

- Access rules: `examples/BaSyxDigitalTwinRegistryExample/security_env/access-rules.json`
- Trust list: `examples/BaSyxDigitalTwinRegistryExample/security_env/trustlist.json`

If you want different outcomes, change formulas and role claims there, then restart the stack.

