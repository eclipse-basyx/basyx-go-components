# BaSyx Minimal Example (Go Components)

This example shows a minimal BaSyx setup with:

- AAS Registry
- Submodel Registry
- AAS Discovery
- AAS Repository
- Submodel Repository
- Concept Description Repository
- BaSyx Web UI
- Shared PostgreSQL database

## Prerequisites

- Docker + Docker Compose
- Free ports: `3000`, `8082`, `8083`, `8084`, `8090`, `8091`, `8092`

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
2. Upload [`aas/IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx)
3. Verify the AAS shell **IESEDriveMotorDM3000** appears in the UI
4. Open the submodels:
   - **Nameplate** — digital nameplate with manufacturer info, serial number, markings
   - **TechnicalData** — electrical and mechanical specifications with product classifications
   - **HandoverDocumentation** — operating manual, data sheet, declaration of conformity (PDF)
   - **ContactInformations** — manufacturer contact details
   - **CarbonFootprint** — product carbon footprint (cradle-to-gate, phases A1–A3)
5. Check that concept descriptions are loaded (104 entries)

## Service Endpoints

- AAS Registry: [http://localhost:8082](http://localhost:8082)
- Submodel Registry: [http://localhost:8083](http://localhost:8083)
- AAS Discovery: [http://localhost:8084](http://localhost:8084)
- AAS Repository: [http://localhost:8090](http://localhost:8090)
- Submodel Repository: [http://localhost:8091](http://localhost:8091)
- Concept Description Repository: [http://localhost:8092](http://localhost:8092)

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
