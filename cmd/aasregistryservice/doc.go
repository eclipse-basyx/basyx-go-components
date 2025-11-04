// Command aasregistryservice starts the AAS Registry HTTP service.
//
// It loads configuration, initializes the PostgreSQL-backed Asset Administration
// Shell (AAS) registry persistence, registers the API routes, and serves the
// HTTP endpoints. CORS and a health endpoint are enabled via common helpers.
//
// Flags:
//   -config          Path to service configuration file
//   -databaseSchema  Optional path to a SQL schema file to initialize the
//                    database (overrides the default bundled schema)
package main

