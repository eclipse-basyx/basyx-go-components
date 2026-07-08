# BaSyx DPP API Example

This example starts:

- Digital Product Passport API Service
- AAS Environment Service using the same database as the DPP API
- BaSyx Web UI connected to the AAS Environment
- BaSyx Configuration Service for database schema initialization
- Shared PostgreSQL database

## Prerequisites

- Docker + Docker Compose
- Free ports for the default stack: `3000`, `8080`, `8082`
- Free ports for the secured stack: `8080`, `8088`

Run either the default stack or the secured stack at one time unless you change ports and container names.

## Start The Example

From this folder:

```bash
docker compose up -d
```

Open the Swagger UI:

- [http://localhost:8080/swagger](http://localhost:8080/swagger)

Open the BaSyx UI:

- [http://localhost:3000](http://localhost:3000)

## Start The Secured DPP API Example

The unsecured example above remains the default. To run only the DPP API with route-based OIDC + ABAC security:

```bash
docker compose -f docker-compose.secured.yml up -d
```

Open the secured DPP Swagger UI:

- [http://localhost:8088/swagger](http://localhost:8088/swagger)

Open Keycloak:

- [http://keycloak.localhost:8080](http://keycloak.localhost:8080)

Useful test users from the shared BaSyx Keycloak realm:

- `usera` / `pwd`: `viewer`, read-only access to DPP routes
- `userx` / `pwd`: `editor`, create/read/update/delete access to DPP routes

Get a token:

```bash
TOKEN=$(curl -s \
  -X POST "http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=basyx-ui" \
  -d "grant_type=password" \
  -d "username=userx" \
  -d "password=pwd" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
```

Use the token against the secured DPP API:

```bash
curl -i \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data @sample-dpp.json \
  http://localhost:8088/v1/dpps
```

Security files:

- [`security_env/access-rules.json`](security_env/access-rules.json)
- [`security_env/trustlist.json`](security_env/trustlist.json)

The secured example protects DPP API routes only. It does not add DPP object-, field-, or query-filter authorization.

## Postman Collection

Import `BaSyx-DPP-API.postman_collection.json` into Postman to run the example scenarios:

- Create and read the demo DPP
- Resolve a DPP by product ID
- Read and update individual DPP elements
- Delete the demo DPP when you are done

No Postman environment file is currently committed. Create a local environment with these variables when you want to run the full collection:

- `baseUrl`: `http://localhost:8080` for the default stack or `http://localhost:8088` for the secured stack
- `bearerToken`: bearer token for secured requests
- `dppId`, `dppIdEncoded`: demo DPP ID and its double-URL-encoded form
- `productId`, `productIdEncoded`: demo product ID and its double-URL-encoded form
- `elementPath`: DPP element path such as `technicalData/manufacturerName`
- `representation`: `compressed` or `full`
- `historicalDate`, `currentTimestamp`: ISO-8601 timestamps used by history requests
- `limit`, `cursor`: pagination values

## Create A DPP

```bash
curl -i \
  -H "Content-Type: application/json" \
  --data @sample-dpp.json \
  http://localhost:8080/v1/dpps
```

## Read The DPP

The example DPP ID is `https://example.org/dpp/demo-product-001`.

```bash
curl http://localhost:8080/v1/dpps/https%253A%252F%252Fexample.org%252Fdpp%252Fdemo-product-001
```

Read the full representation:

```bash
curl "http://localhost:8080/v1/dpps/https%253A%252F%252Fexample.org%252Fdpp%252Fdemo-product-001?representation=full"
```

Read by product ID:

```bash
curl http://localhost:8080/v1/dppsByProductId/https%253A%252F%252Fexample.org%252Fproducts%252Fdemo-product-001
```

Read a single data element:

```bash
curl http://localhost:8080/v1/dpps/https%253A%252F%252Fexample.org%252Fdpp%252Fdemo-product-001/elements/technicalData/manufacturerName
```

Update a single data element:

```bash
curl -i \
  -X PATCH \
  -H "Content-Type: application/json" \
  --data '"B"' \
  http://localhost:8080/v1/dpps/https%253A%252F%252Fexample.org%252Fdpp%252Fdemo-product-001/elements/technicalData/energyClass
```

Read a historical DPP version:

```bash
curl "http://localhost:8080/v1/dppsByIdAndDate/https%253A%252F%252Fexample.org%252Fdpp%252Fdemo-product-001?date=2026-06-11T12:00:00Z&representation=compressed"
```

## Service Endpoints

- DPP API: [http://localhost:8080/v1/dpps](http://localhost:8080/v1/dpps)
- DPP Swagger UI: [http://localhost:8080/swagger](http://localhost:8080/swagger)
- DPP OpenAPI document: [http://localhost:8080/api-docs/openapi.yaml](http://localhost:8080/api-docs/openapi.yaml)
- DPP health endpoint: [http://localhost:8080/health](http://localhost:8080/health)
- BaSyx UI: [http://localhost:3000](http://localhost:3000)
- AAS Environment: [http://localhost:8082](http://localhost:8082)
- AAS Repository API: [http://localhost:8082/shells](http://localhost:8082/shells)
- Submodel Repository API: [http://localhost:8082/submodels](http://localhost:8082/submodels)
- AAS Registry API: [http://localhost:8082/shell-descriptors](http://localhost:8082/shell-descriptors)
- Submodel Registry API: [http://localhost:8082/submodel-descriptors](http://localhost:8082/submodel-descriptors)
- Discovery API: [http://localhost:8082/lookup/shells](http://localhost:8082/lookup/shells)

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

- This example is intentionally unsecured (`ABAC_ENABLED=false`).
- The DB schema is initialized by the BaSyx Configuration Service before the DPP API and AAS Environment start.
- The DPP API and AAS Environment use the same PostgreSQL database, so DPP-created AAS and Submodels are visible through the AAS Environment APIs and UI.
- The DPP API Service enables audit history internally, and the compose environment enables the same audit/history settings for both DPP API and AAS Environment.
- Path parameters containing URLs must be URL-escaped twice for the generated router.
