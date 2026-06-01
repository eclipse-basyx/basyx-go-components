# BaSyx Ory Hydra Example

This example runs a secured BaSyx AAS Environment with an Ory identity stack:

- [Ory Hydra](https://www.ory.sh/docs/hydra/) issues signed JWT access tokens.
- [Ory Kratos](https://www.ory.sh/docs/kratos/) stores the demo identities and
  passwords.
- The maintained
  [Kratos self-service UI](https://github.com/ory/kratos-selfservice-ui-node)
  renders the browser login form.
- BaSyx verifies the Hydra issuer, signature, expiry, audience, and configured
  claims before applying the Part 4 ABAC rules.

The included AASX file is imported automatically at startup.

## Prerequisites

- Docker + Docker Compose
- Free ports: `3000`, `3001`, `4433`, `4444`, `4455`, `8081`

## Start The Example

From this folder:

```bash
docker compose up -d
```

Open the BaSyx UI at [http://localhost:3000](http://localhost:3000).

## Demo Users

Use the OAuth2 login action in the BaSyx UI:

| User | Password | Access |
| --- | --- | --- |
| `admin@example.com` | `pwd` | Full read and write access |
| `viewer@example.com` | `pwd` | Read-only access |
| Not logged in | n/a | Nameplate and CarbonFootprint public data only |

Kratos self-registration is disabled. Roles are assigned by the bootstrap
container through the internal Kratos Admin API, not by browser users.

## Test Scenario

The AAS shell **IESEDriveMotorDM3000** is loaded from
[`aas/IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx) during startup.

1. Open the UI and inspect the public shell while logged out. Only its public
   product information is readable.
2. Log in as `viewer@example.com`. All submodels are readable, but write
   operations are denied.
3. Log out and log in as `admin@example.com`. Read and write operations are
   permitted.

## How Authorization Works

Hydra stores custom consent-session access token data below its `ext` claim.
The local consent bridge obtains the authenticated Kratos identity and gives
Hydra the configured identity role:

```json
{ "ext": { "role": "admin" } }
```

BaSyx maps that verified provider-specific claim to `basyx.role`:

```json
{
  "issuer": "http://hydra.localhost:4444",
  "audience": "basyx-api",
  "scopes": ["openid"],
  "claimMappings": [
    { "target": "role", "mode": "scalar", "sources": ["/ext/role"] }
  ]
}
```

The Part 4 ABAC model checks the canonical claim with the existing `$eq`
operator:

```json
{ "$eq": [{ "$attribute": { "CLAIM": "basyx.role" } }, { "$strVal": "admin" }] }
```

## Demo Adapters

Hydra intentionally delegates login and consent to external applications. The
maintained Kratos UI handles login but does not copy arbitrary Kratos traits
into access tokens. The small `consent-bridge` container performs that explicit
role mapping for this example.

The current BaSyx UI OAuth configuration exposes issuer, client ID, and scope,
but no extra authorization request parameters. Hydra requires a requested
audience for a resource-server token, so `hydra-proxy` adds
`audience=basyx-api` to authorization requests. Hydra still validates that the
audience is allowed for the bootstrapped `basyx-ui` client. That client also
registers `http://localhost:3000/aasviewer` as its UI callback and post-logout
return URL.

Both adapters are intentionally small teaching components. Production systems
should implement their own consent UX, audience model, secure secrets, TLS, and
operational persistence.

## Service Endpoints

- BaSyx UI: [http://localhost:3000](http://localhost:3000)
- AAS Environment: [http://localhost:8081](http://localhost:8081)
- Hydra discovery:
  [http://hydra.localhost:4444/.well-known/openid-configuration](http://hydra.localhost:4444/.well-known/openid-configuration)
- Kratos self-service UI: [http://localhost:4455](http://localhost:4455)

Hydra and Kratos Admin APIs are intentionally not published to the host.

## Stop / Clean Up

Stop containers:

```bash
docker compose down
```

Stop containers and delete the local Hydra, Kratos, and PostgreSQL data:

```bash
docker compose down -v
```
