# CatenaXample

CatenaXample is a Catena-X example for marker-based access control in the
Digital Twin Registry and Submodel Repository. The name combines "Catena-X"
and "example"; the scenarios model Catena-X-style partner visibility with
public markers and BPN-specific markers.

This example builds the current repository code and runs:

- a Digital Twin Registry on `http://localhost:5004/api/v3`;
- a Submodel Repository on `http://localhost:5005`;
- the BaSyx UI on `http://localhost:3000`;
- Keycloak on `http://keycloak.localhost:8080`;
- separate databases and configuration-service runs for DTR and Submodel Repository.

The services use marker values to control visibility:

- DTR asset identifiers: `specificAssetIds[].externalSubjectId.keys[].value`;
- DTR submodel descriptors: `submodelDescriptors[].supplementalSemanticIds[].keys[].value`;
- submodels: `supplementalSemanticIds[].keys[].value`;
- submodel elements: `supplementalSemanticIds[].keys[].value`.

Recognized example markers are `PUBLIC_READABLE`, `BPN_COMPANY_001`,
`BPN_COMPANY_002`, and `BPN_COMPANY_003`. The example descriptor grants its
restricted customer-part and restricted submodel data to BPN1 and BPN2. BPN3 is
included as a negative-access viewer.

The Keycloak user `admin` with password `pwd` has the
`data_provider` role. Authenticated requests with its token bypass marker
filtering and return all descriptors, submodels, elements, and supplemental
semantic ID marker rows.

## Start

Run from this directory:

```bash
docker compose up -d --build
```

The BaSyx UI uses the `basyx-ui` OAuth2 auth-code client from the imported
Keycloak realm and is configured through `basyx-infra.yml`.

## UI Setup

Open the UI at:

```text
http://localhost:3000
```

The UI authenticates against:

```text
http://keycloak.localhost:8080/realms/basyx
```

Use these example users. All passwords are `pwd`.

| Username | Token role | Edc-Bpn | Purpose |
| --- | --- | --- | --- |
| `admin` | `data_provider` | none | Data-provider user with full access. |
| `company1_viewer` | `edc_user` | `BPN_COMPANY_001` | BPN1 viewer, sees public and BPN1-restricted data. |
| `company2_viewer` | `edc_user` | `BPN_COMPANY_002` | BPN2 viewer, sees public and BPN2-restricted data. |
| `company3_viewer` | `edc_user` | `BPN_COMPANY_003` | BPN3 viewer, useful for negative-access checks. |
| `no_bpn_viewer` | `user` | none | Authenticated viewer without an EDC BPN claim. |
| `collection_viewer` | `user` | none | Generic collection user; collection scenarios set BPN headers explicitly. |

Keycloak admin console access is available at:

```text
http://keycloak.localhost:8080
```

with `admin` / `admin`.

## Collection Setup

Import `CatenaXample.postman_collection.json` into Postman to run the
example scenarios. Run the `Setup - Write Example Data` folder first; the
collection automatically fetches OAuth2 password-grant tokens from Keycloak for
protected requests and writes the shell descriptor plus both submodels. The
remaining folders show the expected DTR and Submodel Repository results for
data-provider, public descriptor, authenticated no-BPN, BPN1, BPN2, BPN3, and
unrelated-BPN access.

The collection variables use:

- `adminUsername = admin` for setup and data-provider requests;
- `regularUsername = collection_viewer` for non-provider token requests;
- `bpnCompany1`, `bpnCompany2`, `bpnCompany3`, and `unrelatedBpn` as test
  `Edc-Bpn` header values.

The generated Bruno collection is kept in sync with the Postman source:

- Bruno folder collection: `bruno_collection/`

Regenerate it after changing the Postman source:

```powershell
node ..\..\scripts\generate_bruno_collections.js
```

## Variant Data Script

`post_marker_variants.py` posts generated variants using the same structure as
the JSON files in `data/`. It creates unique shell and submodel IDs, cycles
marker values through `BPN_COMPANY_001` to `BPN_COMPANY_020` plus
`PUBLIC_READABLE`, and keeps each marker reference capped at two keys. Marker
lists may contain two BPN markers. `PUBLIC_READABLE` is selected like the BPN
markers, with about 20 percent of generated marker slots using it. The script
uses hash-based IDs seeded by a generated run id, so repeated runs do not
collide unless the same `--run-id` is passed intentionally. It fetches one admin
token, refreshes it before expiry during long runs, uses persistent per-worker
HTTP connections, generates compact JSON payloads directly, and posts with
bounded parallelism.

```powershell
python .\post_marker_variants.py --count 20000 --max-parallel 64
```

## DTR visibility

Anonymous DTR descriptor discovery returns the descriptor because it has a
public asset marker. Public submodel descriptors remain visible, while
restricted submodel descriptors and restricted `specificAssetIds` are removed.
Lookup and `assetIds` queries require an authenticated non-provider token and
then apply the same marker checks.

An unknown BPN sees public asset identifiers and the public submodel descriptor.

BPN1 sees public and BPN1 data. BPN2 marker values are removed from returned
marker arrays. BPN2 receives the equivalent BPN2 view.

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

Submodel Repository data reads require an authenticated non-provider token.
Tokens without an EDC BPN claim can read only the public submodel. BPN1 and
BPN2 can additionally read the restricted submodel.

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
