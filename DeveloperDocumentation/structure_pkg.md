# structure_pkg.md: Shared Libraries (pkg/)

## Purpose
Provides reusable libraries, helpers, and generated API clients for use across services.

## Typical Contents
- `api.go`, `routers.go`: Shared API logic and HTTP routing
- `helpers.go`: Utility functions
- Generated code from OpenAPI specs

## Example Usage
- Common request/response handling
- Shared data models
- API client code for service-to-service communication

## How to Extend
- Add new helpers or shared logic as needed
- Regenerate API clients when OpenAPI specs change
