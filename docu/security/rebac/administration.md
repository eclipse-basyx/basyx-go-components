# ReBAC Administration Guide

This guide is for administrators who set up and operate relationship-based
access control (ReBAC) in BaSyx Go. Read the [overview](README.md) first,
in particular how ReBAC and ABAC interplay. The
[Sharing API](sharing-api.md) describes the endpoints referenced here.

## Contents

- [Prerequisites](#prerequisites)
- [Setup](#setup)
- [Identity providers](#identity-providers)
- [Existing data](#existing-data)
- [Operations](#operations)
- [Audit trail and evidence](#audit-trail-and-evidence)
- [Troubleshooting](#troubleshooting)
- [Limits](#limits)
- [Upgrade notes](#upgrade-notes)

## Prerequisites

- **ABAC and OIDC.** ReBAC extends ABAC, so every service needs
  `abac.enabled=true` and a readable OIDC trustlist (`oidc.trustlistPath`).
  A service with ReBAC enabled refuses to start otherwise
  (`REBAC-SETUP-ABACREQUIRED`, `REBAC-SETUP-OIDCREQUIRED`). Anonymous
  requests are always ABAC-only.
- **JWT access tokens.** BaSyx validates access tokens locally against the
  keys the issuer publishes. The identity provider must issue signed JWT
  access tokens; opaque tokens that need introspection are not supported.
- **A stable user identifier** in the token that is the same for a person
  in every client of the API (see [Identity providers](#identity-providers)).
- **Database schema v1.2.2.** Run the configuration service before starting
  the upgraded services (see [Upgrade notes](#upgrade-notes)).

## Setup

### 1. Configure every service

Enable ReBAC in **every** service that shares the database: repositories,
registries, discovery, AASX file server, AAS Environment and DPP API.

| Key | Env | Default | Purpose |
| --- | --- | --- | --- |
| `rebac.enabled` | `REBAC_ENABLED` | `false` | Master switch |
| `rebac.subjectClaim` | `REBAC_SUBJECT_CLAIM` | `sub` | Claim with the stable user identifier |
| `rebac.groupClaim` | `REBAC_GROUP_CLAIM` | `groups` | Claim with group names |
| `rebac.administrators` | `REBAC_ADMINISTRATORS` (comma-separated) | `[]` | Administrators as `issuer\|subject` or `issuer\|group:<name>` |

```yaml
abac:
  enabled: true
  modelPath: /security_env/access-rules.json
oidc:
  trustlistPath: /security_env/trustlist.json
rebac:
  enabled: true
  subjectClaim: sub
  groupClaim: groups
  administrators:
    - "https://idp.example.com/realms/basyx|group:operators"
```

Users and groups are scoped by issuer. `subject` in `administrators` is the
value of the configured subject claim.

A service that shares the database but runs **without** ReBAC ignores all
grants, assigns no owner to resources created through it, and does not
remove the grants of resources it deletes. The next ReBAC-enabled startup or
reconciliation removes such orphaned state, but resources created in the
meantime stay without owner. Enable ReBAC consistently.

### 2. Map the claims

`rebac.subjectClaim` and `rebac.groupClaim` name top-level claims of the
access token. If the provider puts the values elsewhere or under another
name, map them in the trustlist entry of the issuer. Mapped claims are
available with the prefix `basyx.`:

```json
[
  {
    "issuer": "https://idp.example.com/realms/basyx",
    "audience": "basyx-api",
    "claimMappings": [
      { "target": "groups", "mode": "list", "sources": ["/groups", "/realm_access/roles"] }
    ]
  }
]
```

With this entry, set `rebac.groupClaim` to `basyx.groups`. Group values are
used exactly as they appear in the token, so decide on one form (for
example `engineering`, not `/engineering`) before people start sharing.

### 3. Allow what the policy must still allow

ReBAC decides only for covered resource routes. Add these ABAC rules:

- **`/description` for everyone.** Clients detect ReBAC through the profile
  `https://basyx.org/aas/API/3/2/RelationshipBasedAccessControl/1.0` in the
  service description.
- **List routes for signed-in users.** For a list request, ReBAC applies
  when the caller holds at least one relevant grant and then filters the
  list in SQL. A caller without any grant falls back to the ABAC decision,
  which is `403` without a matching rule. To return empty lists instead,
  allow the list routes for signed-in users with a formula that matches no
  resource, so only ReBAC contributes entries:

```json
{
  "DEFATTRIBUTES": [
    { "name": "authenticated", "attributes": [ { "CLAIM": "sub" } ] }
  ],
  "DEFOBJECTS": [
    { "name": "shell_lists", "objects": [ { "ROUTE": "/shells" }, { "ROUTE": "/query/shells" } ] }
  ],
  "DEFACLS": [
    { "name": "authenticated_read", "acl": { "USEATTRIBUTES": "authenticated", "RIGHTS": ["READ"], "ACCESS": "ALLOW" } }
  ],
  "DEFFORMULAS": [
    { "name": "no_shell_lists", "formula": { "$eq": [ { "$field": "$aas#id" }, { "$strVal": "urn:basyx:none" } ] } }
  ],
  "rules": [
    { "USEACL": "authenticated_read", "USEOBJECTS": [ "shell_lists" ], "USEFORMULA": "no_shell_lists" }
  ]
}
```

Repeat the pattern for `/submodels` (`$sm#id`), `/concept-descriptions`
(`$cd#id`), `/shell-descriptors` (`$aasdesc#id`), `/submodel-descriptors`
(`$smdesc#id`) and `/lookup/shells` with `/lookup/shellsByAssetLink`
(`$bd#globalAssetId`). A constant `false` formula does not work: the ABAC
engine treats it as no match. The complete rules are in
[examples/BaSyxReBACExample/security_env/access-rules.json](../../../examples/BaSyxReBACExample/security_env/access-rules.json).

The management API (`$access`, `/security/rebac/…`) needs no ABAC rule. It
authorizes with ReBAC permissions and administrator status only.

If the ABAC policy is stored in the database, edits to the rules file are
only imported according to `abac.policyFileImport` (with `if_missing`, not
at all once a policy exists). Change the rules through the policy management
API or import them explicitly, see
[ABAC_POLICY_REPOSITORY.md](../ABAC_POLICY_REPOSITORY.md).

### 4. Allow the BaSyx UI

The BaSyx UI detects ReBAC on its own and needs no ReBAC-specific
configuration. The BaSyx services must accept its origin and the headers
sharing uses:

```yaml
- CORS_ALLOWEDORIGINS=https://ui.example.com
- CORS_ALLOWEDHEADERS=Authorization,Content-Type,If-Match
- CORS_ALLOWCREDENTIALS=true
- CORS_ALLOWEDMETHODS=GET,POST,PUT,PATCH,DELETE,OPTIONS
```

`ETag` and `Location` are exposed to browsers automatically. Register the
UI's redirect URIs and web origin with the identity provider, and make sure
its access tokens carry the audience of the trustlist entry.

### 5. Let people create resources

Right after setup, only the configured administrators and callers with an
ABAC `CREATE` right can create resources. Grant `creator` on the repository
families people should create in, preferably to groups:

- In the BaSyx UI: **Access Management** → **Repositories**, select the
  repository, add the user or group as **Creator**.
- Through the API: `PUT /security/rebac/repositories/{kind}/$access/grants`
  (see [Sharing API](sharing-api.md#repository-grants)).

Grant `admin` on a repository family only to people who may see and manage
every resource of that family.

## Identity providers

ReBAC works with any OIDC provider that issues JWT access tokens. It only
reads the issuer, the subject claim and the group claim. Several issuers can
be trusted at the same time; the same person at two issuers counts as two
users.

### Keycloak

- The `sub` claim is stable; keep `rebac.subjectClaim=sub`.
- Add a *Group Membership* mapper named `groups` to the client scope of the
  UI and API clients. Switch **Full group path** off to get `engineering`
  instead of `/engineering`.
- Realm or client roles can serve as groups: map `/realm_access/roles` or
  `/resource_access/<client>/roles` as shown above.
- Add an *Audience* mapper when the trustlist entry requires an audience.

### Microsoft Entra ID

- **User IDs.** `sub` is pairwise in Entra ID: it differs per application.
  Set `rebac.subjectClaim` to `oid`, the object id of the user in the
  tenant, so that grants match the same person in every client. The BaSyx UI
  shows this value as the user ID.
- **Token version.** The issuer of v1 tokens (`https://sts.windows.net/<tenant>/`)
  differs from v2 tokens (`https://login.microsoftonline.com/<tenant>/v2.0`).
  Set `accessTokenAcceptedVersion` of the API app registration to `2` and
  trust only the v2 issuer.
- **Groups.** Entra ID emits group object ids, not names. App roles are
  usually more practical: define roles such as `engineering` on the API app
  registration, assign groups to them, map `/roles` into the group claim
  (`{ "target": "groups", "mode": "list", "sources": ["/roles"] }`) and set
  `rebac.groupClaim` to `basyx.groups`.
- **Group overage.** If a user belongs to more than 200 groups, Entra ID
  omits the `groups` claim and only links to Microsoft Graph. ReBAC then
  sees no groups for that user. Use app roles, or emit only the groups
  assigned to the application.

### Ory Hydra

- Hydra issues opaque access tokens by default. Set the access token
  strategy to JWT (`strategies.access_token: jwt`).
- Custom claims from the consent session appear under `ext`. Map them, for
  example `{ "target": "groups", "mode": "list", "sources": ["/ext/groups"] }`,
  or allow them as top-level claims (`oauth2.allowed_top_level_claims`).

### Other providers

Check that access tokens are JWTs, which claim is stable per person across
clients, and in which claim and form groups or roles appear. Then configure
the subject claim, the group claim and, if needed, a claim mapping as above.

## Existing data

Resources created before ReBAC was enabled, or through a service without
ReBAC, have no owner and no derivation. They stay accessible exactly as ABAC
allows. To hand one over to its owners, an administrator sets its owners:

- in the BaSyx UI: **Access Management** → **Resources**, open the resource
  by its ID and add owners in the Share dialog, or
- with `PUT /security/rebac/admin/owners/{type}/{base64url id}` and
  `If-Match` (see [Sharing API](sharing-api.md#administration)).

Descriptors and discovery entries created before ReBAC was enabled do not
follow their shell or Submodel. Share them separately if needed.

## Operations

### Startup and reconciliation

While ReBAC is disabled, nothing tracks deletions or removed references.
Every startup with `rebac.enabled=true` therefore removes grants, element
grants, invitations, derivations and links of resources that no longer
exist, before the service accepts requests. The log line `ReBAC enabled`
reports the removed counts. Administrators can run the same step at any time
with `POST /security/rebac/admin/reconcile`. Reconciliation only removes
state and never grants access.

### Disabling ReBAC

With `rebac.enabled=false`, the services ignore all grants and behave as
before; the ReBAC tables remain untouched. Re-enabling restores all grants
and reconciles first. Resources created while ReBAC was disabled have no
owner.

### Backup and restore

All ReBAC state lives in the BaSyx database (`rebac_*` tables and the
`auth_uuid` columns). A database backup contains resources and their access
consistently; restore both together. When WORM evidence is enabled, retain
the latest audit head hash outside the database as well (see below).

### Monitoring

ReBAC uses the service's OpenTelemetry configuration:

| Signal | Meaning |
| --- | --- |
| Span `rebac.decision` | One per decision, with `http.route` and `rebac.outcome` (`granted`, `none`, `uncovered`, `unavailable`) |
| `basyx.rebac.decisions`, `basyx.rebac.decision.duration` | Decisions and their duration by outcome and route |
| `basyx.rebac.access.changes` | Access changes by event type |
| `basyx.rebac.management.denials` | Denied management requests by route |

Alert on:

- **`503 SECURITY-REBAC-UNAVAILABLE`** (log code `SECURITY-REBAC-UNAVAILABLE`,
  outcome `unavailable`). ReBAC could not decide, usually because the
  database failed. BaSyx never falls back to ABAC-only decisions when ReBAC
  was needed, so affected requests fail.
- **`503` on access changes with WORM evidence enabled.** The evidence store
  is unavailable, and access changes are rejected instead of being applied
  unaudited.
- **Unusual `basyx.rebac.management.denials`**, which can indicate probing
  of the management API.

## Audit trail and evidence

Every access change is recorded in `rebac_audit_event` in the transaction
of the change: grant changes, invitations (created, revoked, accepted),
Shell link changes and reconciliations that removed state. Ownership
assigned at creation is part of the resource history instead. Denied
requests are not recorded; they appear in the metrics and logs.

- The events form a hash chain and are append-only while the history guard
  is enabled.
- With history evidence enabled (`history.evidence.*`), each event is
  archived in the WORM store before its transaction commits.
- Administrators review the trail in the BaSyx UI (**Access Management** →
  **Audit trail**) or through the audit API, filtered by resource or by the
  user who made a change.
- **Verify regularly** and keep the reported event ID and head hash outside
  BaSyx. The next verification can start from this checkpoint and only
  check newer events. Passing the head hash as *expected head* detects
  events removed from the end of the trail. The evidence verifier checks
  the whole chain offline:

  ```bash
  historyevidenceverifier -config config.yaml -rebac-audit -expected-head-hash <hash>
  ```

See [NIS2_HISTORY_EVIDENCE.md](../NIS2_HISTORY_EVIDENCE.md) for the evidence
store and retention.

## Troubleshooting

**A user does not see a resource that was shared with them.**

1. Ask the user for their identity from `GET /security/rebac/principal`
   (shown as *User ID* in the UI user menu) and compare issuer, subject and
   groups with the grants of the resource (`GET …/$access`, or **Share** in
   the UI). A grant for another issuer, another claim value or another
   group spelling does not match.
2. Group changes take effect with the next token. The user may need to sign
   in again.
3. Access is per resource. A shared Submodel does not show its shell; a
   shared shell does not share its Submodels unless a Shell link exists.
4. Descriptors created before ReBAC was enabled do not follow their source.

**Lists return `403` for users without shares.** Add the list rules from
[step 3](#3-allow-what-the-policy-must-still-allow).

**Nobody can create resources.** Grant `creator` on the repository family
([step 5](#5-let-people-create-resources)).

**The UI shows no Share entries or no Access Management.** The UI shows
sharing only for services that list the ReBAC profile in `/description`.
Check that ReBAC is enabled, that `/description` is readable, and that the
UI's infrastructure points to the right service URLs. Access Management
also needs a signed-in user.

**A service does not start.** `REBAC-SETUP-*` log codes name the missing
prerequisite; `CONFIG-REBAC-*` codes name an invalid setting.

## Limits

- Repository admins see every resource of their family.
- History, `$recent-changes`, event feeds, `$signed`, `/verify` and
  historical passports stay ABAC-only.
- Query conditions over related resources (`$sm`/`$sme` inside other
  resource queries) only see ABAC-visible related rows.
- Items of `/upload` and of bulk requests are authorized one by one, so an
  upload can be applied partially, as with ABAC.

## Upgrade notes

Database schema `v1.2.2` adds `auth_uuid` columns to `aas`, `submodel`,
`concept_description`, `descriptor`, `aas_identifier` and `aasx_package` and
assigns a UUID to every existing row. PostgreSQL rewrites these tables while
holding an exclusive lock; plan a maintenance window for large
installations. Run the configuration service before upgrading services. The
patch also adds the `rebac_*` tables, which stay empty while ReBAC is
disabled.
