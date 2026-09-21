# Scripts Folder

Utilities and scripts related to the basyx-go-components project.

## Build BaSyx Images

`build_images.sh` builds the twelve BaSyx service images through an interactive
wizard. It uses `SNAPSHOT`, all services, and Docker Build as the defaults. The
wizard asks for the image repository prefix (default `eclipsebasyx`) and
supports local Docker loads and BuildX multi-platform registry pushes.
It must be run from the Project root directory.

```bash
./scripts/build_images.sh
```

For automation, skip the wizard with explicit options:

```bash
./scripts/build_images.sh --non-interactive --services all --tag SNAPSHOT \
  --method buildx --output push --platform linux/amd64,linux/arm64 \
  --repository ghcr.io/eclipse-basyx --yes
```

The repository prefix defaults to `eclipsebasyx`. For example,
`--repository myregistry.example/team` produces
`myregistry.example/team/aasenvironment-go:SNAPSHOT`. `--registry` remains
available as an alias.

Use `--dry-run` to review generated Docker commands without running a build.

## Bruno Collection Generator

Script: `generate_bruno_collections.js`

### Purpose
Generates Bruno OpenCollection YAML files for example API client workflows from
the checked-in Postman collections.

The script currently generates:
- `examples/CatenaXample/bruno_collection`

The generated CatenaXample collection uses `bruno_collection/opencollection.yml` as
the collection root. Protected requests include Bruno-native pre-request
scripts for the Keycloak password-grant token flow.

### Usage

From repository root:

```bash
node scripts/generate_bruno_collections.js
```

Generated Bruno folders are marked with `.generated-by-postman-to-bruno`. The
script only overwrites folders with that marker.

## Reference Table Generator

Script: `reference_table_generator.sh`

### Purpose
Generates SQL DDL for AAS reference structures in a consistent and repeatable way.

The script creates the table triplets used across multiple contexts:
- `<context>_reference`
- `<context>_reference_key`
- `<context>_reference_payload` (optional, context-dependent)

This avoids manual copy/paste SQL, reduces schema drift, and keeps naming and constraints uniform.

### What it generates
For each configured context, the script writes:

1. A parent reference table with a typed reference root
2. A key table with ordered keys (`position`) and uniqueness on `(reference_id, position)`
3. Optionally, a payload table storing `parent_reference_payload` as JSONB

All foreign keys are generated with `ON DELETE CASCADE` to preserve referential cleanup behavior.

### Input / Output
- Input: no external config file; contexts are currently defined inside the script.
- Output: SQL file path passed as first argument.
	- Default output file: `draft_ref.sql`

### Usage

From repository root:

```bash
./scripts/reference_table_generator.sh
```

Custom output filename:

```bash
./scripts/reference_table_generator.sh basyx_reference_tables.sql
```

### Typical workflow
1. Run the generator.
2. Review the generated SQL file.
3. Apply relevant parts to a versioned schema patch under `database/patches/`, then register the patch and update schema version metadata when it becomes the current schema.
4. Run tests after schema changes.

### Failure behavior
The script fails fast (`set -euo pipefail`) and returns explicit error codes in the format:

`ERROR [GENREF-<STEP>] <message>`

This improves diagnosability for invalid context entries or missing configuration fields.

### Notes
- The script is deterministic for the configured context list order.
- Generated files are auto-generated artifacts and should not be hand-edited.
