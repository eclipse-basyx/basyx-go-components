# Resource-bound bridge example

This example builds the local AAS Environment and configuration service and uses PostgreSQL. Supply an existing OIDC provider reachable from the containers.

1. Replace `https://issuer.example`, the audience, and subject IDs in `config.yaml`, `trustlist.json`, and `fallback.json` with your verified OIDC identities. The owner needs a token for the configured audience. `bridge-auditor` demonstrates object-based ABAC fallback.
2. Run `docker compose up --build` in this directory.
3. Set `OWNER_TOKEN`, `ISSUER`, `OWNER_SUBJECT`, `READER_SUBJECT`, and `MANAGER_SUBJECT`, then run `python3 share_bridge.py`.

The script creates a bridge AAS and an inspection Submodel, installs a local Submodel policy, shares READ with a user, delegates management, and overrides inheritance on `InternalCost`. It reads the current access ETag before every mutation.

A reader receives the visible Submodel content without `InternalCost`. The manager cannot administer that overridden child or change owners. A directly assigned owner can administer its policy even without data READ. An auditor authorized by `fallback.json` can read the otherwise ungranted child through ABAC fallback.

Collection access is configured in ABAC. Manage concrete resource policies through `/$access/policy`. Restarting does not overwrite access changes. Stop the example with `docker compose down`.

See [resource-bound security](../../docu/security/RESOURCE_BOUND_ACCESS.md) for API contracts, inheritance, supported routes, and test commands.
