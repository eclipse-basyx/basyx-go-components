# structure_api.md: API Specifications (api/)

## Purpose
Defines the REST API contracts for each service using OpenAPI (Swagger) YAML files. Used for code generation, documentation, and client/server validation.

## Typical Contents
- `openapi.yaml`: Main OpenAPI specification for each service
- Example endpoints: `/submodels/{id}/submodel-elements/{idShort}/attachment`

## How It Works
- OpenAPI files describe endpoints, request/response schemas, and error codes
- Used by code generators to produce Go server/client stubs
- Ensures consistent API documentation and validation

## How to Extend
### For new Components
- Add new endpoints to the relevant `openapi.yaml`
- Regenerate API code if needed
- Update documentation for new features
### Extending an already existing Component
- Add the endpoint to the interface and the implementation
