# BaSyx Entra ID Example

This example runs the BaSyx AAS Environment and Web UI with Microsoft Entra ID
as the external OpenID Connect provider. It does not start an identity provider
locally.

## Entra ID Setup

Create two single-tenant app registrations:

1. Create a BaSyx API app registration.
   - Under **Expose an API**, keep the default Application ID URI:
     `api://<basyx-api-client-id>`.
   - Add the delegated scope `access_as_user`.
   - In the app manifest, set `"requestedAccessTokenVersion": 2`.
2. Create a Web UI single-page application registration.
   - Add `http://localhost:3000/` as a **Single-page application** redirect URI.
   - Add the delegated permission
     `api://<basyx-api-client-id>/access_as_user` from the BaSyx API app.
   - Grant consent if required by the tenant policy.

Replace the placeholders in:

- `basyx-infra.yml`
- `security_env/trustlist.json`

Then start the stack:

```bash
docker compose up -d
```

Open `http://localhost:3000`.

The compose file uses the `SNAPSHOT` Go images so the backend includes Entra
ID `scp` claim support. Images built from `1.0.0-rc.2` do not include this fix.

## Why `openid` Alone Fails

`openid` signs the user in and can produce an access token for the OpenID
Connect UserInfo endpoint. That token is not a BaSyx API access token. The Web
UI must request the exposed BaSyx API scope, and the backend must validate the
BaSyx API audience.

Entra ID stores delegated permissions in the `scp` access-token claim. The Go
components accept both the common `scope` claim and Entra ID's `scp` claim.

The included access model intentionally grants full API access to a signed-in
user with the `access_as_user` delegated scope. Add Entra ID app roles and adapt
the access rules before using the example as a production authorization model.

## App-Only Tokens

Application permissions are emitted in Entra ID's `roles` claim rather than the
delegated `scp` claim. When one issuer must accept both delegated and app-only
tokens, remove mandatory `scopes` from the trustlist and express the alternatives
in ABAC. For an app registration that emits exactly one role per token, add a
scalar mapping:

```json
{
  "issuer": "https://login.microsoftonline.com/<tenant-id>/v2.0",
  "audience": "<basyx-api-client-id>",
  "claimMappings": [
    { "target": "role", "mode": "scalar", "sources": ["/roles"] }
  ]
}
```

The mapped claim is available as `basyx.role` after JWT verification and can be
checked with the existing Part 4 `$eq` operator:

```json
{ "$eq": [{ "$attribute": { "CLAIM": "basyx.role" } }, { "$strVal": "admin" }] }
```

Scalar mappings reject arrays with more than one item. Multi-value app-role
authorization needs a policy design that does not rely on substring matching;
the current grammar has no exact list-membership operator.

The access-token version is selected by the API app registration, independently
of the authorization endpoint version. Without `"requestedAccessTokenVersion":
2`, Entra ID may issue a v1 token with an `https://sts.windows.net/.../` issuer,
which does not match the v2 trustlist issuer.

Any local `openid-configuration.json` and `Bild.png` files are diagnostic
references only and are ignored by Git. The services fetch OpenID discovery
metadata from the issuer URL at startup.

## Troubleshooting

- `Failed to read token issuer: token has invalid format`: the bearer token is
  not a JWT for the BaSyx API. Verify that the Web UI requests the exposed
  `api://<basyx-api-client-id>/access_as_user` scope.
- `unknown token issuer`: inspect the token locally and compare its `iss`, `ver`,
  `aud`, and `scp` claims with `security_env/trustlist.json`. A v1 token has an
  `https://sts.windows.net/.../` issuer; set `"requestedAccessTokenVersion": 2`
  on the BaSyx API registration and sign in again.

Treat access tokens as credentials. Do not paste them into issue reports.
