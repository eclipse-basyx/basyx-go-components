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
- Manage vulnerability handling, patching, and dependency updates.
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

Avoid describing a deployment as "NIS2 compliant" based only on BaSyx configuration. The accurate statement is that BaSyx supplies technical controls that may support NIS2-aligned operation when combined with the required organisational and operational measures.
