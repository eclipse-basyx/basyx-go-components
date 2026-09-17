# Try the BaSyx DPP API

Create, read and update a sample Digital Product Passport (DPP), then inspect its AAS and Submodels in the BaSyx Web UI. The example runs the DPP API and AAS Environment against the same database.

## Prerequisites

- Docker with Docker Compose
- `curl` for the command-line examples, or Postman
- Available ports `3000`, `8080` and `8082`

Run the commands below in the same terminal, starting from the repository root.

## Start the example

```bash
cd examples/BaSyxDPPAPIExample
docker compose up -d
```

The first start downloads the container images and initializes the database. Check startup progress with `docker compose ps`. If a service does not start, use `docker compose logs dpp-api aas-environment basyx_configuration`.

| Open | Use it to |
| --- | --- |
| [DPP Swagger UI](http://localhost:8080/swagger) | Explore and call the DPP endpoints |
| [BaSyx Web UI](http://localhost:3000) | Inspect the AAS and Submodels created by your requests |
| [DPP health endpoint](http://localhost:8080/health) | Check whether the DPP service is ready |

This default example does not require authentication. To try access control, use the [secured example](#try-the-secured-example) below.

Set the API URL and the percent-encoded identifiers used by the sample:

```bash
BASE_URL='http://localhost:8080'
DPP_ID='https%3A%2F%2Fwww.example.org%2Fbatterypassport%2F1234545'
PRODUCT_ID='https%3A%2F%2Fwww.example.org%2F1234545'
```

For ID-based DPP requests, use the same value for the owning AAS identifier and
`DppMetadata.digitalProductPassportId`. This example uses the following
matching values:

| Value | Example |
| --- | --- |
| AAS identifier | `https://www.example.org/batterypassport/1234545` |
| `DppMetadata.digitalProductPassportId` | `https://www.example.org/batterypassport/1234545` |
| `DPP_ID` path value | `https%3A%2F%2Fwww.example.org%2Fbatterypassport%2F1234545` |

If the two stored values differ, an ID-based request returns `404 Not Found`.
This is a BaSyx API limitation; the DPP specification permits the identifiers
to differ. `PRODUCT_ID` is the separate `uniqueProductIdentifier` used by
product lookup. Product lookup can still return a passport when its AAS and
metadata identifiers differ, provided the product ID matches.

## Create a passport

The [sample DPP](sample-dpp.json) describes a battery pack with nameplate, carbon footprint, documentation and circularity data.

```bash
curl -i \
  -H 'Content-Type: application/json' \
  --data @sample-dpp.json \
  "$BASE_URL/v1/dpps"
```

Expect `201 Created` and the DPP ID `https://www.example.org/batterypassport/1234545`. Creating the same passport again returns `409 Conflict`; use an update request to change it.

Refresh the BaSyx Web UI to see the corresponding AAS and Submodels.

## Read the passport

Read the default, compressed representation:

```bash
curl "$BASE_URL/v1/dpps/$DPP_ID"
```

Read the full representation, which includes data element types and metadata:

```bash
curl "$BASE_URL/v1/dpps/$DPP_ID?representation=full"
```

Find the same passport using its product ID:

```bash
curl "$BASE_URL/v1/dppsByProductId/$PRODUCT_ID"
```

### Read one element

Element paths identify the content section and the element within it. For the sample manufacturer name, the path is:

```text
$['https://admin-shell-io/idta/digitalproductpassport/Nameplate/1']['ManufacturerName']
```

URL path parameters must be percent-encoded once. The encoded manufacturer path is provided here for copying:

```bash
MANUFACTURER_PATH='%24%5B%27https%3A%2F%2Fadmin-shell-io%2Fidta%2Fdigitalproductpassport%2FNameplate%2F1%27%5D%5B%27ManufacturerName%27%5D'
curl "$BASE_URL/v1/dpps/$DPP_ID/elements/$MANUFACTURER_PATH"
```

Expect the JSON string `"VoltFabrik GmbH"`.

## Update the passport and read its history

Save a timestamp before making changes. The one-second wait ensures this timestamp is after creation, even when you run the commands in quick succession:

```bash
sleep 1
HISTORY_DATE=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
```

Change the manufacturer name:

```bash
curl -i \
  -X PATCH \
  -H 'Content-Type: application/json' \
  --data '"VoltFabrik GmbH - Updated"' \
  "$BASE_URL/v1/dpps/$DPP_ID/elements/$MANUFACTURER_PATH"
```

Expect `200 OK` with the updated value. Read the passport again to see the change, or refresh its Submodel in the BaSyx Web UI.

You can also patch passport metadata:

```bash
curl -i \
  -X PATCH \
  -H 'Content-Type: application/merge-patch+json' \
  --data '{"dppStatus":"archived"}' \
  "$BASE_URL/v1/dpps/$DPP_ID"
```

This changes the status and `lastUpdate` without changing the content sections.

Read the passport as it was before these updates:

```bash
curl "$BASE_URL/v1/dppsByIdAndDate/$DPP_ID?date=$HISTORY_DATE&representation=compressed"
```

The historical response contains the original manufacturer name and status. Use a timestamp from your own session; a date before the passport existed returns `404 Not Found`.

History is enabled for both APIs in this example. Changes made through the AAS API also appear in historical DPP reads, although they do not refresh the DPP's `lastUpdate` field.

## Choose which content appears

`contentSpecificationIds` lists the semantic IDs of the Submodels that contribute content to the passport. To show only the sample nameplate:

```bash
curl -i \
  -X PATCH \
  -H 'Content-Type: application/merge-patch+json' \
  --data '{"contentSpecificationIds":["https://admin-shell-io/idta/digitalproductpassport/Nameplate/1"]}' \
  "$BASE_URL/v1/dpps/$DPP_ID"
```

Read the passport again: only the nameplate content section is included. The other Submodels remain available in the AAS Environment and Web UI. Add their semantic IDs back to the list to include them again.

- An empty or missing `contentSpecificationIds` list returns passport metadata with no content sections.
- Unselected sections cannot be read through the DPP element endpoints.
- Historical reads use the selection that applied at the requested time.

When preparing your own DPP JSON, use the listed semantic IDs as the top-level content keys, as shown in [sample-dpp.json](sample-dpp.json).

## Delete the passport

```bash
curl -i -X DELETE "$BASE_URL/v1/dpps/$DPP_ID"
```

Expect `204 No Content`. Reading the current passport now returns `404 Not Found`. Its historical versions remain readable in this example.

Deleting the passport removes its AAS, DppMetadata and their descriptors. The
content Submodels, their descriptors and managed attachments remain available
in the AAS Environment. Removing a content section with `null` also retains
that Submodel and its attachments; changing `contentSpecificationIds` only
changes what the passport shows.

To repeat the walkthrough with the same sample IDs, use the [cleanup commands](#stop-and-clean-up) to remove all sample data, then start the example again.

## Try file references

The sample contains PDF links for manuals and reports. These are placeholder URLs; the example does not host those files.

For your own data, provide an HTTP(S) URL in a file's `url` field. The DPP API does not upload files. You can upload managed files through the AAS Environment attachment endpoints instead; see the [AAS API guide](../../docu/user/aas_api_v3_2.md).

Historical responses preserve file references. If a file is replaced behind the same URL, downloading it from an old passport may return the new bytes. Use separate URLs for different file versions when you need to retrieve the original files.

## Use Postman instead of curl

Import [BaSyx-DPP-API.postman_collection.json](BaSyx-DPP-API.postman_collection.json). It includes create, read, update, history, search and delete requests, with the required IDs and element paths already configured.

No separate Postman environment is needed. The main collection variables to adjust are:

| Variable | Default / purpose |
| --- | --- |
| `baseUrl` | `http://localhost:8080`; use `http://localhost:8088` for the secured example |
| `bearerToken` | Access token for secured requests |
| `dppId`, `productId` and their `Encoded` variants | Identifiers for the passport you want to use |
| `representation` | `compressed` or `full` |
| `historicalDate` | Timestamp for a historical read |
| `limit`, `cursor` | Pagination for search requests |

The collection also supplies encoded element paths. If you use your own passport, update these to match its content.

## Try the secured example

This alternative runs the DPP API with Keycloak authentication and role-based permissions. It uses ports `8080` and `8088` and does not include the AAS Environment or Web UI. Stop the default example first because both stacks use port `8080`:

```bash
docker compose down
docker compose -f docker-compose.secured.yml up -d
```

Open the [secured Swagger UI](http://localhost:8088/swagger) or [Keycloak](http://keycloak.localhost:8080). If `keycloak.localhost` does not resolve, add `127.0.0.1 keycloak.localhost` to your hosts file.

| Test user | Password | DPP permissions |
| --- | --- | --- |
| `usera` | `pwd` | Read only |
| `userx` | `pwd` | Create, read, update and delete |

The token command below requires `jq`:

```bash
TOKEN=$(curl -fsS \
  -X POST 'http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'client_id=basyx-ui' \
  -d 'grant_type=password' \
  -d 'username=userx' \
  -d 'password=pwd' | jq -r '.access_token')

curl -i \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  --data @sample-dpp.json \
  http://localhost:8088/v1/dpps
```

To use the other requests above, change `BASE_URL` to `http://localhost:8088` and add `-H "Authorization: Bearer $TOKEN"` to each request. Alternatively, set `baseUrl` and `bearerToken` in Postman. Obtain a new token when the current one expires.

Try the same write request with a token for `usera`: it should be denied. This example grants permissions per route, so it does not demonstrate different permissions for individual passports or fields. You can inspect the [access rules](security_env/access-rules.json) and [trusted issuer configuration](security_env/trustlist.json), or read the [security guide](../../docu/security/README.md).

## Stop and clean up

Stop the default stack without removing its containers:

```bash
docker compose stop
```

Resume it with `docker compose start`. To remove the containers and delete their database volumes, including all sample data:

```bash
docker compose down -v
```

For the secured stack, use:

```bash
docker compose -f docker-compose.secured.yml down -v
```
