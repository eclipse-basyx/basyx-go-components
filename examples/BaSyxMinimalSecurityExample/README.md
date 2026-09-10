# BaSyx Minimal Security Example

This example combines ABAC and resource-bound access control in one AAS Environment Service. It contains only the components required for an end-to-end demonstration:

- AAS Environment with repository, registry, discovery, and Concept Description APIs
- PostgreSQL and the BaSyx Configuration Service
- Keycloak with two demo users
- ReBAC-enabled AAS GUI `registry.erbenschell.iese.fraunhofer.de/basyx/uni_bw_bruecken/aas-gui:rebac-1.0.0`

## Access model

The service runs with `SECURITY_AUTHORIZATION_MODE=resource-bound-first` and `ABAC_ENABLED=true`.

- ABAC keeps broad, stable permissions: anonymous service information, `viewer` access to the Concept Description catalog, and ABAC policy administration for the `admin` role.
- ReBAC controls AAS, Submodels, Submodel Elements, descriptors, Discovery entries, ownership, sharing, and collection `CREATE` permissions.
- The initial collection policies give the demo administrator `ALL`. A resource created by an authenticated user automatically records that user as its direct owner, which implicitly grants `ALL` on that resource.

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

| User | Password | Role | Stable subject |
|---|---|---|---|
| `admin` | `pwd` | `admin` | `32103f0d-1d0e-4d94-b720-bcd22e1c7709` |
| `usera` | `pwd` | `viewer` | `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa` |

These credentials are for local demonstration only.

## Grant permission to create an AAS

1. Sign in to the GUI as `admin`.
2. Open **Access Management**.
3. Select **AAS collection**.
4. Add a grant with:
   - Issuer: `http://keycloak.localhost:8180/realms/basyx`
   - Subject: `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa`
   - Right: `CREATE`
5. Sign out and sign in as `usera`.
6. Create an AAS. The creation transaction makes `usera` the owner with implicit `ALL` on that AAS. It consequently appears in `usera`'s filtered AAS list without granting `READ` on the complete `/shells` collection.

The administrator can then grant `READ`, `UPDATE`, `DELETE`, `EXECUTE`, `VIEW`, or `ALL`, and manage owners and managers from the same dialog. Registry and Discovery resources are available through the same Access Management module.

## Verify the ABAC part

Sign in as `usera`. The `viewer` claim grants read access to Concept Descriptions through [`security_env/access-rules.json`](security_env/access-rules.json), independently of resource-specific ReBAC grants.

## Reset

```bash
docker compose down -v
```

The ReBAC bootstrap policies are imported only for a new policy scope. Removing the volume resets both application and Keycloak state.
