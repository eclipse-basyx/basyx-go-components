# Integrated MCP sandbox

This disposable example runs PostgreSQL, the schema migrator, Keycloak and the AAS Environment with MCP enabled. It seeds two motors, different caller permissions and two demonstration templates. Docker Compose must be available, and ports 18580 and 18584 must be free.

```sh
docker compose -p basyx-mcp-mvp -f examples/BaSyxMCPExample/docker-compose.yml up --build -d
```

Wait for Keycloak and the AAS Environment to finish starting. The MCP endpoint is `http://localhost:18584/mcp`; identity discovery uses `http://keycloak.localhost:18580/realms/basyx-mcp`. The host must resolve `keycloak.localhost` to loopback. Container requests use the Compose host-gateway mapping.

These credentials and HTTP settings are for this local sandbox only:

| User | Password | Visible data / rights |
| --- | --- | --- |
| alice | sandbox-alice | Both motors; read, create and delete |
| bob | sandbox-bob | Public motor only, read |
| carol | sandbox-carol | TechnicalData submodels through a QueryFilter, read |
| eve | sandbox-eve | No data permissions |

An authenticated request selects the `analyst` or `operator` profile with `?profile=analyst` or `?profile=operator`. Profiles choose tools; they do not grant data access. Without a profile, seven discovery, read and documentation tools are available. Both configured public OAuth clients require the `basyx-mcp` token audience.

## Exercise two clients

Run the Go SDK integration client from the repository root:

```sh
BASYX_MCP_IT_URL=http://localhost:18584/mcp go test -v ./internal/mcp/integration_tests
```

Run all seven MVP scenarios using the official MCP Inspector CLI (Node.js, npm and Python 3.9+ required):

```sh
python3 examples/BaSyxMCPExample/smoke_inspector.py
```

The runner pins Inspector 2.12.0 through `npx`; alternatively set `MCP_INSPECTOR_BIN` to an existing Inspector executable. It exercises discovery/temperature, descriptions and prompts, AASQL validation/query, fixed aggregation, restricted-user parity, template-backed creation/verification/rollback and embedded documentation. Creation uses fresh identifiers and removes its resources afterward. The automated browser interaction is restricted to the fixed local test realm and simulates the human approval step for testing.

To review approval manually, connect an MCP client to the operator profile, use `draft.json` with `validate_aas_draft` and `plan_create_aas`, and open the returned `confirmation_url`. Sign in as Alice, review the proposal and submit the approval form. Copy the resulting single-use token to `apply_plan`. `get_changeset` returns a separate approval link for rollback. Clients cannot approve by setting a boolean argument.

The Go client additionally checks approval replay, forbidden callers, compensation after a later create fails, and rollback refusal after a resource changes. The example's expected rated-power average is 18 kW for Alice and Carol and 12 kW for Bob.

## Templates and operation

`templates/` contains explicitly labelled demonstration structures for the L2 validator. They are not qualified IDTA templates or released `submodel_info` artifacts. Before production use, supply qualified, versioned structures and cheat sheets, configure their allowlist, use HTTPS and configure a production identity provider. See [MCP configuration and recovery](../../docu/mcp/README.md) and [the implementation security review](../../docu/mcp/security-review.md).

Plans, approval consumption, receipts and audit events persist in PostgreSQL across service restarts. Browser login state is temporary; repeat login after restarting the AAS Environment. An interrupted saga may need the original user's explicit rollback after its execution lease expires. The service reports cleanup conflicts instead of deleting changed resources.

To remove this example and its database, run:

```sh
docker compose -p basyx-mcp-mvp -f examples/BaSyxMCPExample/docker-compose.yml down -v
```

This deletes all data belonging to this sandbox project.
