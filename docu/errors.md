# Errors and their meaning
This Documentation helps you to resolve issues that you encouter with BaSyx Go
## Socket Hang Up
This error always indicates that the requested resource is not available - this could be due to an overwhelmed service or a critical Server Bug. Please open a issue on GitHub with information about the error.
## Internal Server Errors
This section focuses on the known types of Internal Server Errors. A specification of each error should be found on the Console.
### Failed to begin PostgreSQL transaction - no changes applied - see console for details
#### Error Description
If you encounter this error the issue most probably lies in the maxOpenConnections, maxIdleConnections and connMaxLifetimeMinutes limit.
#### Solution A
Increase the Limit of the above mentioned variables in your config.yaml or in your Environment Variables
```yaml
maxOpenConnections: 500
maxIdleConnections: 500
connMaxLifetimeMinutes: 5
```

```.env
POSTGRES_MAXOPENCONNECTIONS=500
POSTGRES_MAXIDLECONNECTIONS=500
POSTGRES_CONNMAXLIFETIMEMINUTES=5
```
If this solution does not resolve the error, it is likely that your hardware does not meet the requirements necessary to process the load on the database server.