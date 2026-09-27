# structure_docu.md: Documentation (docu/)

## Purpose
Provides user, administrator and developer documentation for the project. The
[documentation index](../README.md) links every guide by task.

## Layout
Every document is written for one reader and lives in that reader's folder.
A topic with guides for several readers gets its own folder with a `README.md`
index. Developer notes for such a topic still go to `developer/`.

| Path | Reader | Contents |
| --- | --- | --- |
| `README.md` | Everyone | Documentation index |
| `user/` | API users and client developers | API behavior, e.g. `aas_api_v3_2.md` |
| `admin/` | Administrators and operators | `logging.md`, `telemetry.md`, `errors.md` |
| `eventing/` | Administrators and event consumers | MQTT, AMQP and Kafka publishing, the Event Feed |
| `security/` | Administrators and API users | ABAC policy repository, registry rights, NIS2 evidence, supply chain, `keycloak/` realm export, `rebac/` |
| `security/rebac/` | Administrators and API users | ReBAC overview, `administration.md`, `sharing-api.md` |
| `query_language/` | API users and ABAC policy authors | Query and ABAC filter examples |
| `developer/` | Contributors | Structure, runtime notes, `security_architecture.md`, `rebac.md`, `query_language/`, `database/`, tests and tooling |
| `assets/` | - | Images used by the documentation |

## Adding Documentation
- Pick the folder by reader, not by component.
- Put implementation details in `developer/`, even for topics that have their
  own folder, and link them from the topic index.
- When a topic gains guides for a second reader, create a topic folder with a
  `README.md` index, following `security/rebac/`.
- Link the new guide from the topic index or from the [documentation index](../README.md).
- Use relative links and check that they resolve after moving files.
