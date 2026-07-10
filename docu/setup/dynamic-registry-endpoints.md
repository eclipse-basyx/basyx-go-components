# Dynamic Registry Descriptor Endpoint URLs

This document describes how `aasenvironmentservice` can generate AAS Registry and Submodel Registry descriptor endpoint URLs when `general.externalUrl` is not configured.

The feature is only relevant for `aasenvironmentservice` with repository-to-registry synchronization enabled.

## Problem

Registry descriptors contain absolute endpoint URLs, for example:

```text
https://example.com/api/v3/shells/<encoded-aas-id>
https://example.com/api/v3/submodels/<encoded-submodel-id>
```

Previously, these URLs required a static `general.externalUrl`. That is still the preferred setup when the public backend URL is known.

Some deployments cannot know the final public URL at service startup. Common examples are generic Docker Compose setups, ingress-controlled deployments, and reverse-proxy setups where the external host is assigned outside the container configuration. In those cases, the service can derive the external base URL from a trusted HTTP request.

## Configuration Summary

| YAML key | Environment variable | Purpose |
| --- | --- | --- |
| `general.externalUrl` | `GENERAL_EXTERNALURL` | Static public base URL. Preferred for stable deployments. Supports comma-separated URLs. |
| `general.trustedDynamicHosts` | `GENERAL_TRUSTEDDYNAMICHOSTS` | Direct or forwarded host allowlist for request-derived endpoint URLs. Use host:port when the request includes a port. |
| `general.trustProxyHeaders` | `GENERAL_TRUSTPROXYHEADERS` | Enables `Forwarded` and `X-Forwarded-*` handling, but only for trusted proxy source IPs. |
| `general.trustedProxyCIDRs` | `GENERAL_TRUSTEDPROXYCIDRS` | CIDR allowlist for proxy source addresses that may provide forwarded headers. |
| `general.dynamicRegistryReconciliationTimeoutSeconds` | `GENERAL_DYNAMIC_REGISTRY_RECONCILIATION_TIMEOUT_SECONDS` | Maximum runtime for one asynchronous dynamic registry reconciliation run. Default: `30`. |
| `general.aasRegistryIntegration` | `GENERAL_AASREGISTRYINTEGRATION` | Enables AAS repository to AAS registry synchronization. |
| `general.submodelRegistryIntegration` | `GENERAL_SUBMODELREGISTRYINTEGRATION` | Enables Submodel repository to Submodel registry synchronization. |

Readable `BASYX_` aliases are also supported for the timeout override:

```env
BASYX_GENERAL_DYNAMIC_REGISTRY_RECONCILIATION_TIMEOUT_SECONDS=120
```

## Static Mode

Use static mode when the public backend URL is known at startup.

```yaml
general:
  aasRegistryIntegration: true
  submodelRegistryIntegration: true
  externalUrl: https://example.com/api/v3
```

Environment equivalent:

```env
GENERAL_AASREGISTRYINTEGRATION=true
GENERAL_SUBMODELREGISTRYINTEGRATION=true
GENERAL_EXTERNALURL=https://example.com/api/v3
```

Behavior:

- Descriptor endpoint URLs are generated from `general.externalUrl`.
- Request-derived dynamic URLs are ignored.
- Multiple static URLs can be configured as a comma-separated list.
- Registry synchronization can run immediately during startup preconfiguration and normal repository mutations.

Use this mode for production whenever the external URL is stable.

## Dynamic Direct-Host Mode

Use direct-host mode when requests reach `aasenvironmentservice` directly and the service should derive the public base URL from the request `Host` header.

```yaml
general:
  aasRegistryIntegration: true
  submodelRegistryIntegration: true
  externalUrl: ""
  trustedDynamicHosts:
    - localhost:8082
```

Environment equivalent:

```env
GENERAL_AASREGISTRYINTEGRATION=true
GENERAL_SUBMODELREGISTRYINTEGRATION=true
GENERAL_EXTERNALURL=
GENERAL_TRUSTEDDYNAMICHOSTS=localhost:8082
```

Behavior:

- The service uses the direct request host only if it matches `trustedDynamicHosts`.
- If the request host includes a port, the allowlist entry must include the exact same host and port.
- A host-only allowlist entry, for example `localhost`, does not allow `localhost:8082`.
- If the request uses TLS, the derived scheme is `https`; otherwise it is `http`.
- The configured server context path is appended to the derived base URL.

Example request:

```text
Host: localhost:8082
```

With context path `/api/v3`, the derived base URL is:

```text
http://localhost:8082/api/v3
```

## Dynamic Reverse-Proxy Mode

Use reverse-proxy mode when a trusted proxy terminates public traffic and forwards requests to `aasenvironmentservice`.

```yaml
general:
  aasRegistryIntegration: true
  submodelRegistryIntegration: true
  externalUrl: ""
  trustProxyHeaders: true
  trustedProxyCIDRs:
    - 10.10.10.0/24
  trustedDynamicHosts:
    - example.com
```

Environment equivalent:

```env
GENERAL_AASREGISTRYINTEGRATION=true
GENERAL_SUBMODELREGISTRYINTEGRATION=true
GENERAL_EXTERNALURL=
GENERAL_TRUSTPROXYHEADERS=true
GENERAL_TRUSTEDPROXYCIDRS=10.10.10.0/24
GENERAL_TRUSTEDDYNAMICHOSTS=example.com
```

Behavior:

- `Forwarded` and `X-Forwarded-*` headers are trusted only when the direct peer IP is inside `trustedProxyCIDRs`.
- The service reads the forwarded scheme from `Forwarded: proto=...` or `X-Forwarded-Proto`.
- The service reads the forwarded host from `Forwarded: host=...` or `X-Forwarded-Host`.
- If `trustedDynamicHosts` is configured, the forwarded host must match it.
- If `trustedDynamicHosts` is empty, every forwarded host from a trusted proxy is accepted. Only use that setup when the proxy strictly validates or rewrites the forwarded host.
- If the request is not from a trusted proxy CIDR, forwarded headers are ignored and direct-host mode is used instead.

Example forwarded request from a trusted proxy:

```text
Forwarded: proto=https;host=example.com
```

With context path `/api/v3`, the derived base URL is:

```text
https://example.com/api/v3
```

## Host Matching Rules

Host matching is case-insensitive.

| Request host | Allowlist entry | Result |
| --- | --- | --- |
| `example.com` | `example.com` | Allowed |
| `example.com:443` | `example.com` | Rejected |
| `example.com:443` | `example.com:443` | Allowed |
| `[::1]` | `::1` | Allowed |
| `[::1]:8443` | `[::1]:8443` | Allowed |
| `[::1]:9443` | `[::1]:8443` | Rejected |

Use host:port allowlist entries whenever the public endpoint includes an explicit port.

## Descriptor Generation

When a trusted base URL is available, descriptors are generated as follows:

```text
<base-url>/shells/<base64url-aas-id>
<base-url>/submodels/<base64url-submodel-id>
```

Examples:

```text
https://example.com/api/v3/shells/dXJuOmV4YW1wbGU6YWFzOjAwMQ
https://example.com/api/v3/submodels/dXJuOmV4YW1wbGU6c206MDAx
```

If `general.externalUrl` is configured, it always takes precedence over request-derived dynamic URLs.

## Repository Mutation Behavior

When registry integration is enabled and a static or trusted dynamic base URL is available:

- AAS create/update operations write or update AAS registry descriptors.
- Submodel create/update operations write or update Submodel registry descriptors.
- Embedded submodel descriptors inside AAS descriptors are kept in sync where the service supports that relation.

When no static or trusted dynamic base URL is available:

- Create/update operations that would need new endpoint URLs skip descriptor writes instead of writing descriptors with empty endpoints.
- Delete and unlink operations still perform registry cleanup when possible, because deleting existing descriptors or removing embedded references does not require a new endpoint URL.

This means an untrusted request can mutate repository data, subject to normal authorization, without poisoning registry endpoint URLs.

## Startup Preconfiguration And Reconciliation

Startup preconfiguration can import AAS files before any public request is available. In dynamic mode, those startup imports may not have a trusted external base URL yet, so descriptor endpoint writes can be skipped.

After startup preconfiguration is complete, `aasenvironmentservice` waits for the first request that provides a trusted dynamic base URL. It then starts asynchronous reconciliation:

- AAS reconciliation backfills AAS registry descriptors when AAS registry integration is enabled.
- Submodel reconciliation backfills Submodel registry descriptors when Submodel registry integration is enabled.
- Reconciliation runs once per successfully reconciled base URL.
- Concurrent reconciliation is tracked per base URL to avoid duplicate work under host changes.
- Failed reconciliation does not mark the base URL as complete, so a later trusted request can retry it.
- Each reconciliation run is bounded by `general.dynamicRegistryReconciliationTimeoutSeconds`.

Reconciliation does not start before startup preconfiguration has finished. This prevents early health checks or UI requests from marking a base URL as reconciled before all startup-imported AAS data exists.

## Security Requirements

Do not enable dynamic direct-host mode without `trustedDynamicHosts`.

For direct-host deployments:

- Configure `trustedDynamicHosts`.
- Include the exact port when clients use an explicit public port.
- Do not allow arbitrary `Host` values to reach the service if those values should not become registry endpoints.

For reverse-proxy deployments:

- Configure `trustedProxyCIDRs` to match only the actual proxy source addresses.
- Ensure the proxy overwrites, validates, or strips inbound forwarding headers from clients.
- Prefer setting `trustedDynamicHosts` even in proxy mode.
- Leave `trustedDynamicHosts` empty only when the trusted proxy fully controls the forwarded host.

## Recommended Deployment Patterns

Stable production URL:

```env
GENERAL_EXTERNALURL=https://example.com/api/v3
```

Local compose with direct host:

```env
GENERAL_EXTERNALURL=
GENERAL_TRUSTEDDYNAMICHOSTS=localhost:8082
```

Reverse proxy with known public host:

```env
GENERAL_EXTERNALURL=
GENERAL_TRUSTPROXYHEADERS=true
GENERAL_TRUSTEDPROXYCIDRS=10.10.10.0/24
GENERAL_TRUSTEDDYNAMICHOSTS=example.com
```

Reverse proxy with proxy-controlled dynamic host:

```env
GENERAL_EXTERNALURL=
GENERAL_TRUSTPROXYHEADERS=true
GENERAL_TRUSTEDPROXYCIDRS=10.10.10.0/24
GENERAL_TRUSTEDDYNAMICHOSTS=
```

Use the last pattern only when the proxy is the trust boundary and clients cannot influence the forwarded host value.

## Troubleshooting

No registry descriptor endpoints are written:

- Check that `GENERAL_AASREGISTRYINTEGRATION=true` or `GENERAL_SUBMODELREGISTRYINTEGRATION=true`.
- Check whether `GENERAL_EXTERNALURL` is set. If it is blank, a trusted request-derived base URL is required.
- For direct-host mode, check that `GENERAL_TRUSTEDDYNAMICHOSTS` exactly matches the request `Host` header, including port.
- For proxy mode, check that the service sees the proxy source IP inside `GENERAL_TRUSTEDPROXYCIDRS`.
- Check that the proxy sends valid `Forwarded` or `X-Forwarded-*` headers.

Unexpected public URL appears in registry descriptors:

- Prefer static `GENERAL_EXTERNALURL` when possible.
- Add or tighten `GENERAL_TRUSTEDDYNAMICHOSTS`.
- In proxy mode, ensure the proxy strips client-supplied forwarded headers and overwrites them with trusted values.

Startup-imported descriptors are missing endpoints:

- Send one request with a trusted dynamic base URL after startup preconfiguration has completed.
- Check logs for `AASENV-DYNREGRECON-AASERR` or `AASENV-DYNREGRECON-SMERR`.
- Increase `GENERAL_DYNAMIC_REGISTRY_RECONCILIATION_TIMEOUT_SECONDS` for large repositories.
