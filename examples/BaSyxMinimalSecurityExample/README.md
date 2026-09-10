# BaSyx Minimal Security Example

This example combines ABAC and resource-bound access control in one AAS Environment Service. It contains only the components required for an end-to-end demonstration:

- AAS Environment with repository, registry, discovery, and Concept Description APIs
- PostgreSQL and the BaSyx Configuration Service
- Keycloak with two demo users and group claims
- ReBAC-enabled AAS GUI `registry.erbenschell.iese.fraunhofer.de/basyx/uni_bw_bruecken/aas-gui:rebac-1.0.0`

## Access model

The service runs with `SECURITY_AUTHORIZATION_MODE=resource-bound-first` and `ABAC_ENABLED=true`.

- ABAC keeps broad, stable permissions: collection reads and creation, anonymous service information, `viewer` access to the Concept Description catalog, and ABAC policy administration for the `admin` role.
- ReBAC controls concrete AAS, Submodels, Submodel Elements, descriptors, Discovery entries, ownership, and sharing.
- A resource created by an authenticated user automatically records that user as its direct owner, which implicitly grants `ALL` on that resource.
- `/aas-admins` is the bootstrap owner group for resources present when the policy scope is initialized.

An ABAC allow remains an allow. Do not grant broad ABAC access to resources that should be restricted exclusively through ReBAC.

## Prerequisites

- Docker with Compose
- Free ports `3100`, `8180`, and `8182`
- Access to the configured Fraunhofer container registry
- `keycloak.localhost` resolving to `127.0.0.1` (the `.localhost` domain normally does this automatically)

## Start

From this directory:

```bash
docker compose up --build -d
```

Open:

- AAS GUI: [http://localhost:3100](http://localhost:3100)
- Keycloak: [http://keycloak.localhost:8180](http://keycloak.localhost:8180)
- AAS Environment: [http://localhost:8182](http://localhost:8182)

Demo users:

| User | Password | Role | Group | Stable subject |
|---|---|---|---|---|
| `admin` | `pwd` | `admin` | `/aas-admins` | `32103f0d-1d0e-4d94-b720-bcd22e1c7709` |
| `usera` | `pwd` | `viewer` | `/aas-viewers` | `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa` |

These credentials are for local demonstration only.

## Create an AAS as `usera`

1. Sign in to the GUI as `usera`.
2. Create an AAS. The `viewer` role already has ABAC `CREATE` and `READ` admission on the AAS collection.
3. The creation transaction makes `usera` the owner with implicit `ALL` on that AAS. It consequently appears in `usera`'s filtered AAS list; AAS resources owned by other users remain hidden unless ReBAC or object-based ABAC grants access.

The administrator or owner can then grant `READ`, `UPDATE`, `DELETE`, `EXECUTE`, `VIEW`, or `ALL`, and manage owners and managers on a concrete resource. There is no Collection target in ReBAC Access Management.

## Grant access to a group

The Keycloak client emits full group paths in the `groups` claim. Select a group principal in Access Management or send one through the resource `/$access` API:

```json
{
  "principal": {
    "type": "group",
    "issuer": "http://keycloak.localhost:8180/realms/basyx",
    "subject": "/aas-viewers"
  },
  "rights": ["READ"]
}
```

Group membership becomes effective after the user obtains a new token. If the example was started before group support was added, run the reset command below so Keycloak reimports the realm and the configuration service applies the new database patch.

## Verify the ABAC part

Sign in as `usera`. The `viewer` claim grants read access to Concept Descriptions through [`security_env/access-rules.json`](security_env/access-rules.json), independently of resource-specific ReBAC grants.

## Reset

```bash
docker compose down -v
```

Removing the volume resets application, authorization, and Keycloak state.
