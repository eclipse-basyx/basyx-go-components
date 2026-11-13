# Changelog

All notable user-visible and security-relevant changes are documented here.
Changes are collected as reviewed fragments and batched into this file when a
release is created.

Each entry states whether users need to take action and records the security
consequence separately. High-impact entries require an API, configuration,
policy, or deployment update. Low-impact entries do not require migration.

## v1.1.3 (2026-11-12)

Changes since [v1.1.2](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.1.2...v1.1.3).

### Added

* **Low impact** — Add an optional MCP endpoint to the AAS Environment. MCP clients such as Claude Code and Codex can list, read and query shells, submodels, submodel elements and concept descriptions, and, when writes are enabled, create, change and delete them. Tool calls go through the REST API with the caller's token, return ETags for conditional changes, and carry read-only or destructive annotations so clients ask before changing data. ([#774](https://github.com/eclipse-basyx/basyx-go-components/pull/774))
  * **Security:** The endpoint and its write tools are disabled by default. With ABAC enabled it requires a bearer token that the OIDC trustlist accepts for a configured audience, publishes OAuth protected resource metadata for client sign-in, and applies the caller's ABAC and ReBAC permissions to every call. Without ABAC it is unauthenticated, like the REST API.

* **Low impact** — With ReBAC enabled, newly created shells, Submodels, concept descriptions, shell descriptors and Submodel descriptors (including Submodel descriptors created inside a shell descriptor) get administration.creator filled with the authenticated caller when the client sends none. This covers POST, upserts that create, environment uploads and bulk registration. The value is an ExternalReference with one GlobalReference key "<issuer>|<subject>", using rebac.subjectClaim. A client-provided creator is kept, updates never change the field, and synchronized descriptors mirror their shell or Submodel. ([#846](https://github.com/eclipse-basyx/basyx-go-components/pull/846))
  * **Security:** The filled creator is business data and grants no access. It exposes the caller's issuer and rebac.subjectClaim value to everyone who can read the resource; choose a subject claim that contains no personal data you do not want to share.

* **Low impact** — The AAS Environment offers `POST /upload-async`, an asynchronous variant of `POST /upload` with `GET /upload-async/status/{handleId}` and `GET /upload-async/result/{handleId}`, following the semantics of the AASX File Server's asynchronous package upload. The accepted file is staged durably and imported in the background with the same all-or-nothing semantics as `POST /upload`: a failed operation stores nothing, and its result carries the messages the synchronous upload returns. An unsupported format is rejected with 415 before an operation is created, and exhausted execution capacity with 429. ([#875](https://github.com/eclipse-basyx/basyx-go-components/pull/875))
  * **Security:** The operation handle is bound to the uploader. With security enabled, `/upload-async` requires authentication, only the uploader can read the status and result, and the background import is authorized with the uploader's ABAC and ReBAC permissions exactly like `POST /upload`.


### Changed

* **Low impact** — AASX import now logs the warning AASENV-UPLDSUPPL-NOSUPPLREL with the Submodel, idShort path, and part when a File element references a package part that is not related to the AAS specification as aas-suppl. Such parts, for example a package thumbnail reused as product image, are not imported as attachments (IDTA-01005); previously they were skipped without a hint. ([#819](https://github.com/eclipse-basyx/basyx-go-components/pull/819))
  * **Security:** None.

* **High impact** — Writes reject an idShort that does not match the idShort type of the AAS metamodel, in every verification mode: 2 to 128 characters, only letters, digits, "-" and "_", starting with a letter and not ending with "-". Explicit empty idShorts are invalid; an optional idShort may be omitted. This applies to Asset Administration Shells, Submodels, Concept Descriptions, and Submodel elements written with POST, PUT, or PATCH (including $metadata) in the AAS Repository, Submodel Repository, Concept Description Repository, and AAS Environment, including environment uploads, and in the DPP API. Rejected writes answer with 400 and an error code ending in -INVALIDIDSHORT; the DPP API answers with 409 for invalid element keys and with 400 for a content section whose name does not give a valid idShort, for example "1a". Previously, idShorts such as "a;b" or "a b" were stored in off and permissive mode, and on some write paths also in strict mode; clients that URL-encode the idShortPath, as AAS Part 2 requires, then received 404 for these elements (#848). Single-letter idShorts such as "X", which some IDTA Submodel Templates use, are rejected as well. Resources stored earlier with invalid idShorts are not migrated or repaired; delete them and create them again with valid idShorts. ([#863](https://github.com/eclipse-basyx/basyx-go-components/pull/863))
  * **Security:** None.


### Removed

* **Low impact** — The non-standard fileName form field is no longer read by PutFileByPath, PutThumbnail, PostAASXPackage, PutAASXByPackageId, and PostAsyncAASXPackage. As defined by the V3.2 OpenAPI, the file name comes from the filename parameter of the file part's Content-Disposition header; browsers and the BaSyx UI already send it. Attachment and thumbnail uploads are now always streamed. The /upload endpoint of AAS Environment still reads fileName. ([#819](https://github.com/eclipse-basyx/basyx-go-components/pull/819))
  * **Security:** Attachment and thumbnail uploads without a fileName field before the file part, which includes uploads from the BaSyx UI and most HTTP clients, were buffered in memory up to the configured upload size limit per request. They are now streamed to PostgreSQL, which bounds the memory a concurrent upload can occupy.


### Fixed

* **Medium impact** — Apply the configured semantic verification mode to Concept Description creation and replacement in the standalone repository and AAS Environment, including imports. Previously, semantically invalid Concept Descriptions could be stored even with strict verification enabled. Strict mode now returns HTTP 400 without changing the rejected object, permissive mode logs warnings and accepts the object, and off mode skips semantic verification. ([#786](https://github.com/eclipse-basyx/basyx-go-components/pull/786))
  * **Security:** None.

* **Low impact** — Preserve an omitted Submodel idShort through persistence, individual and metadata reads, and metadata updates in the Submodel Repository, AAS Environment, and nested AAS Repository endpoints, as well as JSON, XML, and AASX exports from AAS Environment. Full Submodel replacement now distinguishes an absent idShort from an explicitly supplied empty string, while retaining the configured verification policy and PATCH omission semantics. ([#800](https://github.com/eclipse-basyx/basyx-go-components/pull/800))
  * **Security:** None.

* **Medium impact** — Return only the fields of the Part 2 metadata schemas from the Submodel Repository `$metadata` endpoints of submodel elements. Child elements of collections, lists, entities, and annotated relationships are no longer included, and value fields such as `valueId`, `contentType`, `first`, `second`, `observed`, `entityType`, `globalAssetId`, `specificAssetIds`, and operation variables are left out. The list endpoint therefore returns only top-level elements. Both endpoints read only the requested elements' own rows instead of their subtrees. ([#801](https://github.com/eclipse-basyx/basyx-go-components/pull/801))
  * **Security:** None.

* **Medium impact** — Accept the type-specific fields of the Part 2 metadata schemas in Submodel Repository `PATCH .../submodel-elements/{idShortPath}/$metadata`: `valueType` of Property and Range, `orderRelevant`, `semanticIdListElement`, `typeValueListElement`, and `valueTypeListElement` of SubmodelElementList, and `direction`, `state`, `messageTopic`, `messageBroker`, `lastUpdate`, `minInterval`, and `maxInterval` of BasicEventElement. Fields outside the metadata representation are still rejected. Before the update commits, the persisted element is verified with its unchanged value according to the configured verification mode, together with its members for a SubmodelElementList and with the enclosing list for a list member, so strict mode rejects changes such as a `valueType` that no longer matches the stored value or the list's `valueTypeListElement`. ([#801](https://github.com/eclipse-basyx/basyx-go-components/pull/801))
  * **Security:** None.

* **Medium impact** — Verify writes of SubmodelElementList members against their enclosing list in the Submodel Repository. `PUT` and `PATCH` of a list member, `POST` of a new member into a list, and `PATCH` of a list are now verified before the transaction commits according to the configured verification mode, so strict mode rejects members that violate the list's `typeValueListElement`, `valueTypeListElement`, or `semanticIdListElement` (AASd-107, AASd-108, AASd-109) with HTTP 400 instead of storing an invalid list. Each member write checks only the written member against the list's own attributes, so its cost does not grow with the list size. Writes of a list and of its members are serialized per Submodel, so concurrent writes cannot combine into an invalid list, and verification checks the complete persisted list even when access rules hide some members from the caller, without revealing them in the error. `PATCH` of any submodel element now also verifies the merged result with its unchanged value, not only the request payload. ([#801](https://github.com/eclipse-basyx/basyx-go-components/pull/801))
  * **Security:** None.

* **Low impact** — Omit submodel elements and optional fields without a value from Value-Only ($value) responses of the Submodel Repository, the AAS Repository superpaths, and AAS Environment, instead of emitting null, empty strings, empty arrays, or empty objects. This applies to Submodels, collections, Entity statements, annotations, the submodel element listing, and Operation result arguments. SubmodelElementList members stay in place for index stability, and a list that contains Operations or Capabilities is omitted. Operation result arguments that are not serialized in Value-Only, such as Capabilities, are now omitted instead of failing the invocation. With level=core, direct child collections and lists are returned as {} and []. PATCH $value accepts null for a ReferenceElement so responses can be sent back unchanged. ([#804](https://github.com/eclipse-basyx/basyx-go-components/pull/804))
  * **Security:** None.

* **Low impact** — Return a Blob that was stored without a contentType without one, instead of with an empty contentType string. ([#804](https://github.com/eclipse-basyx/basyx-go-components/pull/804))
  * **Security:** None.

* **Low impact** — Return only the top-level submodel elements, without their children, when a Submodel or the list of Submodels is requested with level=core, including the AAS Repository superpaths. Previously, the children of each top-level element were included as well. ([#804](https://github.com/eclipse-basyx/basyx-go-components/pull/804))
  * **Security:** None.

* **Medium impact** — Give the key of a SubmodelElementList item in $reference responses of the Submodel Repository, the AAS Repository superpaths, and AAS Environment the model type of the item, such as Property or SubmodelElementCollection, instead of SubmodelElementList, as required by AAS Part 1. Every key of the reference now uses the model type of the element it refers to, and the types are read with one query instead of loading every parent element. With ABAC, the $reference of a submodel element is now authorized like a read of the element itself: the formula applies to the element, and the parent elements must be visible, but no longer have to match the formula themselves. ([#815](https://github.com/eclipse-basyx/basyx-go-components/pull/815))
  * **Security:** A caller who may read a submodel element can now also get its $reference when a parent element does not match the ABAC formula. The reference reveals the model types of the parent elements, which a read of the element does not.

* **Medium impact** — Answer Submodel superpaths of the AAS Repository and AAS Environment (/shells/{aasIdentifier}/submodels/{submodelIdentifier} and the routes below it) with the requested Submodel or element instead of HTTP 500 when an ABAC rule restricts Submodels with a formula on $sm or $sme fields. The shell lookup of a superpath is no longer filtered with the Submodel formula. ([#812](https://github.com/eclipse-basyx/basyx-go-components/pull/812))
  * **Security:** None.

* **Medium impact** — Accept file uploads without a file name in PutFileByPath of the Submodel Repository, the AAS Repository superpath, and AAS Environment, and in PutThumbnail, as the V3.2 request body defines the file name as optional. Previously the first such upload was rejected with 400 BINARYCONTENT-FILENAME-EMPTY. A nameless upload keeps the previously stored file name; otherwise it is named after the File element's idShort (file for list items without idShort) or thumbnail, with an extension matching the resolved content type, for example manual.txt. ([#819](https://github.com/eclipse-basyx/basyx-go-components/pull/819))
  * **Security:** None.

* **Medium impact** — PutThumbnail now stores the content type of each upload, taken from the part's Content-Type like PutFileByPath, instead of keeping the content type of the first upload for the lifetime of the shell. DeleteThumbnail clears the content type, so a later upload no longer inherits it, and AASX import passes the thumbnail's declared content type. ([#819](https://github.com/eclipse-basyx/basyx-go-components/pull/819))
  * **Security:** None.

* **Low impact** — Return the latest version from GET /submodels/{submodelIdentifier}/$history and GET /shells/{aasIdentifier}/$history in the Submodel Repository, the AAS Repository, and AAS Environment when the optional date query parameter is omitted, instead of rejecting the request with 422. If the latest version is a deletion, the response is 404. The published OpenAPI now declares date as optional, as in Part 2 V3.2, and names the Submodel operation GetSubmodelVersionByIdAndDate with its V3.2 semantic ID instead of GetSubmodelByIdAndDate. ([#820](https://github.com/eclipse-basyx/basyx-go-components/pull/820))
  * **Security:** None.

* **High impact** — Enforce AASd-117 for submodel elements in every verification mode. An element without idShort, or with an empty idShort, that is not a direct child of a SubmodelElementList is now rejected instead of being stored under an empty idShort path that could not be addressed afterwards. This covers top-level elements, collection values, entity statements, annotations, and operation variable values on POST, PUT, and PATCH of submodel elements and of whole Submodels in the Submodel Repository, the AAS Repository superpath, and AAS Environment including uploads, as well as DPP creation, DPP updates, and DPP data element updates. These services answer with 400 and an error code ending in -MISSINGIDSHORT, unless strict semantic verification already rejects the request with its own error; the DPP endpoints currently answer with 409 instead. Previously, an element without idShort was accepted in every mode when it was POSTed itself or sent in a DPP data element update; all other writes rejected it only in strict mode. List items remain allowed without idShort. Upgrade note: elements stored without idShort by earlier versions are kept, and PATCH requests that do not include them are not affected, but writing them back unchanged, for example by PUTting a Submodel as it was read, is now rejected; add the missing idShorts in that PUT to repair them. ([#842](https://github.com/eclipse-basyx/basyx-go-components/pull/842))
  * **Security:** With ABAC enabled, a submodel element POSTed without idShort, or with an empty one, skipped the check after the insert that verifies that the created element is allowed by the caller's ABAC rules. A caller allowed to add elements only under conditions, for example a certain semanticId or idShort prefix, could therefore store an element that did not meet them, on POST /submodels/{id}/submodel-elements and on POST by path into a collection, entity, or annotated relationship, including the AAS Repository superpath. The element could not be addressed afterwards, so it could only be removed by replacing or deleting the Submodel. Such requests are now rejected and nothing is stored, and the check runs for every created element.

* **Medium impact** — Return the compact JWS from GET /shells/{aasIdentifier}/$signed, GET /submodels/{submodelIdentifier}/$signed and GET /submodels/{submodelIdentifier}/$value/$signed in the AAS Repository, the Submodel Repository and AAS Environment as a text/plain body, as in Part 2 V3.2, instead of as a JSON string in double quotes with application/json. Clients that decoded the response as JSON must read the body as the JWS directly. ([#823](https://github.com/eclipse-basyx/basyx-go-components/pull/823))
  * **Security:** None.

* **Low impact** — Return the paths of the element and of its direct children from GET /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$path with level=core in the Submodel Repository, the AAS Repository superpath and AAS Environment, as in Part 2 V3.2, instead of only the path of the requested element. ([#829](https://github.com/eclipse-basyx/basyx-go-components/pull/829))
  * **Security:** None. The paths of the direct children are filtered by the same ABAC rules as the normal serialization of the element with level=core.

* **Medium impact** — Return an OperationResult with status 200 from GET /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId} and its $value variant in the Submodel Repository and the AAS Repository superpath when an asynchronously invoked operation fails during processing, as required by Part 2 V3.2, instead of 502 with an error Result. executionState is Timeout when clientTimeoutDuration runs out and Failed for every other error, success is false, and messages describe the error. Clients that treated a non-200 result as a failed invocation must check executionState instead. ([#832](https://github.com/eclipse-basyx/basyx-go-components/pull/832))
  * **Security:** None.

* **Medium impact** — Return a Result with only messages from GET /bulk/result/{handleId} in the AAS Registry, the Submodel Registry, the Digital Twin Registry and AAS Environment when a bulk operation fails, as in Part 2 V3.2, instead of a body which also had executionState, success, processedCount, successfulCount, failedCount and details. Each failed request item has one Error message with the item's status code as code, bulk-item-<index> as correlationId and the item's index, identifier and reason in text. Clients that read details or the counters must read messages instead. ([#834](https://github.com/eclipse-basyx/basyx-go-components/pull/834))
  * **Security:** None.

* **Medium impact** — Return the package descriptions of the AASX File Server as in Part 2 V3.2: a description has only packageId and aasIds, which give the identifiers as they are instead of base64url-encoded, and GET /packages gives the paging metadata as paging_metadata instead of pagingMetadata. Each aasIds part of a multipart upload is one identifier, so an identifier with a comma is no longer split; clients that sent several identifiers in one comma-separated part must send one part per identifier. ([#836](https://github.com/eclipse-basyx/basyx-go-components/pull/836))
  * **Security:** None.

* **Low impact** — Send delegated operation invocations of the Submodel Repository through one shared HTTP connection pool instead of a new one per invocation, so connections to a delegation target are reused and idle connections, file descriptors and goroutines no longer grow with the invocation rate. ([#838](https://github.com/eclipse-basyx/basyx-go-components/pull/838))
  * **Security:** None. Every new connection is still checked against SMREPO_DELEGATION_TRUSTED_HOSTS and pinned to a trusted resolved IP before dialing, and pooled connections are dropped when that allowlist changes.

* **Low impact** — GET /serialization of the AAS Environment accepts Accept: application/aas+zip, the AASX media type of the OpenAPI and of Part 5, and answers with an AASX package containing the XML serialization and with the Content-Type application/aas+zip. The previously accepted AASX media types still work. ([#837](https://github.com/eclipse-basyx/basyx-go-components/pull/837))
  * **Security:** None.

* **Medium impact** — Return the content of a File element through GET /shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment as in the OpenAPI, with its Content-Type and Content-Disposition, instead of a JSON object with the base64-encoded content. A File element with an external URL now redirects with a Location header, the same as the Submodel Repository, instead of a JSON body. ([#839](https://github.com/eclipse-basyx/basyx-go-components/pull/839))
  * **Security:** None.

* **Low impact** — GET /packages/{packageId} of the AASX File Server returns every package with the Content-Type application/asset-administration-shell-package, as the OpenAPI defines, instead of application/aasx+xml or application/aasx+json derived from the package's environment. This also applies to packages stored before the update. ([#840](https://github.com/eclipse-basyx/basyx-go-components/pull/840))
  * **Security:** None.

* **Low impact** — Omit empty submodelElements in Submodel Repository list and query responses, including when filtering removes all elements. ([#854](https://github.com/eclipse-basyx/basyx-go-components/pull/854))
  * **Security:** None.

* **High impact** — Correct query and security fragment scope so submodel fields do not mask submodel element fields. Reject filters targeting mandatory submodel IDs or named element idShorts, and restrict general element idShort filters to direct list members. Update affected queries and security policies before upgrading. ([#855](https://github.com/eclipse-basyx/basyx-go-components/pull/855))
  * **Security:** Security fragment filters remain scoped to their intended fields; policies targeting mandatory identifiers are rejected during validation and must be updated.

* **Low impact** — Omit the non-standard top-level createdAt from AAS Registry descriptor creation responses, allowing strict clients to read them. Digital Twin Registry responses retain the timestamp. ([#856](https://github.com/eclipse-basyx/basyx-go-components/pull/856))
  * **Security:** None.

* **Low impact** — Apply Submodel field filters to direct Submodel and metadata reads consistently with list and query responses. Internal Submodel and element PATCH merges preserve fields hidden from the caller so changing another field does not erase them. ([#855](https://github.com/eclipse-basyx/basyx-go-components/pull/855))
  * **Security:** With ABAC enabled direct Submodel reads previously returned idShort and semantic references even when an applicable fragment filter hid them. Direct reads now enforce those filters including when ReBAC is enabled and preserve the configured ReBAC grant semantics.

* **Low impact** — Record complete persisted Submodel, Asset Administration Shell, and registry descriptor state in history snapshots after mutations even when history evidence is disabled. ([#855](https://github.com/eclipse-basyx/basyx-go-components/pull/855))
  * **Security:** Response fragment filters no longer truncate internal history snapshots. Mutation authorization and audit identity remain enforced.

* **Low impact** — Apply Submodel element metadata PATCH requests as selective metadata updates. Omitted values, children, and metadata stay untouched, avoiding stale value overwrites and unnecessary blob rewrites during concurrent updates. Property and Range valueType changes still validate and convert the stored value inside the writer transaction. ([#855](https://github.com/eclipse-basyx/basyx-go-components/pull/855))
  * **Security:** Metadata updates retain current and prospective write authorization checks and atomic history recording. Validation errors for incompatible valueType changes do not disclose hidden stored values.

* **Low impact** — A PUT that renames a File or Operation Submodel element in the Submodel Repository and AAS Environment now stores the File's value and content type and the Operation's variables at the new idShortPath. Previously, renaming a File answered with 404 and renaming an Operation discarded the variables of the request. ([#863](https://github.com/eclipse-basyx/basyx-go-components/pull/863))
  * **Security:** None.

* **Low impact** — Return the HTTP status that the repositories report from the DPP API: invalid input such as an invalid idShort now answers 400 instead of 409, a denied ABAC check 403 instead of 409, an internal error 500 instead of 404 or 409, an exceeded payload limit 413, and an unavailable dependency such as the object store 503. DPP errors without a status from storage, such as a stored DPP that cannot be composed, now answer 500 instead of 404 on reads. ([#874](https://github.com/eclipse-basyx/basyx-go-components/pull/874))
  * **Security:** None. ABAC denials from storage now answer 403, as the AAS Repository already does for the same requests, so no additional information is disclosed.

* **Low impact** — The AAS Environment upload (`POST /upload`) of AASX, JSON and XML files is now all-or-nothing. All Concept Descriptions, Submodels and Asset Administration Shells, the AASX thumbnails and supplementary files, and the registry descriptors synchronized for them are stored in one transaction, together with their history and event outbox entries. If any of them is rejected, nothing from the upload is stored, and the error response names the rejected resource or package part and states that the upload was rolled back. Previously, resources stored before the failing one remained, together with their descriptors. The HTTP status codes of failed uploads are unchanged. ([#875](https://github.com/eclipse-basyx/basyx-go-components/pull/875))
  * **Security:** An upload with a resource denied by ABAC no longer leaves the resources the requester was allowed to write before it, so a rejected upload has no persistent effect.

* **Low impact** — Submodel Repository reads no longer fail with HTTP 500 when an ABAC rule combines a FILTERLIST fragment on `$sm#supplementalSemanticIds[]` or `$sm#semanticId.keys[]` with a formula or fragment condition on `$sm#id` or `$sm#idShort`; the filtered references are now returned as the rule describes. ([#881](https://github.com/eclipse-basyx/basyx-go-components/pull/881))
  * **Security:** None. Affected reads failed closed with HTTP 500 and disclosed no data.

* **Low impact** — AAS Registry and Digital Twin Registry reads of shell and submodel descriptors no longer fail with HTTP 500 when an ABAC FILTERLIST condition uses a shell-level field such as `$aasdesc#id` or `$aasdesc#idShort` on a nested fragment like `$aasdesc#submodelDescriptors[].semanticId.keys[]`, `$aasdesc#submodelDescriptors[].supplementalSemanticIds[]` or `$aasdesc#submodelDescriptors[].endpoints[]`, or on `$aasdesc#endpoints[]` while a shell field mask is active; the condition is now evaluated against the owning AAS descriptor. ([#882](https://github.com/eclipse-basyx/basyx-go-components/pull/882))
  * **Security:** None. Affected reads failed closed with HTTP 500 and disclosed no data.


### Security

* **Medium impact** — With ABAC, Submodel superpaths of the AAS Repository and AAS Environment now also require access to the enclosing shell, checked with the $aas and ROUTE rules of /shells/{aasIdentifier}: READ for every superpath operation, and UPDATE for PUT and DELETE of the Submodel, which add or remove the shell's Submodel reference. A policy that grants superpaths only through $sm, $sme, or /shells/*/submodels/* ROUTE rules needs an additional rule on the shell. ([#812](https://github.com/eclipse-basyx/basyx-go-components/pull/812))
  * **Security:** Previously ABAC did not authorize the shell of a superpath request, so a caller without access to a shell could use its superpaths and learn that the shell exists and references the Submodel. A shell without a matching rule is now answered with 403, and a shell excluded by a formula with 404, as for a direct shell request. This matches the existing ReBAC superpath rules.

* **High impact** — With ABAC enabled, `GET /submodels/{submodelIdentifier}/$history` and `GET /shells/{aasIdentifier}/$history` in the Submodel Repository, AAS Repository and AAS Environment now answer 404 when the caller's access rules restrict the resource with a formula filter or a `FILTER`/`FILTERLIST` fragment filter. Stored versions cannot be filtered, so only callers allowed to read the complete resource receive them. ([#883](https://github.com/eclipse-basyx/basyx-go-components/pull/883))
  * **Security:** Previously, `$history` returned the complete stored version to every caller whose rules granted READ on the route, ignoring formula conditions and fragment filters. A caller could read Submodels and AAS that the direct read hides from them, and fields such as `supplementalSemanticIds` that a FILTERLIST removes. Deployments with `history.mode` set to `api` or `audit` were affected.

* **Medium impact** — With ABAC, the Submodel Element collection reads of the Submodel Repository and AAS Environment (`GET /submodels/{submodelIdentifier}/submodel-elements` and its `$value`, `$metadata`, `$reference` and `$path` variants) now return only the elements that the caller's `$sme` `REFERABLE` grants cover, as the single-element reads do. ([#883](https://github.com/eclipse-basyx/basyx-go-components/pull/883))
  * **Security:** Previously a READ grant on one Submodel Element made these reads return every element of the Submodel, including values.
## v1.1.2 (2026-10-16)

Changes since [v1.1.1](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.1.1...v1.1.2).

### Added

* **Low impact** — Support type-scoped IDENTIFIABLE permissions for AAS Environment upload and serialization, including filtered JSON, XML and AASX exports and CREATE/UPDATE checks based on actual object and binary content existence. ([#723](https://github.com/eclipse-basyx/basyx-go-components/pull/723))
  * **Security:** AAS, Submodel and ConceptDescription grants remain isolated by type. Exports enforce existing formulas, field and element filters, and authorization for attachments and thumbnails; explicitly requested inaccessible objects fail without returning a partial selection. Imports preserve per-object checks, 403 responses and ABAC/ReBAC alternatives. Creating attachment or thumbnail content requires CREATE permission even when its owner exists; replacing stored content requires UPDATE. Earlier authorized writes can remain committed if a later import step is denied.

* **High impact** — All services support HTTP conditional requests for single resources: `GET` returns an `ETag`, `PUT`, `PATCH`, `POST` below a resource and `DELETE` accept `If-Match` and answer `412 Precondition Failed` without changing anything when it does not match, `PUT` accepts `If-None-Match: *` for create-only writes, and `GET` answers `If-None-Match` with `304 Not Modified`. Nested parts share the revision of their top-level resource, a DPP covers its shell and all its Submodels, and the AAS Environment returns the same revisions as the standalone services. The new setting `server.conditionalRequests.requireIfMatch` (default `false`) answers writes of existing resources without `If-Match` with `428 Precondition Required`. CORS always allows `If-Match` and `If-None-Match`. Revisions are stored by database schema `v1.2.3`; all services sharing a database must be stopped before the update and started in the new version, because older services do not update revisions. ([#742](https://github.com/eclipse-basyx/basyx-go-components/pull/742))
  * **Security:** Preconditions are evaluated after the not-found and authorization checks, in the same transaction as the change, so a failed precondition never reveals hidden resources and two writers with the same entity tag never both succeed. `304` is only returned after the request was authorized, and representation tags differ between callers with different views, so a changed view is never answered with `304`. The revision part of an entity tag is the same for all callers and changes when parts change that a caller cannot see. Revisions come from one database-wide sequence and therefore reveal the approximate number of writes between two reads.

* **Low impact** — Services announce history and the Event Feed in `GET /description`. With `history.mode` `api` or `audit`, the AAS Repository lists `https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRepositoryServiceSpecification/SSP-006`, the Submodel Repository lists `https://admin-shell.io/aas/API/3/2/SubmodelRepositoryServiceSpecification/SSP-007`, and the AAS Environment lists both. With the Event Feed enabled, the services hosting it list `https://basyx.org/aas/API/3/2/EventFeed/1.0`. ([#764](https://github.com/eclipse-basyx/basyx-go-components/pull/764))
  * **Security:** None.


### Changed

* **High impact** — Error responses of all services now return the `Result` object defined by the AAS API, `{"messages":[{…}]}`, instead of a bare JSON array of messages. This makes BaSyx work with clients that parse errors as `Result`, such as the Tractus-X SDK. Clients that read the array must read `messages` instead. The Company Lookup OpenAPI document now also defines `Result` as this object. ([#749](https://github.com/eclipse-basyx/basyx-go-components/pull/749))
  * **Security:** None.

* **Low impact** — When the endpoint of a delegated Operation answers with an error status, synchronous and asynchronous invocations, including their `$value` variants, now return `502 Bad Gateway` with a `Result` that names the endpoint's status code. Before, they forwarded the endpoint's status code and response body unchanged. Failed asynchronous results stored by an earlier version are returned as `502 Bad Gateway` with a generic `Result`, unless the Submodel Repository created the stored error itself; then it is returned as a `Result` with UTC timestamps. ([#749](https://github.com/eclipse-basyx/basyx-go-components/pull/749))
  * **Security:** The response body of the delegated Operation endpoint is no longer passed to clients, so internal details of that endpoint, such as error messages or stack traces, are no longer exposed through the Submodel Repository, including for results stored before the update or by instances of an earlier version that share the database.

* **High impact** — ValueOnly (`$value`) serialization uses the JSON types of the IDTA-01001 mapping: numeric `valueType`s are JSON numbers, `xs:boolean` is a JSON boolean and all other types stay strings, for Property values and Range bounds. Numbers keep all stored digits. A Property without a value is `null`; `INF`, `-INF`, `NaN` and stored values that do not match their `valueType` are returned as strings. This also changes `$value/$signed` payloads, DPP compressed content and PCN event payloads; PCN records omit Properties without a value so that they keep matching `pcnNotificationEvent.v1.schema.json`. `PATCH` on `$value` now interprets the payload against the stored elements instead of guessing element types from its shape: numeric types accept JSON numbers and `xs:boolean` accepts JSON booleans, both also accept their lexical form as a string such as `"42"`, and other types require strings. Values that are not valid for their `valueType`, other JSON types and unknown members return `400`, `null` clears a Property value or Range bound, and omitted members stay unchanged. The whole payload is validated before anything is written. Operation invocations with ValueOnly arguments apply the same scalar rules. ([#751](https://github.com/eclipse-basyx/basyx-go-components/pull/751))
  * **Security:** None.

* **High impact** — AAS Environment `GET /serialization` serializes only explicitly requested identifiables. A request without any `aasIds` or `submodelIds` is rejected with `400` instead of exporting every stored AAS and Submodel, and `aasIds` alone no longer adds all stored Submodels. `includeConceptDescriptions` adds only the Concept Descriptions that the serialized Submodels reference directly or indirectly, as IDTA-01002 specifies, instead of all stored Concept Descriptions. This includes semantic IDs, the `semanticIdListElement` inherited by list children, value IDs, and Concept Descriptions referenced by included ones, such as IEC 61360 value list entries. Clients that exported the whole repository without ids must list the AAS and Submodel ids. ([#767](https://github.com/eclipse-basyx/basyx-go-components/pull/767))
  * **Security:** A single request can no longer export the complete repository. Referenced Concept Descriptions that the caller may not read are omitted.


### Fixed

* **Low impact** — Error message `timestamp` values are always in UTC, as the AAS API pattern requires; before, they used the server time zone. `correlationId` is limited to the 128 characters allowed by the schema, and DPP API errors now carry a non-empty `correlationId` as EN 18222 requires. ([#749](https://github.com/eclipse-basyx/basyx-go-components/pull/749))
  * **Security:** None.

* **Low impact** — `POST /query/shell-descriptors` of the AAS Registry and the AAS Environment returns the following pages for a `cursor`; before, every page after the first was empty. ([#761](https://github.com/eclipse-basyx/basyx-go-components/pull/761))
  * **Security:** None.

* **Low impact** — ValueOnly `PATCH` of an Entity no longer fails when it sets `entityType`, and an AnnotatedRelationshipElement update without `first` or `second` no longer stores `null`. Blob content written by a full `PUT`/`PATCH` or by `$value` is now stored in the same encoding as on create, so non-UTF-8 bytes are preserved and base64 payloads are no longer stored as text. The AAS repository `$value` `PATCH` no longer rounds large numbers, ValueOnly output includes the complete `specificAssetIds` of an Entity, and an empty `min` or `max` of a Range with a text `valueType` such as `xs:string` is stored as an empty string instead of being cleared. ([#751](https://github.com/eclipse-basyx/basyx-go-components/pull/751))
  * **Security:** None.

* **Low impact** — All query endpoints (`/query/shells`, `/query/submodels`, `/query/concept-descriptions`, `/query/shell-descriptors` and `/query/submodel-descriptors`, also in the AAS Environment) accept `"$select": "id"` of the AAS Query Language. They return only the identifiers of the matching objects as `result` and set `paging_metadata.resultType` to `Identifier`. Before, `"$select": "id"` was rejected with `400`. Other `$select` values, including the arrays that were accepted and ignored before, are rejected with `400`. The served OpenAPI documents allow identifier results for `/query/shell-descriptors` as for the other query endpoints; the IDTA schema lacks them (admin-shell-io/aas-specs-api#693). ([#762](https://github.com/eclipse-basyx/basyx-go-components/pull/762))
  * **Security:** Identifier results are read with the same access rules as object results, so only identifiers of objects the caller may read are returned.

* **Low impact** — `GET /submodel-descriptors` and `POST /query/submodel-descriptors` of the Submodel Registry apply the access rules and the query condition before the page limit. Before, pages could be short or empty although more results followed. ([#762](https://github.com/eclipse-basyx/basyx-go-components/pull/762))
  * **Security:** The paging cursor is always taken from a Submodel Descriptor that is visible to the caller.

* **Low impact** — AAS Environment `GET /serialization` no longer builds the whole environment in memory. It serializes one AAS, Submodel or Concept Description at a time into bounded staging storage and streams the result, so memory use depends on the largest single identifiable instead of the request or database size. Before, one request could get the service OOM-killed. A JSON, XML or AASX specification larger than `general.aasxMaxPartExpandedSizeBytes`, or a serialization whose referenced Concept Description identifiers exceed an estimated 16 MiB, is rejected with `413`. ([#767](https://github.com/eclipse-basyx/basyx-go-components/pull/767))
  * **Security:** Fixes a denial of service: a single `GET /serialization` request permitted by the route's access rules could exhaust the service memory and get it killed, and concurrent requests made this worse. Memory use is now bounded and oversized serializations are rejected with `413`.

* **Low impact** — With AAS or Submodel registry integration enabled, `DELETE /shells/{aasIdentifier}` and `DELETE /submodels/{submodelIdentifier}` now remove the AAS or Submodel when its registry descriptor is already missing. The missing descriptor counts as already removed and is logged as a warning. Before, the deletion failed with a misleading `404` "AAS Descriptor not found" or "Submodel Descriptor not found", and the AAS or Submodel stayed in the repository until a descriptor was registered again. ([#769](https://github.com/eclipse-basyx/basyx-go-components/pull/769))
  * **Security:** None.
## v1.1.1 (2026-10-08)

Changes since [v1.1.0](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.1.0...v1.1.1).

### Changed

* **High impact** — All paginated endpoints now share one configurable page size policy. `server.pagination.defaultLimit` (default `100`, env `SERVER_PAGINATION_DEFAULT_LIMIT`) is applied when a request omits `limit`, and `server.pagination.maxLimit` (default `1000`, env `SERVER_PAGINATION_MAX_LIMIT`) is the largest accepted `limit`; a larger `limit` is rejected with HTTP 400 instead of being clamped. The policy covers the AAS, Submodel, SubmodelElement, Concept Description, registry, discovery, Company Lookup, AASX File Server, DPP product id search and Event Feed endpoints. The AAS Repository list endpoints no longer return unbounded pages when `limit` is omitted, and the AASX File Server maximum of `500` is replaced by the shared maximum. ([#735](https://github.com/eclipse-basyx/basyx-go-components/pull/735))
  * **Security:** Every paginated endpoint now applies a default page size and enforces a maximum, so a single request can no longer load an arbitrarily large result set into memory. This mitigates memory exhaustion and denial-of-service through oversized or omitted `limit` values.


### Removed

* **High impact** — The setting `eventing.feed.maxPageSize` and its environment variable `BASYX_EVENTING_FEED_MAX_PAGE_SIZE` have been removed. The Event Feed now uses `server.pagination.defaultLimit` and `server.pagination.maxLimit`, so its default and maximum page sizes are `100` and `1000` unless those settings are changed. The removed key is ignored if it is still present in a configuration. ([#735](https://github.com/eclipse-basyx/basyx-go-components/pull/735))
  * **Security:** None.


### Fixed

* **Low impact** — The Digital Twin Registry now reports all supported AAS Registry and Discovery service profiles through GET /description. ([#725](https://github.com/eclipse-basyx/basyx-go-components/pull/725))
  * **Security:** None.

* **Low impact** — The Asset Administration Shell Repository now exposes GET /serialization and returns 501 Not Implemented until serialization is supported. ([#732](https://github.com/eclipse-basyx/basyx-go-components/pull/732))
  * **Security:** None.

* **Low impact** — `server.pagination.maxLimit` must now not exceed `2147483646`. A configured maximum of `2147483647` left no room for the extra row that page queries fetch to detect a following page, so a request with that `limit` overflowed the query limit and failed with HTTP 500 or a panic. Page buffers are also no longer sized from the requested `limit`, so a large `limit` does not reserve memory before any row is read. ([#736](https://github.com/eclipse-basyx/basyx-go-components/pull/736))
  * **Security:** Prevents a configured maximum page size from causing integer overflow in page queries, and from reserving memory proportional to the requested limit before any rows exist.
## v1.1.0 (2026-09-28)

Changes since [v1.0.12](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.12...v1.1.0).

### Added

* **Low impact** — Experimental Event Feed API for AAS, Submodel, Asset, and PCN change notifications, including opt-in configuration, served JSON Schemas with optional Submodel semantic IDs, the feed_events schema migration, and an interactive example covered by CI smoke tests. ([#647](https://github.com/eclipse-basyx/basyx-go-components/pull/647))
  * **Security:** Event Feed inherits OIDC and ABAC. Records require access to the referenced model and all AAS ownership data disclosed by the payload. Remaining query filters or unknown ownership fail closed.

* **High impact** — Experimental MQTT 5 CloudEvents publishing for existing AAS, asset, Submodel and PCN mutations, independently of the HTTP feed and history, with a transactional per-sink outbox, retries, ordering, TLS authentication and telemetry. Apply database schema v1.2.1 before upgrading services. ([#678](https://github.com/eclipse-basyx/basyx-go-components/pull/678))
  * **Security:** MQTT supports authenticated, verified TLS connections. Broker ACLs govern subscribers; HTTP caller-specific ABAC remains on the feed. Effective configuration output omits credentials.

* **Low impact** — Add experimental Kafka CloudEvents eventing with a Kafka playground, using the existing event contract and outbox. ([#679](https://github.com/eclipse-basyx/basyx-go-components/pull/679))
  * **Security:** Kafka supports TLS, mutual TLS and SASL authentication. Broker ACLs control consumer access; HTTP authorization does not filter Kafka subscribers.

* **Low impact** — Add experimental AMQP 1.0 CloudEvents eventing with a RabbitMQ playground using the existing event contract and outbox. ([#680](https://github.com/eclipse-basyx/basyx-go-components/pull/680))
  * **Security:** AMQP supports TLS and mutual TLS with optional SASL PLAIN authentication. Broker ACLs control subscriber access; HTTP authorization does not filter AMQP subscribers.

* **Low impact** — Implement value-only synchronous and asynchronous operation invocation and result retrieval for the Submodel and AAS Repository APIs, with typed numeric and boolean values, empty structured arguments, shared persistent async handles, and delegated-operation example coverage. ([#685](https://github.com/eclipse-basyx/basyx-go-components/pull/685))
  * **Security:** None. Existing OIDC, ABAC, async-job owner and path validation, and request security contexts are preserved.

* **Low impact** — Configurable response size limit for delegated Submodel Operations, allowing larger synchronous and asynchronous results while retaining the 1 MiB default. ([#707](https://github.com/eclipse-basyx/basyx-go-components/pull/707))
  * **Security:** Delegated responses remain size-limited, and invalid limits are rejected at startup. Raising the limit increases peak memory use per request.

* **Low impact** — Experimental relationship-based access control (ReBAC) for the AAS, Submodel and Concept Description repositories, the AAS and Submodel registries, the Digital Twin Registry, discovery, the AASX file server, the AAS Environment and the current state of Digital Product Passports. When `rebac.enabled` is set, access is granted when ABAC or ReBAC allows it. Creators own new resources, and owners share Shells, Submodels, single SubmodelElement paths, Concept Descriptions, descriptors, discovery entries and AASX packages with users and groups as viewer, editor, executor or owner through `$access` sub-resources with optimistic concurrency (`If-Match`). Further features are approved AAS-to-Submodel inheritance links, invitation links, and administrator bootstrap and recovery. Users are identified by issuer and the claim configured in `rebac.subjectClaim` (default `sub`, for example `oid` for Microsoft Entra ID), and `GET /security/rebac/principal` returns the identity grants are matched against. Descriptors synchronized from Shells and Submodels, and discovery entries created by the discovery integration, inherit the access of their source. Relationships are stored and evaluated in the BaSyx PostgreSQL database, including lists, `/serialization`, `/upload` and the registry bulk API. Services with ReBAC enabled announce the profile `https://basyx.org/aas/API/3/2/RelationshipBasedAccessControl/1.0` in `/description`, and CORS responses of all services expose the `ETag` and `Location` headers so web clients such as the BaSyx UI can manage access. ReBAC is disabled by default and otherwise leaves all existing behavior unchanged. The example `examples/BaSyxReBACExample` includes the BaSyx UI. See docu/security/rebac/README.md. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** ReBAC is only evaluated when ABAC does not allow unconditionally; if it cannot decide, requests fail with 503 instead of falling back to ABAC-only decisions. A ReBAC grant widens only the granted resource and, by design, is not limited by ABAC fragment filters or update conditions of that resource, so administrators should keep sensitive content in separate resources. Denials without grants look exactly like ABAC denials, management routes answer 404 to non-managers, and anonymous callers stay ABAC-only. Grants are re-evaluated by every query, so revocations take effect at commit, also for queued asynchronous work. Matching identifiers never grant access; only resources created by synchronization inherit from their source. Invitation tokens are stored only as SHA-256 hashes and accepted in the request body. Access changes check the permission of the caller again while the object is locked, so a revocation always wins over a pending change, and access ETags are bound to their object. Upserts enforce the create or update right for what they actually write, so a creation right never replaces a resource created concurrently by someone else. `/$access/effective` answers only for confirmed rights and treats conditional ABAC rights as unconfirmed.

* **Low impact** — With ReBAC enabled, every access change (grants, invitations, inheritance links and reconciliations) is recorded in a hash-chained, append-only audit trail. With history evidence enabled, each audit event is archived in the WORM evidence store. Administrators page through the trail newest first through `/security/rebac/admin/audit`, filtered by resource or by the user who made the change, and each event names its resource while it exists. `/security/rebac/admin/audit/verify` verifies the chain in ranges and from checkpoints (event id and hash) retained outside the database, and `historyevidenceverifier -rebac-audit` verifies it offline against an independently retained head hash. ReBAC decisions and access changes are reported as OpenTelemetry spans and metrics. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** Access changes are attributable and tamper-evident. With history evidence enabled, a change fails with 503 instead of being applied without its WORM audit record. The audit trail is readable by configured ReBAC administrators only.


### Changed

* **High impact** — Database schema v1.2.2 adds a persistent `auth_uuid` to `aas`, `submodel`, `concept_description`, `descriptor`, `aas_identifier` and `aasx_package`, and adds the `rebac_*` tables. Applying the patch rewrites these tables under an exclusive lock; plan a maintenance window for large installations and apply schema v1.2.2 with the configuration service before upgrading services. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** None.

* **Low impact** — `PATCH /submodels/{submodelIdentifier}` and `PUT` on a SubmodelElementCollection, SubmodelElementList, Entity or AnnotatedRelationshipElement now write only the differences to the stored state, like Submodel `PUT` already does. The Submodel and unchanged elements keep their database rows instead of being deleted and inserted again, which reduces write load for large Submodels. `PATCH` with `statements` or `annotations` also reconciles the child elements. Removing many elements in one write no longer takes time quadratic in their number, neither when planning nor in the database. ([#716](https://github.com/eclipse-basyx/basyx-go-components/pull/716))
  * **Security:** The ReBAC authorization identity of a Submodel stays on its row instead of being copied during a replacement, so grants on the Submodel and on element paths are kept by construction. ABAC checks of the existing and the prospective state are unchanged.

* **Low impact** — The secured examples and the integration test setups now use Keycloak 26.7.4 instead of 26.0.6. Their realm imports only contain the BaSyx-specific clients, roles, groups, users and user profile attributes, so Keycloak supplies its current built-in defaults. The Compose files use `KC_BOOTSTRAP_ADMIN_USERNAME` and `KC_BOOTSTRAP_ADMIN_PASSWORD` and no longer set removed hostname v1 options. Existing example Keycloak databases are migrated by Keycloak on startup; an already imported realm is not re-imported. ([#717](https://github.com/eclipse-basyx/basyx-go-components/pull/717))
  * **Security:** Moves the example and test identity provider from Keycloak 26.0.6 to 26.7.4, which includes the upstream security fixes released in between. The committed realm files no longer contain realm signing and encryption private keys, so each fresh import generates its own keys instead of reusing keys published in the repository.

* **High impact** — Signed-in callers without ReBAC grants receive empty lists instead of `403` on the list routes ReBAC covers, so ABAC policies no longer need list rules for them. The ReBAC example and the administration guide drop their list rules, which compared identifiers with a placeholder value. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** The removed placeholder rules exposed every resource created with the placeholder identifier to all signed-in users, including its content; policies that copied them should remove them. Anonymous callers and single resources keep the ABAC denial, and backends without formula support, such as the AASX file server, return no row to callers without grants.

* **Low impact** — Lists that only ReBAC grants make visible are read from the granted rows instead of testing every row of the table. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** None.


### Removed

* **Low impact** — The Digital Twin Registry does not offer ReBAC: it authorizes with ABAC only, ignores `rebac.*` settings, and neither provides, documents nor announces the ReBAC management API or profile. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** ReBAC grants no longer give access to the Digital Twin Registry; ABAC alone decides there.


### Fixed

* **High impact** — AAS Environment and Submodel Repository writes reject semantically invalid boolean and temporal Property or Range values with HTTP 400 instead of exposing PostgreSQL conversion errors as HTTP 500 responses. ([#649](https://github.com/eclipse-basyx/basyx-go-components/pull/649))
  * **Security:** None.

* **Low impact** — Reuse PostgreSQL plans for unrestricted shell descriptor updates to reduce repeated query planning on the writer database. ([#669](https://github.com/eclipse-basyx/basyx-go-components/pull/669))
  * **Security:** None. Existing authorization checks and writer transaction consistency are preserved.

* **Low impact** — Correct registry synchronization descriptor interface to use API version 3.2. ([#670](https://github.com/eclipse-basyx/basyx-go-components/pull/670))
  * **Security:** None.

* **Low impact** — Run registry benchmark compose commands with literal arguments and report process-launch failures with BENCH-RUN-EXEC. ([#681](https://github.com/eclipse-basyx/basyx-go-components/pull/681))
  * **Security:** Benchmark hardening: compose paths are passed as literal arguments without shell interpretation; no demonstrated production vulnerability.

* **High impact** — Align encoded API parameters and exact reference and asset filtering across registries, repositories, Discovery, DTR and AAS Environment. Accept valid padded and unpadded base64url for Tractus-X conformance and require nonempty supplied pagination parameters; standardize query and path cursors without legacy fallback. Use the standalone registry and discovery contracts in the joined AAS Environment OpenAPI, keeping DTR extensions exclusive to the DTR specification. Correct DTR asset-link schema references and Company Lookup reference, required-name, and error-response schemas; require clientTimeoutDuration only for asynchronous value-only invocation and return HTTP 400 when missing. ([#683](https://github.com/eclipse-basyx/basyx-go-components/pull/683))
  * **Security:** Rejects malformed API parameters and prevents reference filters from matching supplemental semantic IDs hidden by collection-level authorization masks.

* **High impact** — Require ID-based DPP lookup to match both the owning AAS identifier and its DppMetadata.digitalProductPassportId; a requested ID returns 404 when either stored value differs. Historical lookup applies the same check, while product-based retrieval remains available through uniqueProductIdentifier. Generate a bounded, valid DPP AAS idShort independently of potentially long or punctuated identifiers while preserving the exact identifier as the AAS ID. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Preserves request authorization and applies metadata value visibility before returning product-search DPP IDs or pagination cursors; mismatched AAS and metadata IDs return 404 for ID-based requests. The generated idShort is non-authoritative display metadata; authorization and lookup continue to use the exact AAS/DPP identifier.

* **High impact** — Download attachments with missing filenames using the managed reference or file path, and return 404 when attachment bytes are absent. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Normalizes fallback download filenames.

* **High impact** — Apply DPP merge patches only to affected resources, preserving existing AAS and Submodel identities and metadata. Reuse the loaded passport state while preparing updates and reject generated-ID collisions when creating new sections. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Prevents unintended overwrite or deletion of unrelated and shared Submodels during DPP updates.

* **High impact** — Delete a DPP's owning AAS, DppMetadata and their descriptors while retaining all content Submodels, their descriptors and managed attachments. Removing a content section also retains its Submodel and attachments. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Revalidates the matching DPP ID and ownership under transaction row locks before deletion while preserving request authorization; content Submodels and attachments are retained instead of being removed as a side effect of DPP deletion or section removal.

* **High impact** — Preserve collection field names and shapes in compressed DPP representations while converting only typed multilingual properties and files. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** None. Serialization remains subject to the existing authorization and field-visibility checks.

* **High impact** — Include DPP content only for Submodels whose semantic IDs are declared in contentSpecificationIds, with empty or absent declarations selecting no content. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Prevents undeclared AAS Submodels from being exposed through current, historical, full, compressed, or fine-grained DPP reads.

* **High impact** — Serialize AAS Entity elements in full DPP read representations as EN 18223 DataElementCollection values, preserving entity identity, specific asset references, nested statements, multilingual values, and file resources. Unsupported full element conversions now return 422 instead of being reported as missing DPPs. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Existing authorization and content-selection filtering remain unchanged; the fix only corrects authorized DPP read serialization and error classification.

* **Low impact** — Pull current SNAPSHOT images when starting the MQTT, Kafka, and AMQP examples, avoiding startup failures caused by outdated cached images. ([#698](https://github.com/eclipse-basyx/basyx-go-components/pull/698))
  * **Security:** None.

* **Low impact** — Declare AAS Environment health checks in the Event Feed, MQTT, Kafka, and AMQP examples so Podman Compose can start their dependent Web UI services. ([#699](https://github.com/eclipse-basyx/basyx-go-components/pull/699))
  * **Security:** None.

* **Low impact** — Allow Submodel creation with field-based ABAC CREATE rules when Submodel Registry synchronization is enabled. ([#700](https://github.com/eclipse-basyx/basyx-go-components/pull/700))
  * **Security:** Submodel CREATE authorization remains enforced; only the internal descriptor synchronization excludes the Submodel-specific field filter.

* **Low impact** — The guarded History Audit example and the Submodel repository benchmark start again: MinIO no longer publishes container images, so they now use pinned community builds of MinIO and its client (`pgsty/minio`, `pgsty/mc`) with unchanged object lock settings. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** The WORM evidence store of the example keeps compliance-mode object lock. The images are pinned to release tags instead of `latest`.

* **Low impact** — `PATCH` on a SubmodelElementCollection or SubmodelElementList with a `value` now replaces the child elements with the given ones: omitted children are deleted and `"value": []` removes all children. Before, the given children were inserted next to the existing ones, which failed with a conflict, and an empty array changed nothing. Clients that sent only new children or an empty array must now send the complete list. A `$metadata` `PATCH` on a container with children no longer fails, and a `PATCH` without `value`, `statements` or `annotations` leaves the child elements untouched. ([#716](https://github.com/eclipse-basyx/basyx-go-components/pull/716))
  * **Security:** A `PATCH` without child elements no longer rewrites the children of an Entity or AnnotatedRelationshipElement from the caller's filtered view, so children the caller cannot see are no longer removed.

* **Low impact** — A Submodel `PUT` no longer fails with a conflict when an element was deleted from a SubmodelElementCollection, Entity or AnnotatedRelationshipElement before and the new state adds elements to it. Reconciliation now compares against the stored sibling positions instead of assuming gapless positions. ([#716](https://github.com/eclipse-basyx/basyx-go-components/pull/716))
  * **Security:** None.

* **Low impact** — The OpenAPI specification of the Company Lookup service no longer documents ReBAC endpoints when `rebac.enabled` is set; the service does not offer ReBAC. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** None.

* **Medium impact** — Allow Submodel deletion and descriptor-changing Submodel updates (PUT, PATCH, PATCH $metadata) with field-based ABAC rules when Submodel Registry synchronization is enabled; these requests previously failed with HTTP 500. ([#719](https://github.com/eclipse-basyx/basyx-go-components/pull/719))
  * **Security:** Submodel and AAS mutation authorization remains enforced by the repositories; internal descriptor synchronization no longer evaluates the caller's repository-specific field filter against registry tables.


### Security

* **Low impact** — An AASX package `PUT` no longer replaces an existing package with only the right to create packages, or creates one with only the right to update packages. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** Callers with only one of the create and update rights could write packages beyond that right.
## v1.0.12 (2026-09-10)

Changes since [v1.0.11](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.11...v1.0.12).

### Added

* **High impact** — Synchronize DPP API writes with AAS and Submodel registries and product discovery. ([#650](https://github.com/eclipse-basyx/basyx-go-components/pull/650))
  * **Security:** DPP write authorization now also covers the configured internal AAS Registry, Submodel Registry, and Discovery mutations; their independent read policies continue to control visibility.

* **Low impact** — AAS Environment Service shell queries can evaluate `$sm` and `$sme` field conditions against Submodels referenced by each Asset Administration Shell, with logical `$match` correlating all enclosed conditions to the same referenced Submodel hierarchy. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Cross-resource query fields use the authorization view of their corresponding semantic resource.

* **Low impact** — Querying the text of a MultiLanguageProperty ([#659](https://github.com/eclipse-basyx/basyx-go-components/pull/659))
  * **Security:** None

* **Low impact** — The AASX File Server now supports durable asynchronous package uploads through `POST /packages-async`. ([#609](https://github.com/eclipse-basyx/basyx-go-components/pull/609))
  * **Security:** Async uploads use existing create/read authorization; status and results are restricted to the submitting owner.


### Changed

* **High impact** — ABAC claims retain their JWT JSON types. `CLAIM` selects a top-level value and `CLAIMPATH` selects a nested value using a non-empty RFC 6901 JSON Pointer. Every declared claim must be present before its rule can become active; values are resolved and type-checked when formulas evaluate them. Uncast scalar comparisons require strings; claim casts accept only the corresponding JSON type. Exact string-array membership uses `$contains` with `CLAIMPATH` first. The branch-only `$in` operator has been removed, so policies must migrate from `{"$in":[scalar,CLAIMPATH]}` to `{"$contains":[CLAIMPATH,scalar]}`. Formula evaluation uses true/false/indeterminate logic, and CREATE/UPDATE checks are performed on staged state before commit, with UPDATE requiring both current and prospective state. ([#616](https://github.com/eclipse-basyx/basyx-go-components/pull/616))
  * **Security:** Claim evaluation now follows the documented JSON types and paths. Missing declared claims make the rule ineligible. Present `null`, wrong-type, object, mixed-array, and unsupported values are indeterminate and fail closed when evaluated; empty string arrays evaluate false.

* **High impact** — Expose AAS-managed File attachments as AAS Environment attachment URLs in DPP representations; GENERAL_EXTERNALURL is required to serialize these URLs even when registry synchronization is disabled. ([#650](https://github.com/eclipse-basyx/basyx-go-components/pull/650))
  * **Security:** None.

* **Low impact** — Query-capable repository, registry, and discovery endpoints now represent caller conditions, public list selectors, projections, and authorization constraints in an immutable request-scoped semantic access-view intermediate representation. Responses for authorized resources remain compatible. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** The policy version, claims, trusted global attributes, and per-resource access views remain consistent throughout each request.

* **High impact** — Database schema `v1.1.19` requires PostgreSQL 16 or newer and installs `basyx_safe_regex_pattern` for total regular-expression evaluation and `basyx_validated_cast_input` for safe nested query casts. Run the configuration service before starting updated services. Deployments using an older PostgreSQL version must upgrade before applying this schema patch. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Invalid regular-expression and cast inputs use consistent no-match semantics. Nested casts validate and convert the same textual value without duplicating inner SQL expressions.


### Fixed

* **Low impact** — Invalid ABAC claim comparisons now retain their indeterminate result through `$not`, `$and`, `$or`, and `$match` instead of being converted to Boolean values. ([#616](https://github.com/eclipse-basyx/basyx-go-components/pull/616))
  * **Security:** Invalid or unusable claim values now consistently fail closed.

* **Low impact** — Reduce database round trips and redundant root writes when updating AAS and Submodel Descriptors. ([#658](https://github.com/eclipse-basyx/basyx-go-components/pull/658))
  * **Security:** None.

* **Low impact** — Query conditions now evaluate fields and related resources through their effective semantic access view, including negated conditions, nested `$match`, response filters, list selectors, casts, and regular expressions. AAS object grants no longer authorize nested Submodel data, Fragment objects constrain exact SME fields, and list selectors are emitted only once. Incompatible cast values and invalid regular expressions return no match without hiding PostgreSQL's regex operator from index planning, and Basic Discovery global asset identifier lookups use the Basic Discovery semantic field. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Caller and policy expressions now use consistent per-resource authorization and query-evaluation semantics. Nested AAS Environment Submodel routes require dedicated Submodel or Submodel Element grants.

* **High impact** — Nested query casts preserve authorization checks without exponential SQL expansion or unsupported PostgreSQL casts. Policy routes containing parent segments derive query permissions from the same mounted path as direct authorization. Queries and policy expressions now enforce 64 JSON container levels and 8,192 JSON tokens before recursive decoding. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Bounds expression complexity and SQL expansion, prevents nested cast database errors, and prevents semantic permissions from escaping the configured route mount. Existing oversized expressions must be reduced.

* **High impact** — Compiled query policies are copied structurally so many individually valid access rules retain their grants when combined. External query and policy-expression limits remain enforced. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Prevents valid combined allow rules from silently becoming deny-all while preserving fragment scope, indeterminate conditions, and request policy isolation.
## v1.0.11 (2026-08-31)

Changes since [v1.0.10](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.10...v1.0.11).

### Changed

* **Low impact** — AAS create and delete operations avoid separate root and lookup executions for batches within the existing size limits, reducing writer connection occupation under concurrent load. ([#645](https://github.com/eclipse-basyx/basyx-go-components/pull/645))
  * **Security:** Existing duplicate visibility, ABAC, history, row-locking, not-found, and managed-thumbnail cleanup behavior remains unchanged.

* **Low impact** — Shell descriptor creates avoid rewriting newly inserted embedded Submodel descriptors while synchronizing administration timestamps. ([#646](https://github.com/eclipse-basyx/basyx-go-components/pull/646))
  * **Security:** Descriptor validation, authorization, history, and stored graph semantics remain unchanged.

* **Low impact** — Nested SubmodelElement creates combine child position, duplicate, and identifier allocation state into one database statement. ([#646](https://github.com/eclipse-basyx/basyx-go-components/pull/646))
  * **Security:** Existing parent locking and ABAC-aware duplicate visibility checks remain unchanged.


### Fixed

* **Low impact** — The release workflow can inspect its newly created draft release and continue directly into artifact publication. ([#637](https://github.com/eclipse-basyx/basyx-go-components/pull/637))
  * **Security:** The guard receives contents write permission only because GitHub requires push access to view drafts; its operations remain read-only.

* **Low impact** — Docker release verification now authenticates every Docker Hub access and avoids redundant multi-platform registry reads that could trigger rate limits. ([#638](https://github.com/eclipse-basyx/basyx-go-components/pull/638))
  * **Security:** Docker Hub credentials remain limited to release jobs and are used only for registry operations; immutable digest, signature, attestation, and image-label verification remain enforced.

* **Low impact** — Existing Submodel PUT requests reuse generic PostgreSQL plans while loading persisted SubmodelElement state, avoiding repeated planning overhead on large databases. ([#640](https://github.com/eclipse-basyx/basyx-go-components/pull/640))
  * **Security:** Authorization and transaction isolation remain unchanged; persisted replacement semantics are unchanged.

* **Low impact** — Registry PUT operations preserve descriptor root rows and rewrite only changed descriptor fields and child collections. ([#641](https://github.com/eclipse-basyx/basyx-go-components/pull/641))
  * **Security:** Existing pre-update and post-update ABAC checks remain in place; restricted reads use a complete internal snapshot only after authorization succeeds.

* **Low impact** — Concept Description PUT operations update existing rows in place and reject mismatching path and body identifiers. ([#642](https://github.com/eclipse-basyx/basyx-go-components/pull/642))
  * **Security:** Prevents PUT authorization and history from being split across different Concept Description identifiers.

* **Low impact** — SubmodelElement writes no longer maintain a duplicate path index already covered by the stable path-order index. ([#643](https://github.com/eclipse-basyx/basyx-go-components/pull/643))
  * **Security:** Submodel persistence and authorization behavior remain unchanged; the retained index covers the same Submodel and idShort-path lookup prefix.

* **Low impact** — The configuration service skips the baseline schema for initialized databases, preventing later configuration runs from recreating objects removed by versioned patches. ([#644](https://github.com/eclipse-basyx/basyx-go-components/pull/644))
  * **Security:** None.
## v1.0.10 (2026-08-29)

Changes since [v1.0.9](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.9...v1.0.10).

### Changed

* **Low impact** — Every declared `CLAIM` is mandatory, including when combined with `GLOBAL=ANONYMOUS`; unsupported attributes fail closed. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** More restrictive: missing claims and unsupported attributes deny access.

* **Low impact** — Anonymous time-based rules can grant access when `GLOBAL=ANONYMOUS` is explicitly configured. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** Potentially broader, but only for policies that explicitly allow anonymous access.

* **Low impact** — Existing AAS and Submodel `PUT` requests now reconcile only changed persisted rows, preserve unchanged persistence identities and managed binary references, and synchronize registries only when the derived descriptor content changes. Semantic no-op replacements skip the live-model write but still record the acknowledged update when history or WORM evidence is enabled. ([#591](https://github.com/eclipse-basyx/basyx-go-components/pull/591))
  * **Security:** Improves audit completeness for acknowledged no-op `PUT` requests; authorization semantics are unchanged.

* **Low impact** — AAS Repository, Submodel Repository, and registry `getAll` endpoints now use bounded, set-based database reads whose query count is independent of the requested page size. Pagination, ordering, representations, and filters remain compatible. ([#606](https://github.com/eclipse-basyx/basyx-go-components/pull/606))
  * **Security:** Authorization behavior is unchanged; existing ABAC filters and fragment masks remain enforced.


### Fixed

* **High impact** — ABAC rules containing only `UTCNOW`, `LOCALNOW`, or `CLIENTNOW` now require a `CLAIM`, or `GLOBAL=ANONYMOUS` for intentionally public access. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** More restrictive: existing time-only rules no longer grant access.

* **High impact** — `CLIENTNOW` is no longer generated or overwritten by the server and must come from the verified access token. Use `UTCNOW` or `LOCALNOW` when server time is intended. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** More restrictive: rules requiring `CLIENTNOW` no longer match without the token claim.

* **Low impact** — `UTCNOW` and `LOCALNOW` now use the trusted server clock instead of same-named JWT claims. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** JWT values can no longer control server-time authorization decisions.

* **Low impact** — Concurrent top-level and nested SubmodelElement `POST` requests now allocate unique sibling positions without deadlocking or timing out on a second database connection. These writes honor request cancellation, and nested inserts persist the correct tree depth. ([#599](https://github.com/eclipse-basyx/basyx-go-components/pull/599))
  * **Security:** Authorization and information-disclosure behavior are unchanged.
## v1.0.9 (2026-08-04)

See the [v1.0.9 GitHub release](https://github.com/eclipse-basyx/basyx-go-components/releases/tag/v1.0.9).
