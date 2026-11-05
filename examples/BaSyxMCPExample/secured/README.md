# BaSyx MCP Example with Sign-In

This variant runs the AAS Environment with ABAC and a local Keycloak. MCP clients have to sign in
before they can use the tools, and every tool call runs with the signed-in user's permissions.

| User | Password | Permissions |
| --- | --- | --- |
| `alice` | `alice` | Read and change all shells, submodels and concept descriptions |
| `bob` | `bob` | Read only |

The credentials are for local testing only. Both ports are bound to `127.0.0.1`.

## Start

From this folder:

```bash
docker compose up -d
```

| Service | URL |
| --- | --- |
| MCP endpoint | `http://localhost:8082/mcp` |
| Keycloak | `https://localhost:8443` (admin console: `admin` / `admin`) |

## Trust the Keycloak Certificate

MCP clients only open HTTPS sign-in pages, so Keycloak serves HTTPS. On the first start, the
`keycloak-certificate` service creates a self-signed certificate for `localhost` in
`keycloak/tls/`. Trust it once so that the browser and the MCP client accept it. On macOS, trust
it for TLS only:

```bash
security add-trusted-cert -r trustRoot -p ssl -k ~/Library/Keychains/login.keychain-db keycloak/tls/keycloak.crt
```

Remove the trust again when you no longer need the example:

```bash
security remove-trusted-cert keycloak/tls/keycloak.crt
```

On other systems, add `keycloak/tls/keycloak.crt` to the system or browser trust store. If the
client still rejects the certificate, start it with
`NODE_EXTRA_CA_CERTS=$PWD/keycloak/tls/keycloak.crt`.

To create a new certificate, delete `keycloak/tls/` and start the example again.

## Connect Claude Code

```bash
claude mcp add --transport http basyx-secured http://localhost:8082/mcp
```

Start `claude`, run `/mcp`, select `basyx-secured` and choose *Authenticate*. Claude Code
registers itself with Keycloak and opens the browser. Sign in as `alice` or `bob` and grant access.

## Connect Codex

```bash
codex mcp add basyx-secured --url http://localhost:8082/mcp
```

```bash
codex mcp login basyx-secured
```

## Try It

Signed in as `alice`, ask for example:

- "List the shells in basyx-secured."
- "Add a property `MaintenanceInterval` with the value `P6M` to the TechnicalData submodel of the DM3000."

Signed in as `bob`, reading works but the change is denied with HTTP 403.

To switch users, clear the sign-in (in Claude Code via `/mcp`, in Codex with
`codex mcp logout basyx-secured`) and authenticate again.

## How Sign-In Works

1. The client calls the endpoint without a token and receives HTTP 401 with a link to the protected
   resource metadata at `http://localhost:8082/.well-known/oauth-protected-resource/mcp`.
2. The metadata names the Keycloak realm as authorization server.
3. The client registers itself through Keycloak's dynamic client registration. The realm only
   accepts loopback redirect URIs.
4. The user signs in; the client receives an access token with the audience
   `basyx-aas-environment`, which the [trustlist](security/trustlist.json) requires.

The AAS Environment fetches Keycloak's keys over the Docker network through the trustlist's
`discoveryUrl`, while browsers and clients use `https://localhost:8443`.

## Stop

```bash
docker compose down
```

Add `-v` to also delete the database volume.
