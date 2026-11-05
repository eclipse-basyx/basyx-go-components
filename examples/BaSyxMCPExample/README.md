# BaSyx MCP Example

This example runs an AAS Environment with its [MCP endpoint](../../docu/mcp/README.md) enabled, so that
MCP clients such as Claude Code or Codex can read and change the Asset Administration Shells. It
preloads the shell **IESEDriveMotorDM3000** from [`aas/IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx).

The example has no authentication and allows writes. Use it only on your own machine; the port is
bound to `127.0.0.1`. The [secured variant](secured/README.md) adds Keycloak sign-in and per-user
permissions.

## Start

From this folder:

```bash
docker compose up -d
```

The MCP endpoint is `http://localhost:8081/mcp`. The REST API is available on the same port.

## Connect Claude Code

```bash
claude mcp add --transport http basyx http://localhost:8081/mcp
```

Start `claude` and run `/mcp` to see the server and its tools.

## Connect Codex

```bash
codex mcp add basyx --url http://localhost:8081/mcp
```

Run `codex mcp list` to check the connection.

## Try It

Ask the agent, for example:

- "Which asset administration shells are in BaSyx?"
- "What is the rated power of the drive motor DM3000? Use its TechnicalData submodel."
- "Add a property `MaintenanceInterval` with the value `P6M` to the TechnicalData submodel of the DM3000."

The tools carry read-only, additive or destructive hints. Your MCP client uses them and its own
permission settings to decide when to ask before a tool runs.

## Stop

```bash
docker compose down
```

Add `-v` to also delete the database volume.
