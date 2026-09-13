# BaSyx Delegated Operations Example

This Docker Compose example demonstrates synchronous and asynchronous delegated
operations in full-metadata and value-only representations with:

- AAS Environment at `http://localhost:8090`
- AAS Web UI at `http://localhost:3000`
- delegated operation service at `http://localhost:8099`

The example loads an AAS and a Submodel containing these operations:

- `AddNumbersSync`
- `AddNumbersAsync`
- `AddNumbersSyncValueOnly`
- `AddNumbersAsyncValueOnly`

All four add the input values `numberA` and `numberB` and return `sum`.
The `Sync` operations delegate to an endpoint that responds immediately, while
the `Async` operations delegate to one that simulates delayed work. The names
demonstrate invocation and representation choices; they do not restrict the
supported modes. Every operation can be invoked synchronously or asynchronously
in either representation through the AAS Environment.

## Start

```bash
docker compose up --build
```

## Delegation trust

The delegated service has the static address `172.28.0.10`, and the example
allowlists both its service name and IP address. If `172.28.0.0/24` conflicts
with another Docker network, update the subnet, service address, and
`SMREPO_DELEGATION_TRUSTED_HOSTS` together.

## Invoke the operations

Open `http://localhost:3000` and select `DelegatedOperationsAAS`, then
`DelegatedOperationsSubmodel`. Select either operation, enter values for
`numberA` and `numberB`, and execute it. The operation view lets you choose
between synchronous and asynchronous invocation. For example, entering `5` and
`3` displays `8` as `sum`.

## Invoke through the API

The requests in `data/invoke-request-add-5-and-3.json` and
`data/invoke-request-add-5-and-3-value-only.json` contain the same example input
in full-metadata and value-only form. Matching expected responses are available
as `data/expected-result-add-5-and-3.json` and
`data/expected-result-add-5-and-3-value-only.json`.

### Synchronous invocation

```bash
curl --fail-with-body --silent --show-error \
  --request POST \
  'http://localhost:8090/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersSync/invoke' \
  --header 'Content-Type: application/json' \
  --data @data/invoke-request-add-5-and-3.json |
  jq
```

The response is an `OperationResult` with `executionState` `Completed`,
`success` `true`, and an output variable `sum` with value `8`.

### Synchronous value-only invocation

```bash
curl --fail-with-body --silent --show-error \
  --request POST \
  'http://localhost:8090/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersSyncValueOnly/invoke/$value' \
  --header 'Content-Type: application/json' \
  --data @data/invoke-request-add-5-and-3-value-only.json |
  jq
```

The value-only result contains `"outputArguments": {"sum": 8}`.
Integer and decimal values use JSON numbers, and boolean values use JSON booleans,
in accordance with the [IDTA v3.2 normative value-only mappings](https://industrialdigitaltwin.io/aas-specifications/IDTA-01001/v3.2/mappings/mappings.html).
Full-metadata operation arguments and results retain their XSD lexical strings.

### Asynchronous invocation

AAS V3.2 uses `Location` headers for asynchronous operation resources. Keep
automatic redirect following disabled so the `202` and `302` responses remain
visible.

Start the invocation:

```bash
curl --include --request POST \
  'http://localhost:8090/submodels/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM/submodel-elements/AddNumbersAsync/invoke-async' \
  --header 'Content-Type: application/json' \
  --data @data/invoke-request-add-5-and-3.json
```

The response is `202 Accepted`. Copy its `Location` header and request that URL:

```bash
curl --include '<status Location>'
```

- `200 OK` with `executionState` `Initiated` or `Running`: repeat the status
  request.
- `302 Found`: copy the new `Location` header, which identifies the result.

Fetch the result:

```bash
curl --fail-with-body --silent --show-error '<result Location>' | jq
```

The completed result contains `sum` with value `8`.

For value-only asynchronous invocation, post the value-only fixture to
`AddNumbersAsyncValueOnly/invoke-async/$value` and poll the returned status
location exactly as above. The status endpoint redirects to the canonical result
URL; append `/$value` when fetching that result in value-only form:

```bash
curl --fail-with-body --silent --show-error '<result Location>/$value' | jq
```

The same async handle remains available through both the canonical result URL
and its `/$value` form.
