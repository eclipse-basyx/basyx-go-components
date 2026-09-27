# BaSyx Go Components Documentation

The documentation is organized by reader. Topics with guides for more than one
reader have their own folder with an index.

| Folder | Reader | Contents |
| --- | --- | --- |
| [user/](user/) | API users and client developers | Behavior of the component APIs |
| [admin/](admin/) | Administrators and operators | Logging, telemetry and troubleshooting |
| [eventing/](eventing/README.md) | Administrators and event consumers | MQTT, AMQP, Kafka and the Event Feed |
| [security/](security/README.md) | Administrators and API users | ABAC, ReBAC, registry rights, evidence, supply chain |
| [query_language/](query_language/README.md) | API users and ABAC policy authors | Query and ABAC filter examples |
| [developer/](developer/) | Contributors | Architecture, internals, database schema, tests and tooling |

## Which guide do I need?

| You want to | Read |
| --- | --- |
| Use the AAS API v3.2 features (history, recent changes, signed reads) | [AAS API v3.2 user guide](user/aas_api_v3_2.md) |
| Configure logging | [Logging](admin/logging.md) |
| Export traces and metrics | [OpenTelemetry telemetry](admin/telemetry.md) |
| Resolve a runtime error | [Errors and their meaning](admin/errors.md) |
| Publish or consume change events | [Eventing](eventing/README.md) |
| Secure a deployment or share resources | [Security](security/README.md) |
| Write queries or ABAC filters | [Query language](query_language/README.md) |
| Understand the repository layout | [Structure overview](developer/structure_overview.md) |
| Understand how security is enforced | [Security architecture](developer/security_architecture.md) |
| Understand the database schema | [Database schema notes](developer/database/README.md) |
