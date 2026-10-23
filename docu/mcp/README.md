# Integrated MCP

The AAS Environment can expose a deterministic MCP endpoint alongside its REST API. The server contains no LLM or document parser. Documents stay with the client; only extracted drafts and their source references reach BaSyx.

## Enable the endpoint

The endpoint is disabled by default. Run the configuration service to apply schema `v1.2.4` before starting updated services. Enable it in the AAS Environment YAML or override the flag with `BASYX_MCP_ENABLED=true`.

```yaml
mcp:
  enabled: true
  path: /mcp
  allowAnonymous: false
  publicUrl: https://basyx.example.org/mcp
  authorizationServers: [https://identity.example.org/realms/basyx]
  writeMode: disabled
```

Use the existing OIDC trustlist, with a nonempty audience for every trusted issuer. MCP and REST are interfaces to the same BaSyx protected resource. API tools forward the original token only to the in-process BaSyx router. No downstream URLs, agent service accounts or SQL strings are accepted. The public URL must use HTTPS except for a loopback sandbox. Serve the configured URL through the existing reverse proxy.

Without a token, authenticated deployments return HTTP 401 with protected-resource metadata discovery. An invalid token is not treated as anonymous. `allowAnonymous: true` is intended for an explicitly public, read-only environment; it cannot be combined with confirmed writes.

## Profiles and limits

Select a profile using `?profile=analyst` on the endpoint URL or the `Mcp-Profile` header. The default is `default`. Profiles restrict the tool catalogue; all data operations still require the caller's normal REST permissions.

| Profile | Toolsets |
| --- | --- |
| default | discovery, read, docs |
| analyst | default plus query |
| operator | default plus create |

Configure `mcp.profiles.<name>.toolsets` and `denyTools` to restrict a profile further. A denied tool is absent from `tools/list` and cannot be invoked. The default profile has seven tools.

Default budgets are 65,536 response bytes, 1 MiB request bytes, 50 items per page, 1,000 scanned resources, 100 internal calls per tool, 120 requests per minute per subject, eight concurrent tool calls and a 30-second tool deadline. Requests are limited to 64 nested JSON containers. Limits reject work with distinct codes; aggregate scans never return incomplete statistics. Cursors must be followed to finish a paged result.

## Tools, resources and prompts

| Toolset | Tools |
| --- | --- |
| discovery | `find_asset` |
| read | `get_shell`, `list_submodels_of_shell`, `get_submodel_value`, `get_element`, `describe_asset` |
| query | `query_aasql`, `list_read_models`, `run_read_model` |
| create | `list_submodel_templates`, `get_submodel_template`, `validate_aas_draft`, `plan_create_aas`, `apply_plan`, `get_changeset`, `rollback_changeset` |
| docs | `search_docs` |

IDs are always plain text. `get_submodel_value` pages through the value-only element collection. `get_element` takes an idShortPath such as `Measurements.Temperature` or `Measurements[0].Temperature`. Units are included when they are present in authorized metadata. Descriptions compose authorized data and label values with source paths. Concept descriptions are resolved only through the local authorized API; referenced external URLs are never fetched.

Retrieved content is tagged as untrusted data. It never defines tools or server instructions. `ask_asset(shell, question)` is a prompt for the client LLM to use `describe_asset` and cite sources.

`aasql://grammar` documents the running JSON expression dialect; `aasql://fields` derives example field paths from the shared resolver. `query_aasql` takes `target: shells | submodels`, an `aasql` JSON object, optional `dry_run` and a cursor. Dry runs decode and resolve expressions without making API or database calls. They do not prove that data exists or that the caller will be authorized at execution time.

`value_statistics_by_semantic_id` takes `semanticId` and an optional AASQL `condition`, returning count/min/max/avg/sum for visible numeric Property values, grouped by explicit unit. Empty units form a separate, unknown-unit group; no conversion is inferred. Numeric outputs use decimal strings. `count_submodels_by_semantic_id` counts distinct visible submodels by visible semantic ID. No matches produce an empty result list. Free-form SQL is not supported.

`basyx-docs://...` resources embed Markdown and the AAS Environment OpenAPI from the same source build as the binary. `search_docs` returns bounded excerpts and resource references without a network dependency.

## Confirmed AAS creation

Enable `writeMode: confirm` separately. Configure `confirmationClientId` as a public OAuth authorization-code client with PKCE and register `<publicUrl>/callback` as its exact redirect URI. The sandbox supplies a working example.

Operators need provable CREATE, full READ and DELETE permissions on every target: creation, verification and compensation are preflighted without executing resource handlers. Field-dependent grants that cannot establish this contract are rejected. Each actual API call rechecks current permissions, including after approval.

1. Extract a draft in the client and attach document/page references.
2. Select allowlisted templates and call `validate_aas_draft`.
3. Call `plan_create_aas`. Review its immutable proposal at `confirmation_url`.
4. Sign in to the confirmation page as the same issuer/subject, review the data and submit the explicit approval form.
5. Copy its short-lived token into `apply_plan(plan_id, confirm_token)`.
6. Inspect the changeset. To revert, obtain a separate rollback approval and call `rollback_changeset`.

The endpoint uses stateless Streamable HTTP. The hosted approval page is the fallback for clients without elicitation support; no model-callable `confirm=true` tool exists. An API token alone cannot submit the confirmation form: it also requires the authenticated browser session, a same-origin POST and CSRF protection. Confirmation tokens are hashed at rest and bound to subject, issuer, action, proposal hash (including pinned template artifacts) and plan ID. Compare-and-swap consumes them once.

Draft format:

```json
{
  "shell": {"id": "urn:aas:example", "modelType": "AssetAdministrationShell", "assetInformation": {"assetKind": "Instance", "globalAssetId": "asset-1"}},
  "submodels": [],
  "templates": {},
  "sources": [{"document": "datasheet.pdf", "page": 1, "location": "rated power"}]
}
```

The server creates shell references to exactly the submodels in the draft. Identifiers must be unique; existing resources are never overwritten. The client must provide a template slug per submodel ID. L1 verification is strict even when ordinary REST verification is permissive.

## Template artifacts

Set `templateDirectory` and `allowedTemplates`. Each allowlisted slug requires `<slug>.structure.json` and `<slug>.cheatsheet.md`, mounted read-only. Structure nodes use the `submodel_info` 2.x tree fields: `schema_slug`, `schema_version`, `root`, `id_short`, `model_type`, `value_type`, `semantic_id`, `cardinality`, `children`, and list `item_schema`.

The validator checks metamodel validity, mandatory elements, cardinality, types, semantic IDs, nesting and unexpected elements. Unknown cardinalities fail closed when loading artifacts. Qualify artifacts before enabling them; extraction warnings and free-text business rules are not executable semantic validation. L3 semantic plausibility and automatic template recommendation are outside this MVP.

The sandbox artifacts are explicitly labelled demonstration structures. They are not released IDTA templates or certified `submodel_info` outputs.

## Failure and recovery

Each create commits a revision receipt with its resource mutation. Repeated apply calls cannot create duplicate resources. Verification and rollback read from the writer pool even when a replica is configured. Automatic compensation deletes committed resources in reverse order and uses their original revisions. Rollback preflights revisions before deleting anything; a changed resource produces a conflict.

Multi-resource creation is a saga. Database outages, concurrent changes or revoked permissions can prevent cleanup; these are recorded as `recovery_required` or `conflict`, never reported as successful rollback. After a process interruption and execution-lease expiry, the original user can inspect the changeset and approve recovery through rollback. There is no privileged background identity and no bearer-token persistence.

Plan transitions and their agent/tool identity are recorded in `aol_event`; resource receipts carry their own audit fields in `aol_receipt`. This remains available when functional history is disabled. These tables are operational audit records, not a replacement for the optional tamper-evident history/evidence service. Plan/provenance retention must follow the operator's data-retention policy.

See [the sandbox](../../examples/BaSyxMCPExample/README.md) and [the architecture](../developer/mcp_aol_architecture.md).
