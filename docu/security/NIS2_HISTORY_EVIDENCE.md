# NIS2 History Evidence Guidance

BaSyx provides technical controls that can support NIS2-aligned integrity, auditability, traceability, recovery, and tamper-detection requirements when deployed and operated correctly. Enabling history evidence does not make an operator NIS2 compliant by itself.

## BaSyx Technical Controls

- Canonical `content_hash`, `payload_hash`, `previous_hash`, and `row_hash` values protect history row integrity.
- Append-only PostgreSQL history rows and optional guarded PostgreSQL mode support traceability.
- History-independent WORM `mutation_event` chains provide tamper-evident recovery material for acknowledged writes while evidence storage is enabled, including when PostgreSQL history is off.
- Internal attachment and thumbnail uploads add content-addressed immutable binary evidence. The corresponding mutation hash commits the digest, size, filename, content type, managed path, and immutable object version before the owner-scoped reference artifact is written.
- Activation-time WORM `abac_policy_version` artifacts preserve the configured and materialized access-rule version that produced audited `policy_id` and `matched_rule_id` values.
- Signed manifests and range digests support independent verification of selected history ranges.
- CLI verification and recovery export detect missing tails against an independently retained expected sequence/hash head, modified, reordered, conflicting, retention-unverifiable, or binary-mismatched evidence and identify the terminal deletion/change state.
- Audit context fields record request, OIDC, ABAC, route, and trusted proxy metadata when configured and available.

## Operator Responsibilities

- Perform risk analysis and maintain information security policies.
- Define incident handling, escalation, crisis management, and evidence preservation processes.
- Operate backup management, disaster recovery, and restore drills.
- Configure and monitor S3 Object Lock or equivalent WORM storage, including retention, versioning, IAM, MFA, and least privilege.
- Generate, rotate, back up, and distribute signing and verification keys through controlled trust processes.
- Monitor verifier findings and alert on critical drift or signature failures.
- Retain verified mutation sequence/hash heads outside the BaSyx PostgreSQL database and advance them only after successful verification. Protect the first trust-on-first-use baseline through an operator-controlled process.
- Back up and restore the PostgreSQL evidence catalog with the database. The built-in mutation verifier uses it to locate committed WORM objects and their immutable versions.
- Configure object-store lifecycle deletion separately when required. Object Lock retention expiry permits deletion but does not perform it; keep lifecycle periods aligned with legal and operational verification windows.
- Manage vulnerability handling, patching, and dependency updates.
- Use a controlled, quiesced v1.1.8 upgrade with a verified PostgreSQL backup that includes Large Objects.
- Assess infrastructure and provider supply-chain security.
- Train staff and maintain cyber hygiene practices.
- Validate legal and regulatory compliance with the operator's security and legal teams.

## Deployment Notes

- Use `history.evidence.signing.publicKeyPath` or `BASYX_HISTORY_EVIDENCE_SIGNING_PUBLIC_KEY_PATH` for manifest signature verification.
- Run `cmd/historyevidenceverifier` from cron, Kubernetes CronJobs, or an equivalent scheduler. Treat non-zero exits and `severity: error` findings as alert conditions.
- Supply `-expected-head-hash` from the independently protected head corresponding to the requested `-to` sequence. Do not derive this expected value from the database under verification.
- Use `history.fullSnapshotInterval: 1` when each mutation must be recoverable as a full WORM snapshot without diff replay.
- With `history.fullSnapshotInterval: N`, recovery starts from the nearest WORM snapshot and replays WORM diff artifacts up to the requested evidence sequence.
- Recovery is bounded by the retention period and by the mutations for which evidence storage was enabled. The v1.1.8 upgrade does not convert or retroactively copy existing binaries to WORM; those files remain available through compatibility reads without historical receipts.
- The built-in recovery command exports verified JSON only. PostgreSQL restore should be performed through an operator-approved disaster-recovery procedure.
- Evidence-only PostgreSQL storage contains bounded per-entity chain heads and per-event receipt metadata, not model snapshots or diffs. Receipt rows and WORM mutation/reference artifacts still grow linearly with writes; unique binary payloads are deduplicated.

## Production WORM Smoke Test

Run this check after provisioning or materially changing the production WORM configuration, using a dedicated test AAS or Submodel that the operator is authorized to mutate:

1. Perform a model mutation and upload a new internal attachment or thumbnail, then upload the same bytes again. Both requests must succeed; each upload must produce a distinct managed model path.
2. Read the latest `event_sequence` and `event_hash` for the test entity from `mutation_evidence_artifacts`. Store that pair in a protected monitoring, SIEM, or evidence-preservation system outside the BaSyx database.
3. Run the existing mutation verifier with the real production object-store configuration and the externally retained hash:

   ```sh
   go run ./cmd/historyevidenceverifier \
     -config ./config.yaml \
     -mutation \
     -table submodel_history \
     -identifier '<test-submodel-identifier>' \
     -from 1 \
     -to '<retained-event-sequence>' \
     -expected-head-hash '<externally-retained-event-hash>'
   ```

4. Require a zero exit status and no `severity: error` findings. This verifies the mutation chain, required binary-reference and immutable-binary receipts, object bytes, and live retention state. Preserve the verified head and advance it only after the next newer head passes the same check.

Use an equivalent `aas_history` command for a thumbnail test. This procedure uses ordinary evidence writes and verification; it does not require startup bucket-inspection permissions or permanent probe objects.

Global binary deduplication can expose a noisy new-content-versus-reused-content timing signal to an already-authorized uploader. Deployments with a genuine cross-tenant timing-isolation requirement should use separate databases or service instances for those security boundaries.

Avoid describing a deployment as "NIS2 compliant" based only on BaSyx configuration. The accurate statement is that BaSyx supplies technical controls that may support NIS2-aligned operation when combined with the required organisational and operational measures.
