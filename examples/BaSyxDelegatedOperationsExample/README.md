# BaSyx Delegated Operations Example (Sync + Async)

This example provides a complete docker-compose setup for delegated operation invocation using the Go AAS Environment.

It includes:
- AAS Environment (`http://localhost:8091`)
- Delegated microservice (`http://localhost:8099`) implemented in Go
- AAS Web UI (`http://localhost:3000`)
- PostgreSQL
- Data init container that preloads AAS and Submodel data

## Start

Run from this folder:

```bash
docker compose up --build
```

## Delegation Trust

To protect against DNS rebinding and server-side request forgery, the Submodel
Repository validates both the configured delegation hostname and the IP address
to which it resolves. This example therefore assigns
`delegated-operation-service` the stable address `172.28.0.10` and allowlists
both `delegated-operation-service:8080` and `172.28.0.10:8080`.

If the `172.28.0.0/24` subnet conflicts with an existing Docker network, change
the configured subnet, the delegated service address, and the corresponding IP
entry in `SMREPO_DELEGATION_TRUSTED_HOSTS` together.

## Preloaded IDs

- Submodel ID: `https://example.com/ids/sm/delegated-operations`
- Encoded Submodel ID: `aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM`
- AAS ID: `https://example.com/ids/aas/delegated-operations-example`
- Encoded AAS ID: `aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYWFzL2RlbGVnYXRlZC1vcGVyYXRpb25zLWV4YW1wbGU`

## Delegated Operations

The submodel contains two operation elements:
- `AddNumbersSync` delegates to `http://delegated-operation-service:8080/delegate/add/sync`
- `AddNumbersAsync` delegates to `http://delegated-operation-service:8080/delegate/add/async`

Operation elements JSON for manual loading is provided in:

- `data/operation-submodel-elements.json`

## Sync Invocation

```bash
curl --location --request POST \
  'http://localhost:8091/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersSync/invoke' \
  --header 'Content-Type: application/json' \
  --data @data/invoke-request-add-5-and-3.json
```

Expected result contains `sum = 8` in `outputArguments`.

## Async Invocation

1) Start async invocation:

```bash
curl --location --request POST \
  'http://localhost:8091/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersAsync/invoke-async' \
  --header 'Content-Type: application/json' \
  --data @data/invoke-request-add-5-and-3.json
```

Response contains a `handleId`.

2) Poll status:

```bash
curl --location --request GET \
  'http://localhost:8091/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersAsync/operation-status/<handleId>'
```

- `200` with `executionState=Running` means still running.
- `302` means result is available.

3) Fetch result:

```bash
curl --location --request GET \
  'http://localhost:8091/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersAsync/operation-results/<handleId>'
```

Expected result contains `sum = 8` in `outputArguments`.

## AAS Web UI

Open:

- `http://localhost:3000`

The UI is configured to use:

- the `mono-all` infrastructure template
- AAS Environment `http://localhost:8091`

The infrastructure is preconfigured in `basyx-infra.yml` and mounted read-only
into the UI container.

## Files

- `docker-compose.yml` - complete stack
- `basyx-infra.yml` - AAS Web UI infrastructure configuration
- `startup.sh` - waits for services and preloads AAS/Submodel data
- `data/aas-shell.json` - preloaded AAS payload
- `data/submodel-delegated-operations.json` - preloaded submodel with delegated operations
- `data/operation-submodel-elements.json` - operation elements only JSON
- `delegated-operation-service/main.go` - delegated sync/async add service
- `delegated-operation-service/Dockerfile` - delegated service container image
