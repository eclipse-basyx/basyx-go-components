# MCP Endpoint

The AAS Environment can serve a [Model Context Protocol](https://modelcontextprotocol.io) (MCP)
endpoint next to its REST API. MCP clients such as Claude Code or Codex then read and change Asset
Administration Shells, submodels and submodel elements through it.

Each tool call is a request to the AAS Environment API, sent inside the service with the caller's
token. Authentication, ABAC and ReBAC rules, validation, conditional requests, history and eventing
behave as they do for REST calls.

## Enable the Endpoint

| Setting | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `mcp.enabled` | `MCP_ENABLED` | `false` | Serve the MCP endpoint |
| `mcp.path` | `MCP_PATH` | `/mcp` | Endpoint path below the context path |
| `mcp.writesEnabled` | `MCP_WRITESENABLED` | `false` | Offer the tools that create, change and delete data |
| `mcp.maxResultBytes` | `MCP_MAXRESULTBYTES` | `262144` | Largest API response a tool returns; larger results fail with a hint to page or narrow the request |

The endpoint is `<general.externalUrl><mcp.path>`, for example `http://localhost:8081/mcp`. Without
`general.externalUrl` the service uses `http://localhost:<server.port><server.contextPath>`. The
endpoint uses the stateless Streamable HTTP transport.

## Connect a Client

Claude Code:

```bash
claude mcp add --transport http basyx http://localhost:8081/mcp
```

Codex:

```bash
codex mcp add basyx --url http://localhost:8081/mcp
```

If the endpoint requires a token, the client starts a browser sign-in. Run `/mcp` in Claude Code or
`codex mcp login basyx`. Clients that run in the cloud, for example custom connectors on claude.ai,
need an endpoint that is reachable over the internet with HTTPS.

The [MCP example](../../examples/BaSyxMCPExample/README.md) starts a local AAS Environment with MCP
enabled.

## Tools

| Tool | API call | Hint |
| --- | --- | --- |
| `list_shells` | `GET /shells` | read-only |
| `get_shell` | `GET /shells/{id}` | read-only |
| `list_submodels` | `GET /submodels` | read-only |
| `get_submodel` | `GET /submodels/{id}` or `/$value` | read-only |
| `get_submodel_element` | `GET /submodels/{id}/submodel-elements/{path}` or `/$value` | read-only |
| `get_concept_description` | `GET /concept-descriptions/{id}` | read-only |
| `query_shells` | `POST /query/shells` | read-only |
| `query_submodels` | `POST /query/submodels` | read-only |
| `create_shell` | `POST /shells` | additive |
| `create_submodel` | `POST /submodels`, optionally `POST /shells/{id}/submodel-refs` | additive |
| `add_submodel_element` | `POST /submodels/{id}/submodel-elements[/{path}]` | additive |
| `set_submodel_element_value` | `PATCH /submodels/{id}/submodel-elements/{path}/$value` | destructive |
| `delete_shell` | `DELETE /shells/{id}` | destructive |
| `delete_submodel` | `DELETE /submodels/{id}` | destructive |
| `delete_submodel_element` | `DELETE /submodels/{id}/submodel-elements/{path}` | destructive |

The write tools are only listed with `mcp.writesEnabled: true`. Every tool carries MCP tool
annotations. MCP clients use them to ask the user before a tool changes data; the server adds no
confirmation step of its own.

Tools take identifiers as plain text and encode them for the API. Collection tools accept `limit` and
`cursor` and return `paging_metadata.cursor` for the next page. `list_submodels` matches `semanticId`
against external references with a global reference key.

Read tools return the resource's `etag`. Pass it to a change or delete tool to make the request
conditional. If the resource changed in the meantime, the tool fails with HTTP 412 and the client can
read it again. With `server.conditionalRequests.requireIfMatch`, changes without an `etag` fail with
HTTP 428.

`create_submodel` with `shellId` makes two API calls. If the shell reference cannot be added, the new
submodel stays and the tool says so.

Failed calls return a tool error with the API status, the API error message and a hint, for example
`MCP-DISPATCH-HTTP404: Not Found ... Check the identifier or idShortPath`.

## Security

With `abac.enabled: false` the endpoint has no authentication, like the REST API. Do not expose it,
and do not enable writes on an endpoint others can reach.

With `abac.enabled: true` the endpoint requires a bearer token and follows the
[MCP authorization specification](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization):

- A request without a valid token receives HTTP 401 with
  `WWW-Authenticate: Bearer resource_metadata="<metadata URL>"`.
- The service publishes OAuth 2.0 protected resource metadata (RFC 9728) at
  `/.well-known/oauth-protected-resource<context path><mcp.path>`. Its `resource` is the endpoint URL
  and its `authorization_servers` are the issuers of the OIDC trustlist. A reverse proxy in front of
  the service must forward this path as well.
- Tokens are verified with the OIDC trustlist of the REST API. Every trusted issuer must have an
  `audience`; otherwise the service does not start. Tokens for other audiences are rejected.

### Identity Provider Requirements

The endpoint works with any OpenID Connect provider that the trustlist accepts. For Claude Code,
Codex and other MCP clients to sign in, the provider must:

1. Serve its endpoints over HTTPS with a certificate the client trusts. MCP clients do not open
   sign-in pages over plain HTTP, even on `localhost`.
2. Publish OAuth authorization server metadata or OpenID Connect discovery.
3. Support the authorization code flow with PKCE (`S256`).
4. Let the client obtain a client ID, in one of two ways:
   - Dynamic client registration (RFC 7591). Clients register themselves with a loopback redirect
     URI such as `http://localhost:<port>/callback`.
   - A public client registered by an administrator. Pass its ID to the client, for example
     `claude mcp add --transport http --client-id <id> --callback-port <port> basyx <url>`, and
     register `http://localhost:<port>/callback` as its redirect URI.
5. Put the trustlist `audience` into the access tokens it issues to these clients. MCP clients send
   the endpoint URL as RFC 8707 `resource` parameter, but many providers ignore it, so configure the
   audience in the provider.

For Keycloak, the [secured MCP example](../../examples/BaSyxMCPExample/secured/README.md) shows one
setup:

- A client scope `basyx-aas-environment` with an audience mapper, added to the realm's default
  client scopes. Dynamically registered clients receive it.
- The anonymous client registration policy *Trusted Hosts* lists `localhost` and `127.0.0.1` and
  checks the redirect URIs, but not the host that sends the registration request.
- The other anonymous registration policies stay in place, in particular *Allowed Protocol Mapper
  Types* and *Allowed Client Scopes*. They keep self-registered clients from adding protocol mappers
  that put arbitrary claims, which ABAC rules may evaluate, into their tokens. A realm import that
  defines its own client scopes or registration policies replaces Keycloak's defaults, so it must
  list these policies explicitly.

Providers without dynamic client registration, for example Microsoft Entra ID, need a registered
client.
