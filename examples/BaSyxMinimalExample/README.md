# BaSyx Minimal Example (Go Components)

This example shows a minimal BaSyx setup with:

- AAS Environment Service (registry, repository, discovery, and concept descriptions in one backend)
- BaSyx Web UI
- Shared PostgreSQL database

## Prerequisites

- Docker + Docker Compose
- Free ports: `3000`, `8082`

## Start The Example

From this folder:

```bash
docker compose up -d
```

The setup includes a one-shot DB schema init container (`db-schema-init`).
Backends start only after:

1. Postgres is ready
2. Schema init finished successfully

## Open The UI

- AAS UI: [http://localhost:3000](http://localhost:3000)

## Quick Start

1. Open UI at [http://localhost:3000](http://localhost:3000)
2. Verify the AAS shell **IESEDriveMotorDM3000** appears in the UI (loaded automatically from [`aas/IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx) during startup)
3. Open the submodels:
   - **Nameplate** — digital nameplate with manufacturer info, serial number, markings
   - **TechnicalData** — electrical and mechanical specifications with product classifications
   - **HandoverDocumentation** — operating manual, data sheet, declaration of conformity (PDF)
   - **ContactInformations** — manufacturer contact details
   - **CarbonFootprint** — product carbon footprint (cradle-to-gate, phases A1–A3)
4. Check that concept descriptions are loaded (104 entries)

## Service Endpoints

- AAS Environment (all APIs): [http://localhost:8082](http://localhost:8082)
- Discovery API: [http://localhost:8082/lookup/shells](http://localhost:8082/lookup/shells)
- AAS Registry API: [http://localhost:8082/shell-descriptors](http://localhost:8082/shell-descriptors)
- Submodel Registry API: [http://localhost:8082/submodel-descriptors](http://localhost:8082/submodel-descriptors)
- AAS Repository API: [http://localhost:8082/shells](http://localhost:8082/shells)
- Submodel Repository API: [http://localhost:8082/submodels](http://localhost:8082/submodels)
- Concept Description Repository API: [http://localhost:8082/concept-descriptions](http://localhost:8082/concept-descriptions)

## Stop / Clean Up

Stop containers:

```bash
docker compose down
```

Stop and remove volumes:

```bash
docker compose down -v
```

## Notes

- This example is intentionally unsecured (`ABAC_ENABLED=false` for all Go services).
- The DB schema is initialized by `db-schema-init` and not by the application containers.
- The AAS UI endpoint mapping is configured via `basyx-infra.yml`.
- Startup preconfiguration is enabled with `GENERAL_AAS_PRECONFIG_PATHS=/app/preconfiguration`; files in `aas/` are imported automatically.
- Discovery integration and repository-to-registry integration are enabled in this example:
  - `GENERAL_DISCOVERYINTEGRATION=true`
  - `GENERAL_AASREGISTRYINTEGRATION=true`
  - `GENERAL_SUBMODELREGISTRYINTEGRATION=true`
- Descriptor endpoints are derived dynamically in this local compose setup. Direct request hosts are accepted only when they match `GENERAL_TRUSTEDDYNAMICHOSTS=localhost`.
- For production deployments with a stable public backend URL, prefer `GENERAL_EXTERNALURL=https://...`; comma-separated URLs are supported.
- For generic deployments behind a reverse proxy where the final public host is not known in the compose file, leave `GENERAL_EXTERNALURL` blank and configure the backend to trust only that proxy:
  - `GENERAL_TRUSTPROXYHEADERS=true`
  - `GENERAL_TRUSTEDPROXYCIDRS=<proxy CIDR>`
  - ensure the proxy sends `Forwarded` or `X-Forwarded-*` headers.
- Do not use dynamic direct-host mode without `GENERAL_TRUSTEDDYNAMICHOSTS`; unlisted `Host` values are ignored for registry descriptor generation.
- The UI integration hints in `basyx-infra.yml` are aligned with this behavior (`hasDiscoveryIntegration: true`, `hasRegistryIntegration: true`).
