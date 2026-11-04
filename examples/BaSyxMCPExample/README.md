# Integrated MCP sandbox

This local pilot runs PostgreSQL, the schema migrator, Keycloak and the AAS Environment with MCP enabled. It initializes two motors, different caller permissions and two demonstration templates once. Data and identity-provider state live in named volumes. Docker Compose and Python 3.11+ are required.

From the repository root:

```sh
python3 examples/BaSyxMCPExample/pilot.py start
python3 examples/BaSyxMCPExample/pilot.py status
python3 examples/BaSyxMCPExample/pilot.py connect
```

`start` builds the services, waits for initialization and verifies authenticated MCP discovery. `status` prints the endpoint, available tools and any sign-in/availability error. `connect` adds a named Codex connection without replacing other settings, backs up an existing configuration, and launches the Codex OAuth login. Sign in as Alice using the credentials below. The registered public client uses PKCE; tokens are managed by Codex, not copied into the example or its configuration. The requested `offline_access` scope allows a refresh token if you grant it.

Enable/refresh the connection in your client or reopen the chat after connecting. A healthy server and completed sign-in do **not** verify attachment to a particular chat. Ask “What is the temperature of motor M-4711?” and check that the answer cites the actual tool result. The fresh fixture is 48 °C; this is example repository data, not a live sensor. `get_asset_measurement` reports retrieval time separately from the unknown sensor observation time.

The default endpoint is `http://localhost:18584/mcp`, with identity discovery at `http://keycloak.localhost:18580/realms/basyx-mcp`. The host must resolve `keycloak.localhost` to loopback. To keep an older sandbox intact or use different ports:

```sh
python3 examples/BaSyxMCPExample/pilot.py start --project my-mcp-pilot --api-port 18594 --oidc-port 18590
python3 examples/BaSyxMCPExample/pilot.py connect --project my-mcp-pilot --name my-basyx
```

Subsequent commands remember that project's ports. Startup refuses ports owned by another stack and refuses to replace a legacy anonymous database volume. For legacy data, choose a new project and explicitly export/import the old environment. Never run `down -v` on a project whose data you want to retain.

The pilot starts with MCP writes **disabled**. To exercise the existing human-approved creation and rollback flow in a disposable sandbox, run `start --enable-writes`. This choice is retained for that project; use `start --disable-writes` to turn MCP writes off again. These demo templates are not qualified IDTA production templates.

These credentials and HTTP settings are for this local sandbox only:

| User | Password | Visible data / rights |
| --- | --- | --- |
| alice | sandbox-alice | Both motors; read, create and delete |
| bob | sandbox-bob | Public motor only, read |
| carol | sandbox-carol | TechnicalData submodels through a QueryFilter, read |
| eve | sandbox-eve | No data permissions |

An authenticated request selects the `analyst` or `operator` profile with `?profile=analyst` or `?profile=operator`. Profiles choose tools; they do not grant data access. Without a profile, nine discovery, read and documentation tools are available; `analyst` exposes twelve. The configured public OAuth clients require the `basyx-mcp` token audience.

## Exercise two clients

Enable writes in a disposable sandbox before running the full acceptance scenarios. Run the Go SDK integration client from the repository root:

```sh
BASYX_MCP_IT_URL=http://localhost:18584/mcp go test -v ./internal/mcp/integration_tests
```

Run all seven MVP scenarios using the official MCP Inspector CLI (Node.js, npm and Python 3.11+ required):

```sh
python3 examples/BaSyxMCPExample/smoke_inspector.py
```

The runner pins Inspector 2.12.0 through `npx`; alternatively set `MCP_INSPECTOR_BIN` to an existing Inspector executable. It exercises discovery/temperature, descriptions and prompts, AASQL validation/query, fixed aggregation, restricted-user parity, template-backed creation/verification/rollback and embedded documentation. Creation uses fresh identifiers and removes its resources afterward. The automated browser interaction is restricted to the fixed local test realm and simulates the human approval step for testing.

To review approval manually, connect an MCP client to the operator profile, use `draft.json` with `validate_aas_draft` and `plan_create_aas`, and open the returned `confirmation_url`. Sign in as Alice, review the proposal and submit the approval form. Copy the resulting single-use token to `apply_plan`. `get_changeset` returns a separate approval link for rollback. Clients cannot approve by setting a boolean argument.

The Go client additionally checks approval replay, forbidden callers, compensation after a later create fails, and rollback refusal after a resource changes. The example's expected rated-power average is 18 kW for Alice and Carol and 12 kW for Bob.

## Templates and operation

`templates/` contains explicitly labelled demonstration structures for the L2 validator. They are not qualified IDTA templates or released `submodel_info` artifacts. Before production use, supply qualified, versioned structures and cheat sheets, configure their allowlist, use HTTPS and configure a production identity provider. See [MCP configuration and recovery](../../docu/mcp/README.md) and [the implementation security review](../../docu/mcp/security-review.md).

Plans, approval consumption, receipts and audit events persist in PostgreSQL across service restarts. The registered Codex client and identity-provider state also persist across container recreation. An AAS Environment restart can still invalidate an in-progress browser confirmation; reopen its confirmation link and sign in again. An interrupted saga may need the original user's explicit rollback after its execution lease expires. The service reports cleanup conflicts instead of deleting changed resources.

To stop while preserving data:

```sh
python3 examples/BaSyxMCPExample/pilot.py stop
```

For the large-asset and persistence acceptance test, use a **disposable** project. The test creates temporary resources and a temporary OAuth client, recreates only that project's service containers, verifies the edited data and client survived, then removes its fixtures:

```sh
python3 examples/BaSyxMCPExample/pilot.py start --project basyx-mcp-pilot-test-local --api-port 18594 --oidc-port 18590 --enable-writes
BASYX_MCP_IT_URL=http://localhost:18594/mcp BASYX_MCP_IT_ISSUER_URL=http://localhost:18590/realms/basyx-mcp go test -v ./internal/mcp/integration_tests
BASYX_MCP_IT_BASE_URL=http://localhost:18594 BASYX_MCP_IT_ISSUER_URL=http://localhost:18590/realms/basyx-mcp python3 examples/BaSyxMCPExample/smoke_inspector.py
python3 examples/BaSyxMCPExample/smoke_pilot.py --project basyx-mcp-pilot-test-local
```

Run clients sequentially so temporary write scenarios cannot affect another client's aggregate expectations. `.github/workflows/mcp-pilot.yml` runs these checks on every relevant pull request.

To deliberately delete all data for the default pilot:

```sh
docker compose -p basyx-mcp-pilot -f examples/BaSyxMCPExample/docker-compose.yml down -v
```
