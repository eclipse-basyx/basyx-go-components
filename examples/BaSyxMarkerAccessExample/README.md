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

## Postman Collection

Import `BaSyx-Marker-Access.postman_collection.json` into Postman to run the
example scenarios. Run the `Setup - Write Example Data` folder first; the
collection automatically fetches OAuth2 password-grant tokens from Keycloak for
protected requests and writes the shell descriptor plus both submodels. The
remaining folders show the expected DTR and Submodel Repository results for
admin, anonymous, regular-user, BPN1, BPN2, and unrelated BPN access.

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

Anonymous discovery returns the descriptor because it has a public asset
marker. Public submodel descriptors remain visible, while restricted submodel
descriptors and all `specificAssetIds` are removed.

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

Anonymous callers can read only the public submodel. BPN1 and BPN2 can
additionally read the restricted submodel.

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
