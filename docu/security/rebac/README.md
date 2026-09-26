# Relationship-Based Access Control (ReBAC)

> **Experimental.** The ReBAC integration, its management API and its
> database tables may change incompatibly in future releases. Configuration
> keys carry no experimental marker; this documentation is the only place
> that declares the status.

ReBAC lets people share their own Asset Administration Shells, Submodels,
SubmodelElements, Concept Descriptions, registry descriptors, discovery
entries and AASX packages with other users or groups, without anyone editing
the ABAC policy. Whoever creates a resource becomes its owner, and owners
decide who else may use it.

ReBAC runs next to ABAC as a **strict union**:

```text
access = ABAC allows  OR  ReBAC allows
```

ABAC syntax, evaluator semantics, policy management and existing
configurations are unchanged. With `rebac.enabled=false` (the default) no
ReBAC code is wired into the routers and every service behaves exactly as
before. Relationships are stored in the BaSyx PostgreSQL database and
evaluated there as part of each query. There is no external authorization
service, and all services sharing one database share the same
relationships.

## Which guide do I need?

| You want to | Read |
| --- | --- |
| Try sharing in the BaSyx UI | [examples/BaSyxReBACExample](../../../examples/BaSyxReBACExample/README.md) |
| Set up, operate or troubleshoot ReBAC | [Administration guide](administration.md) |
| Share resources through the API or build a client | [Sharing API](sharing-api.md) |
| Change or extend the implementation | [Developer guide](../../developer/rebac.md) |

## Roles

| Role | Allows |
| --- | --- |
| `viewer` | read |
| `editor` | read, update, create children |
| `executor` | invoke operations and read their status and results (shells, Submodels and SubmodelElements only) |
| `owner` | everything above, plus delete and manage access |

Two more roles apply to a whole repository family (`aas`, `submodel`,
`concept_description`, `aas_descriptor`, `submodel_descriptor`,
`asset_links`, `aasx_package`):

| Repository role | Allows |
| --- | --- |
| `creator` | create top-level resources of the family; the creator becomes their owner |
| `admin` | every right on every resource of the family, and managing its repository grants |

Roles are granted to users or groups of an OIDC issuer. There are no public
or wildcard grants.

## Concepts

- **Ownership.** An authenticated creator becomes owner in the same
  transaction. Owners share, invite and delete.
- **Element grants.** Access can be shared for a single SubmodelElement
  path and its children, without the rest of the Submodel.
- **Shell links.** An owner of both can let the viewers, editors and
  executors of a shell use a Submodel the shell references.
- **Invitation links.** Owners create one-time or limited links that grant a
  role to whoever signs in and accepts them.
- **Derived resources.** Registry descriptors and discovery entries that
  BaSyx synchronizes from shells and Submodels follow the access of their
  source.
- **Audit trail.** Every access change is recorded in a hash-chained,
  append-only trail, optionally archived in WORM storage.

## ReBAC and ABAC interplay (read this first)

A ReBAC grant on a resource is **not limited by ABAC fragment filters, masks
or update conditions** for that resource:

- ABAC rules may hide individual SubmodelElements (for example with a
  `FILTER` on `$sme`) or mask fields. When the owner shares the Submodel with
  a user through ReBAC, that user sees the complete Submodel, including
  elements the ABAC filter would hide.
- ABAC update conditions (formulas on the written state) do not constrain a
  ReBAC editor of the resource.
- ABAC cannot cap what owners share through ReBAC. Anyone who owns or
  manages a resource can share it with any user or group.

Example: the policy lets role `editor` read Submodels with idShort `public`
but hides elements with idShort `secret`. User `userx` has that role.

| Request by `userx` | Result |
| --- | --- |
| `GET /submodels/{public}` | `200`, element `secret` hidden (ABAC) |
| `GET /submodels/{private}` without grant | `403` |
| `GET /submodels/{private}` after the owner granted `viewer` | `200`, **all** elements including `secret` |
| `GET /submodels/{public}` after that grant | `200`, `secret` still hidden (grants are per resource) |

Therefore:

- Keep sensitive content in **separate resources** (separate Submodels or
  SubmodelElements with their own owners) instead of relying on ABAC filters
  inside a resource that will be shared.
- Grant ownership deliberately. Owners can share, invite and delete.
- A grant only widens access to the granted resource. A Submodel grant does
  not make the enclosing shell of a superpath visible, and query conditions
  over related resources (for example `$sm` fields inside an AAS query) are
  still evaluated with ABAC visibility only.

## Covered services and routes

| Service | Covered resources |
| --- | --- |
| AAS Repository | Shells, asset information, thumbnails, Submodel references, Submodel superpaths, `/serialization` |
| Submodel Repository | Submodels and all representations, SubmodelElements, attachments, operations (sync, async, status, results) |
| Concept Description Repository | Concept Descriptions |
| AAS Registry, Submodel Registry, Digital Twin Registry | Shell descriptors including embedded Submodel descriptors, standalone Submodel descriptors, bulk API |
| Discovery | Discovery entries (the asset links of one shell) and lookups |
| AASX File Server | AASX packages, asynchronous uploads |
| AAS Environment | All of the above except AASX packages, plus `/upload` and `/serialization` |
| DPP API | Current state of Digital Product Passports |

The following stay **ABAC-only**; a ReBAC grant never gives access to them:
history endpoints (`$history`), `$recent-changes`, event feeds, `$signed`
representations, `/verify`, historical passports (`/v1/dppsByIdAndDate`),
the ABAC policy management API and the Company Lookup service.
