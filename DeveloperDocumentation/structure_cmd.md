# structure_cmd.md: Service Entry Points (cmd/)

## Purpose
Contains the main entry points for each microservice in the BaSyx Go Components project. Each subfolder represents a service, e.g., `aasregistryservice`, `submodelrepositoryservice`.

## Typical Contents
- `main.go`: Service startup and configuration
- `config.yaml`: Service-specific configuration
- `Dockerfile`: Containerization instructions
- `healthcheck.sh`: Health check scripts for Docker
- `resources/`, `config/`: Additional service resources

## Example: submodelrepositoryservice
- Handles HTTP requests for submodel repository operations
- Loads configuration from `config.yaml`
- Exposes REST API as defined in `api/submodelrepository/openapi.yaml`

## How to Extend
- Add new service folders for additional microservices
- Implement new entry points in `main.go`
- Update Dockerfile and configs as needed
