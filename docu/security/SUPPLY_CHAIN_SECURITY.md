# Supply Chain Security

This repository uses open standards to secure release and snapshot container artifacts:

- Sigstore Cosign keyless signatures (OIDC, no long-lived signing key)
- in-toto attestations for provenance and SBOM
- SPDX and CycloneDX SBOM export for release artifacts
- OCI metadata labels for traceability
- GitHub Actions OIDC and least-privilege workflow permissions

## What Is Produced

For each service image:

- Multi-architecture OCI image (Docker Hub)
- Cosign signature on immutable image digest
- Provenance attestation (`slsaprovenance`)
- SBOM attestation (SPDX predicate)
- Additional release assets:
  - `*.spdx.json`
  - `*.cdx.json` (CycloneDX)
  - per-service metadata JSON
  - release supply-chain manifest JSON

## Trust Model

Images are signed by GitHub Actions keyless identity for this repository.

Expected certificate identity values:

- Release workflow:
  - `https://github.com/eclipse-basyx/basyx-go-components/.github/workflows/docker-release.yml@refs/tags/<tag>`
- Snapshot workflow:
  - `https://github.com/eclipse-basyx/basyx-go-components/.github/workflows/docker-snapshot.yml@refs/heads/main`

Expected OIDC issuer:

- `https://token.actions.githubusercontent.com`

## Verify Image Signatures

Example (replace image and digest):

```bash
IMAGE="eclipsebasyx/aasregistry-go@sha256:<digest>"
IDENTITY="https://github.com/eclipse-basyx/basyx-go-components/.github/workflows/docker-release.yml@refs/tags/v1.2.3"

cosign verify \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "$IMAGE"
```

For snapshots, use the snapshot workflow identity.

## Verify Provenance Attestations

```bash
IMAGE="eclipsebasyx/aasregistry-go@sha256:<digest>"
IDENTITY="https://github.com/eclipse-basyx/basyx-go-components/.github/workflows/docker-release.yml@refs/tags/v1.2.3"

cosign verify-attestation \
  --type slsaprovenance \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "$IMAGE"
```

## Verify SBOM Attestations

```bash
IMAGE="eclipsebasyx/aasregistry-go@sha256:<digest>"
IDENTITY="https://github.com/eclipse-basyx/basyx-go-components/.github/workflows/docker-release.yml@refs/tags/v1.2.3"

cosign verify-attestation \
  --type "https://spdx.dev/Document" \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "$IMAGE"
```

## Download OCI-Attached SBOM

```bash
IMAGE="eclipsebasyx/aasregistry-go@sha256:<digest>"
cosign download sbom "$IMAGE" > sbom-from-oci.spdx.json
```

This command demonstrates OCI discoverability of SBOM data for published images.

## Retrieve Release SBOM Assets

GitHub Releases include per-service exported SBOM files:

- `service-version.spdx.json`
- `service-version.cdx.json`

These assets are generated in CI and attached to the published release.

## Semantic Version and Provenance Consistency

Release workflow enforces semantic version parsing from the Git tag.

- Stable releases receive `major.minor.patch`, `major.minor`, `major`, and `latest` tags.
- Pre-releases keep only explicit pre-release tags.
- Metadata labels include source repository, commit SHA, and version.
- Provenance and signatures are anchored to the immutable digest.

## Vulnerability Scanning

The repository provides a report-only Trivy workflow for continuous visibility.

- Findings are uploaded as SARIF.
- Current mode does not fail builds.
- Maintainers can later switch to fail on threshold (for example `HIGH,CRITICAL`) once baseline noise is reduced.

## Migration Notes

- Existing consumers that previously pulled by mutable tag should migrate to digest-pinned pulls where possible.
- Verification commands now expect digest references (`image@sha256:...`) rather than tag-only references.
- Release pages now contain per-service SPDX and CycloneDX files; automation that consumed old release assets should update file matching patterns.
- Snapshot artifacts remain non-stable by definition; use release tags for production deployment and compliance evidence.

## Current SLSA Posture and Next Steps

Current improvements align with common SLSA build integrity practices:

- Immutable action pinning in workflows
- OIDC-based signing identity
- Provenance attestation and verification in CI
- Signed digest-first release model

Further hardening options:

1. Enforce branch/tag protection and required checks for release publication.
2. Add policy gates that block releases on unsigned/unverifiable artifacts.
3. Add reproducibility checks across repeated builds of the same commit.
4. Add admission-policy examples for downstream runtime verification.
