# Errors and their meaning
This Documentation helps you to resolve issues that you encouter with BaSyx Go
## Socket Hang Up
This error always indicates that the requested resource is not available - this could be due to an overwhelmed service or a critical Server Bug. Please open a issue on GitHub with information about the error.
## Internal Server Errors
This section focuses on the known types of Internal Server Errors. A specification of each error should be found on the Console.
### Failed to begin PostgreSQL transaction - no changes applied - see console for details
#### Error Description
This error can occur when all connections in a service's PostgreSQL pool are busy or when PostgreSQL has reached its connection limit. Check the service's database pool metrics, request latency, PostgreSQL connection count, and slow queries before changing the pool size.

#### Solution
Start with the production defaults:

```yaml
maxOpenConnections: 50
maxIdleConnections: 25
connMaxLifetimeMinutes: 5
connMaxIdleTimeMinutes: 0
```

```env
POSTGRES_MAXOPENCONNECTIONS=50
POSTGRES_MAXIDLECONNECTIONS=25
POSTGRES_CONNMAXLIFETIMEMINUTES=5
POSTGRES_CONNMAXIDLETIMEMINUTES=0
```

These limits apply per service process or Kubernetes pod. Before increasing `maxOpenConnections`, verify that:

- `replicas × maxOpenConnections`, summed across all database-backed services, fits below PostgreSQL's usable connection budget.
- Capacity remains available for migrations, monitoring, and administration.
- Pool wait time shows that requests are actually waiting for connections.
- PostgreSQL CPU, storage latency, locks, and slow queries are not the limiting factor.

If the connection budget is sufficient and pool wait time remains high while PostgreSQL still has capacity, increase `maxOpenConnections` gradually and remeasure. Increasing every pod to hundreds of connections can overload PostgreSQL and reduce throughput.
