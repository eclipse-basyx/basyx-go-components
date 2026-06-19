# Marker-Based DTR and Submodel Access Example

This example builds the current repository code and runs:

- a Digital Twin Registry on `http://localhost:5004/api/v3`;
- a Submodel Repository on `http://localhost:5005`;
- Keycloak on `http://localhost:8080`;
- separate databases and configuration-service runs for DTR and Submodel Repository.

The services use marker values to control visibility:

- DTR asset identifiers: `specificAssetIds[].externalSubjectId.keys[].value`;
- DTR submodel descriptors: `submodelDescriptors[].supplementalSemanticIds[].keys[].value`;
- submodels: `supplementalSemanticIds[].keys[].value`;
- submodel elements: `supplementalSemanticIds[].keys[].value`.

Recognized example markers are `PUBLIC_READABLE`, `BPN_COMPANY_001`, and
`BPN_COMPANY_002`.

The Keycloak user `admin` with password `pwd` has the
`view_digital_twin` data-provider role. Authenticated requests with its token
bypass marker filtering and return all descriptors, submodels, elements, and
supplemental semantic ID marker rows.

## Start

Run from this directory:

```bash
docker compose up -d --build
```

Get the example administrator token:

```bash
export TOKEN=$(curl -s -X POST \
  http://localhost:8080/realms/basyx/protocol/openid-connect/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password" \
  -d "client_id=basyx-ui" \
  -d "username=admin" \
  -d "password=pwd" | jq -r .access_token)
```

Load the example data:

```bash
curl -i -X POST http://localhost:5004/api/v3/shell-descriptors \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @data/shell-descriptor.json

curl -i -X POST http://localhost:5005/submodels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @data/public-submodel.json

curl -i -X POST http://localhost:5005/submodels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @data/restricted-submodel.json
```

## DTR visibility

Anonymous discovery returns the descriptor because it has a public asset
marker. Public submodel descriptors remain visible, while restricted submodel
descriptors and all `specificAssetIds` are removed:

```bash
curl -s http://localhost:5004/api/v3/shell-descriptors | jq
```

An unknown BPN sees public asset identifiers and the public submodel descriptor:

```bash
curl -s http://localhost:5004/api/v3/shell-descriptors \
  -H "Edc-Bpn: BPN_COMPANY_003" | jq
```

BPN1 sees public and BPN1 data. BPN2 marker values are removed from returned
marker arrays:

```bash
curl -s http://localhost:5004/api/v3/shell-descriptors \
  -H "Edc-Bpn: BPN_COMPANY_001" | jq
```

BPN2 receives the equivalent BPN2 view:

```bash
curl -s http://localhost:5004/api/v3/shell-descriptors \
  -H "Edc-Bpn: BPN_COMPANY_002" | jq
```

The DTR policy extends the existing DTR rules with row-local `MATCH` filters
for:

```text
$aasdesc#submodelDescriptors[]
$aasdesc#submodelDescriptors[].supplementalSemanticIds[]
$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]
```

Filtering the supplemental-semantic-ID reference and its keys prevents an
allowed descriptor from leaking marker values belonging to another company.

## Submodel Repository visibility

Anonymous callers can read only the public submodel:

```bash
curl -s http://localhost:5005/submodels | jq
```

BPN1 and BPN2 can additionally read the restricted submodel:

```bash
curl -s http://localhost:5005/submodels \
  -H "Edc-Bpn: BPN_COMPANY_001" | jq
```

Within a visible submodel, whole submodel elements are filtered through the
`$sme` fragment. Their supplemental-semantic-ID references and key arrays are
also filtered:

```text
$sm#supplementalSemanticIds[]
$sm#supplementalSemanticIds[].keys[]
$sme
$sme#supplementalSemanticIds[]
$sme#supplementalSemanticIds[].keys[]
```

The same data can be used by the query endpoint:

```bash
curl -s -X POST http://localhost:5005/query/submodels \
  -H "Edc-Bpn: BPN_COMPANY_001" \
  -H "Content-Type: application/json" \
  -d '{
    "$condition": {
      "$eq": [
        {"$field": "$sme#supplementalSemanticIds[].keys[].value"},
        {"$strVal": "BPN_COMPANY_001"}
      ]
    },
    "$filters": [
      {
        "$fragment": "$sme#supplementalSemanticIds[]",
        "$condition": {
          "$eq": [
            {"$field": "$sme#supplementalSemanticIds[].keys[].value"},
            {"$strVal": "BPN_COMPANY_001"}
          ]
        }
      }
    ]
  }' | jq
```

## Security note

Header-to-claim injection is enabled solely to make local testing convenient.
Do not allow external clients to choose `Edc-Bpn` in production. Use a signed
token claim or trusted ingress mapping and set:

```yaml
GENERAL_ENABLECUSTOMMIDDLEWAREHEADERINJECTION: "false"
```

## Stop

```bash
docker compose down --volumes
```
