# Conditional Requests (ETag, If-Match, If-None-Match)

All BaSyx services that serve single, changeable resources support HTTP
conditional requests as defined in RFC 9110. Clients use them to avoid lost
updates when several clients change the same resource, and to revalidate
cached representations.

Clients opt in by sending the headers. Without `If-Match` or
`If-None-Match`, every request behaves as before; responses only carry an
additional `ETag` header.

## Resources with an entity tag

| Service | Resources |
| --- | --- |
| AAS Repository | shells; asset information, thumbnail and submodel references share the shell's revision; `/shells/{id}/submodels/{smId}/…` use the Submodel's revision |
| Submodel Repository | Submodels; all elements, `$value`, `$metadata`, `$reference`, `$path`, element lists and attachments share the Submodel's revision |
| Concept Description Repository | concept descriptions |
| AAS Registry, Digital Twin Registry | shell descriptors; their submodel descriptors share the shell descriptor's revision, and asset link changes through discovery change it too |
| Submodel Registry | submodel descriptors |
| Discovery, Digital Twin Registry | asset links of a shell (`/lookup/shells/{aasIdentifier}`) |
| AASX File Server | packages (`/packages/{packageId}`) |
| Company Lookup | companies (`/companies/{companyIdentifier}`) |
| DPP API | DPPs and their elements (`/v1/dpps/{dppId}`, `/v1/dpps/{dppId}/elements/…`) |
| AAS Environment | every resource it bundles, with the same revision as the standalone service |

Collections, queries, bulk operations, `/description`, `/serialization`,
`/upload`, `$signed`, `$history`, `$recent-changes`, operation status and
results, and `dppsByIdAndDate` carry no entity tag.

## Entity tags

Every top-level resource has a server-managed revision. It changes in the
same database transaction as any change of the resource or one of its parts.
Revisions are unique and never reused, so a deleted and recreated resource
never gets an entity tag it had before. The revision of a deleted resource
is kept, so deletes without preconditions need no additional database work.
Every service removes the revisions of deleted resources once an hour; only
one service of a database does so at a time.

The server issues two forms of strong entity tags:

| Response | Form | Example |
| --- | --- | --- |
| `GET` of a resource | `"<revision>-<resource>-<representation>"` | `"8123-5c1e7a0d-91d2c3a4b5c6d7e8"` |
| successful `PATCH` and `POST` | `"<revision>-<resource>"` | `"8124-5c1e7a0d"` |

- The `<resource>` part binds the tag to the resource, so a tag of one
  resource never matches another.
- The `<representation>` part is a digest of the returned bytes. Different
  representations (for example `$value`, `level`, `extent` or pagination) and
  different views of callers with different access rights therefore have
  different tags.
- A DPP is made of a shell and its Submodels. Its tag covers the shell, which
  changes when Submodel references change, and all referenced Submodels.
  Changes made through the AAS or Submodel Repository change the DPP's tag.

`PUT` responses carry no entity tag (RFC 9110, section 9.3.4), because the
server normalizes the stored representation. Read the resource again, or use
`PATCH`, to continue with a current tag. `DELETE` responses carry no entity
tag.

## Writes with If-Match

`PUT`, `PATCH`, `POST` below a resource and `DELETE` accept `If-Match`:

- A tag matches when its `<revision>-<resource>` part equals the current
  revision of the target resource. Any tag the server issued for the
  resource or one of its parts is accepted, so a client can read a Submodel
  and then patch one of its elements with the Submodel's tag.
- Weak tags (`W/"…"`) never match.
- `If-Match: *` only requires that the resource exists. It does not protect
  against concurrent changes. For a `PUT` of a part, such as a Submodel
  element, it requires that part.
- If no tag matches, the service answers `412 Precondition Failed` with the
  usual error body and changes nothing. Not-found and authorization errors
  take precedence over `412`.

The precondition is evaluated in the same database transaction as the
change, while the resource's revision is locked. Two clients that write with
the same tag can therefore never both succeed.

```http
GET /submodels/dXJuOmV4YW1wbGU HTTP/1.1

HTTP/1.1 200 OK
ETag: "8123-5c1e7a0d-91d2c3a4b5c6d7e8"

PATCH /submodels/dXJuOmV4YW1wbGU/submodel-elements/Temperature/$value HTTP/1.1
If-Match: "8123-5c1e7a0d-91d2c3a4b5c6d7e8"
Content-Type: application/json

"21.5"

HTTP/1.1 204 No Content
ETag: "8124-5c1e7a0d"
```

## Create-only writes with If-None-Match

`PUT` with `If-None-Match: *` only creates a resource. If the resource
already exists, including when it is created concurrently, the service
answers `412`. For a `PUT` of a part, such as a Submodel element, it
creates the part if it does not exist yet. `If-None-Match` with tags fails when the current revision
matches one of them.

## Reads with If-None-Match and If-Match

- `GET` with `If-None-Match` answers `304 Not Modified` when the current
  representation has one of the listed tags. The request is authorized as
  usual before; a changed view is never answered with `304`.
- `GET` with `If-Match` answers `412` unless one of the tags is the complete
  entity tag of the current representation. The tags returned by `PATCH`
  and `POST` identify a revision, not a representation, and do not match.
- JSON representations larger than 16 MiB are sent without an entity tag.
  Use the tag of a smaller representation of the same resource, such as
  `$metadata`, for writes.
- `HEAD` is not supported.

## Collections

Collections have no entity tag. `If-Match: *` passes, any other `If-Match`
fails with `412`, and `If-None-Match: *` on a write fails with `412`. A
`POST` that creates a resource returns the new resource's entity tag.

## Operation invocation

`invoke` and `invoke-async` accept `If-Match` with the Submodel's tag. The
precondition is checked before the operation is dispatched. It only
guarantees the revision at that moment: the Submodel is not locked during the
operation, and a delegated call cannot be rolled back.

## Requiring If-Match

The service can require `If-Match` for writes to existing resources:

```yaml
server:
    conditionalRequests:
        requireIfMatch: false
```

```env
SERVER_CONDITIONALREQUESTS_REQUIREIFMATCH=false
```

When enabled, writes to existing resources without `If-Match` are answered
with `428 Precondition Required`. Creating a resource, including a create-only
`PUT` with `If-None-Match: *`, needs no `If-Match`. Note that `If-Match: *`
satisfies the requirement without protecting against concurrent changes.

## Browsers

`ETag` is exposed through CORS, and `If-Match` and `If-None-Match` are always
allowed request headers.

## Performance

- Reading a resource reads its revision in the same database snapshot, which
  is one additional indexed lookup. JSON responses are buffered to compute
  the representation digest.
- Every create or change updates the revisions of the changed resources
  with one statement just before commit; the target of a conditional write
  needs three. Deletes without preconditions need none. Concurrent writes to the same resource wait for each other only from
  that point to commit; with history enabled, they are serialized per
  resource already.
- `304` responses save the transfer of the body; the server still reads the
  resource to authorize the request.

## Upgrading

The revisions are introduced by database schema `v1.2.3`. All services that
share a database must be stopped before the schema is updated, including
importers and background jobs, and must be started in the new version
afterwards. A service of an older version that keeps writing would not
update revisions, so other services could accept outdated entity tags.
Resources that existed before the update start with revision `0` until their
first change.
