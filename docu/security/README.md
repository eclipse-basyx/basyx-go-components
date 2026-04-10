# Security Architecture and Flow

This document describes how security is wired in the BaSyx Go components, including the architecture, request flow, and enforcement process. It reflects the current implementation in the codebase.

## High-level architecture

```mermaid
flowchart LR
  Client[Client]

  subgraph Service[BaSyx Service]
    Router[Chi router]
    OIDC[OIDC middleware\nverify issuer/audience + scopes]
    ClaimsMW[Optional claims middleware]
    ABAC[ABAC middleware\naccess model + QueryFilter]
    Ctrl[Controllers]
    Persist[Persistence + SQL builders]
    DB[(PostgreSQL)]
  end

  Rules[Access rules JSON\naccess-rules.json]
  Trust[OIDC trustlist\ntrustlist.json]
  Config[Service config\nconfig.yaml]

  Client --> Router --> OIDC --> ClaimsMW --> ABAC --> Ctrl --> Persist --> DB
  Trust --> OIDC
  Rules --> ABAC
  Config --> Router
  Config --> OIDC
  Config --> ABAC
```

## Request flow

```mermaid
sequenceDiagram
  autonumber
  participant C as Client
  participant R as Router
  participant O as OIDC
  participant M as Claims MW
  participant A as ABAC
  participant H as Controller
  participant P as Persistence
  participant D as PostgreSQL

  C->>R: HTTP request
  R->>O: OIDC middleware
  alt No Bearer token
    alt AllowAnonymous = true
      O-->>R: inject claims sub=anonymous
    else AllowAnonymous = false
      O-->>C: 401 Unauthorized
    end
  else Bearer present
    O->>O: verify issuer + audience
    O->>O: check required scopes
    alt verification failed
      O-->>C: 401 Unauthorized
    else verified
      O-->>R: claims in context
    end
  end

  R->>M: optional claims middleware
  M-->>R: claims enriched

  R->>A: ABAC middleware
  A->>A: map method+route -> rights
  loop for each rule in order
    A->>A: Gate0 ACCESS=DISABLED?
    A->>A: Gate1 rights match
    A->>A: Gate2 attributes satisfied
    A->>A: Gate3 objects/route match
    A->>A: Gate4 formula simplify
  end
  alt no rule matches
    A-->>C: 403 Forbidden
  else rule matches
    alt fully decidable
      A-->>R: allow (no QueryFilter)
    else residual conditions
      A-->>R: allow + QueryFilter
    end
  end

  R->>H: handler
  alt QueryFilter present
    H->>H: enforce on payload or result
  end
  H->>P: build SQL + apply QueryFilter
  P->>D: execute
  D-->>H: data / status
  H-->>C: response
```

## Where security is wired

- OIDC + ABAC middleware is applied in the service entrypoints.
  - AAS Registry: [cmd/aasregistryservice/main.go](cmd/aasregistryservice/main.go)
  - Discovery Service: [cmd/discoveryservice/main.go](cmd/discoveryservice/main.go)
  - Digital Twin Registry: [cmd/digitaltwinregistryservice/main.go](cmd/digitaltwinregistryservice/main.go)
- Core security logic lives in [internal/common/security](internal/common/security).
  - OIDC: [internal/common/security/oidc.go](internal/common/security/oidc.go)
  - ABAC engine: [internal/common/security/abac_engine.go](internal/common/security/abac_engine.go)
  - Route->rights mapping: [internal/common/security/abac_engine_methods.go](internal/common/security/abac_engine_methods.go)
  - Object/route matching: [internal/common/security/abac_engine_objects.go](internal/common/security/abac_engine_objects.go)
  - Attributes handling: [internal/common/security/abac_engine_attributes.go](internal/common/security/abac_engine_attributes.go)
  - Access model materialization: [internal/common/security/abac_engine_materialization.go](internal/common/security/abac_engine_materialization.go)
  - QueryFilter helpers: [internal/common/security/authorize.go](internal/common/security/authorize.go) and [internal/common/security/filter_helpers.go](internal/common/security/filter_helpers.go)

## Enablement rules

- Security is only active when ABAC is enabled in config. If `abac.enabled` is false, no OIDC or ABAC middleware is applied.
  - Example config: [cmd/aasregistryservice/config.yaml](cmd/aasregistryservice/config.yaml)
- OIDC uses the trustlist file to allow issuers and audiences.
  - Example trustlist: [cmd/aasregistryservice/config/trustlist.json](cmd/aasregistryservice/config/trustlist.json)
- Access rules are loaded from the access model JSON.
  - Example rules: [cmd/aasregistryservice/config/access_rules/access-rules.json](cmd/aasregistryservice/config/access_rules/access-rules.json)

## OIDC authentication

- OIDC provider verification uses issuer + audience from the trustlist.
- Required scopes are listed per provider in the trustlist and checked against the `scope` claim.
- If the token is valid, claims are injected into the request context.
- The middleware adds time claims `CLIENTNOW`, `LOCALNOW`, and `UTCNOW` to support time-based ABAC formulas.
- AllowAnonymous is currently enabled by default in `SetupSecurityWithClaimsMiddleware`.

Relevant code:
- [internal/common/security/oidc.go](internal/common/security/oidc.go)
- [internal/common/security/security.go](internal/common/security/security.go)

## ABAC authorization

The ABAC engine evaluates rules in order and either denies, allows, or allows with a QueryFilter.

Evaluation gates:
1. Map HTTP method + route to required rights (deny if no mapping).
   - Rights within one mapping entry are combined using logical OR (example: `PUT -> [CREATE, UPDATE]` means either right is sufficient).
   - Multiple matching mapping entries are also OR alternatives.
2. Check rights in rule ACLs.
3. Check attribute requirements (CLAIM presence or GLOBAL=ANONYMOUS).
4. Match object routes and descriptor objects.
5. Evaluate formula and simplify using claims and globals.

Outcomes:
- No match -> deny.
- Fully decidable true -> allow.
- Residual conditions -> allow + QueryFilter for downstream enforcement.

Relevant code:
- [internal/common/security/abac_engine.go](internal/common/security/abac_engine.go)
- [internal/common/security/abac_engine_methods.go](internal/common/security/abac_engine_methods.go)
- [internal/common/security/abac_engine_objects.go](internal/common/security/abac_engine_objects.go)
- [internal/common/security/abac_engine_attributes.go](internal/common/security/abac_engine_attributes.go)
- [internal/common/security/abac_engine_materialization.go](internal/common/security/abac_engine_materialization.go)

## RIGHT -> Operational Verb -> HTTP method mapping

```mermaid
flowchart LR
  subgraph RIGHTS[RIGHT]
    direction TB
    R_UPDATE[UPDATE]
    R_CREATE[CREATE]
    R_READ[READ]
    R_VIEW[VIEW]
    R_EXECUTE[EXECUTE]
    R_DELETE[DELETE]
  end

  subgraph VERBS[Operational Verb]
    direction TB
    V_PATCH[Patch]
    V_PUT[Put]
    V_POST[Post]
    V_GETALL[GetAll]
    V_GET[Get]
    V_INVOKE[Invoke]
    V_DELETE[Delete]
  end

  subgraph HTTP[HTTP REST Method]
    direction TB
    H_PATCH[PATCH]
    H_PUT[PUT]
    H_POST[POST]
    H_GET[GET]
    H_DELETE[DELETE]
  end

  R_UPDATE --> V_PATCH
  R_UPDATE --> V_PUT
  R_CREATE --> V_PUT
  R_CREATE --> V_POST
  R_READ --> V_GETALL
  R_READ --> V_GET
  R_VIEW --> V_GETALL
  R_EXECUTE --> V_INVOKE
  R_DELETE --> V_DELETE

  V_PATCH --> H_PATCH
  V_PATCH --> H_PUT
  V_PUT --> H_PUT
  V_POST --> H_POST
  V_GETALL --> H_GET
  V_GETALL --> H_POST
  V_GET --> H_GET
  V_INVOKE --> H_POST
  V_DELETE --> H_DELETE
```

Notes:
- Multiple edges into the same HTTP method node indicate different endpoints can use the same HTTP method with different operational verb meaning.
- For each concrete endpoint + HTTP method combination, there is exactly one mapped operational verb.

## QueryFilter propagation

- QueryFilter is stored in request context after ABAC evaluation.
- Controllers can enforce it on payloads or results.
- Persistence helpers apply it to SQL queries and fragment projections.
- QueryFilter carries right-scoped formulas in `FormulasByRight` (for example, separate formulas for `CREATE` and `UPDATE`).
- `SelectPutFormulaByExistence(ctx, dataExists)` switches the active `Formula` for PUT upsert checks (create vs update).

## Formula enforcement gate

- `ShouldEnforceFormula(ctx)` is the single helper used by components to decide if formula-based ABAC checks must run.
- It returns `(false, nil)` when ABAC is disabled or when no `QueryFilter` is present.
- It returns an error when configuration is missing in context.
- It validates the invariant `Formula != nil => len(FormulasByRight) > 0` and returns an error when violated.
- Components must propagate helper errors as internal errors with component-specific error codes.

## Runtime context requirements

- Security-sensitive code paths must use context-aware methods and pass `ctx` through all checks.
- Do not use runtime fallback logic that bypasses context-based security decisions.
- Security-specific work should be scoped inside `if shouldEnforce { ... }` to avoid unnecessary overhead when formula checks are not required.

Relevant code:
- [internal/common/security/authorize.go](internal/common/security/authorize.go)
- [internal/common/security/filter_helpers.go](internal/common/security/filter_helpers.go)

## Claims enrichment

- Digital Twin Registry injects the `Edc-Bpn` header into claims before ABAC.
  - [internal/common/security/edc_bpn.go](internal/common/security/edc_bpn.go)
  - [cmd/digitaltwinregistryservice/main.go](cmd/digitaltwinregistryservice/main.go)

## Access model structure (high level)

Access rules define:
- DEFATTRIBUTES: reusable attribute sets (CLAIM, GLOBAL, or REFERENCE).
- DEFOBJECTS: reusable route or descriptor object sets.
- DEFACLS: reusable rights and attribute bindings.
- DEFFORMULAS: reusable boolean expressions.
- rules: ordered rules that combine ACLs, objects, and formulas.

Validation invariants enforced by the current implementation:
- Rule-level one-of:
  - exactly one of `ACL` or `USEACL`
  - exactly one of `FORMULA` or `USEFORMULA`
  - exactly one of `OBJECTS` or `USEOBJECTS`
- ACL one-of:
  - exactly one of `ATTRIBUTES` or `USEATTRIBUTES`
- Filter validation:
  - `FILTER` (single) and `FILTERLIST` (multiple) are both supported
  - each filter entry must define `FRAGMENT`
  - each filter entry must define exactly one of `CONDITION` or `USEFORMULA`
- Reference resolution:
  - `USEACL`, `USEATTRIBUTES`, `USEFORMULA`, and `USEOBJECTS` are resolved during model materialization at startup
  - unknown references fail fast (`... not found`)
  - `USEOBJECTS` also rejects empty references and circular references
- Parsing strictness:
  - unknown JSON fields are rejected (`DisallowUnknownFields`)
  - object identifiers in `OBJECTS` use the strict `ObjectItem` grammar (ROUTE / IDENTIFIABLE / REFERABLE / FRAGMENT / DESCRIPTOR forms)

Example file:
- [cmd/aasregistryservice/config/access_rules/access-rules.json](cmd/aasregistryservice/config/access_rules/access-rules.json)

## Testing and security environments

- Security-focused tests use dedicated access rules and Keycloak configs under the service-specific security test folders.
  - Example: [internal/aasregistry/security_tests](internal/aasregistry/security_tests)
  - Example: [internal/discoveryservice/security_tests](internal/discoveryservice/security_tests)
- Tests that intentionally run without ABAC enforcement must provide explicit config context with ABAC disabled.
- Production/runtime code must not inject ABAC-disabled fallback config to compensate for missing context.

## Operational checklist

- Enable ABAC in config and set the access model path.
- Configure the trustlist with issuer, audience, and scopes.
- Confirm route-to-rights mapping covers all endpoints used by the service.
- Validate the access rules against the intended claims and objects.
- Restart the service after updating rules (no hot reload).
