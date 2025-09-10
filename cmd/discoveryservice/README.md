# BaSyx Discovery Service

This service implements the Asset Administration Shell (AAS) Discovery API as specified by the Industrial Digital Twin Association (IDTA).

## Features

- Asset Administration Shell lookup by asset identifiers
- Asset link management (create, retrieve, delete)
- Service self-description

## Running with Docker

### Building the Docker image

```bash
# From the root of the project
docker build -t basyx-discovery-service -f cmd/discoveryservice/Dockerfile .
```

### Running the Docker container

```bash
# Run the container
docker run -p 5000:5000 basyx-discovery-service
```

### Using Docker Compose

```bash
# From the root of the project
docker-compose up discovery-service
```

## API Endpoints

- `GET /health` - Health check endpoint
- `GET /swagger-ui/*` - Swagger UI documentation
- `GET /description` - Service description
- `GET /lookup/shells` - Get AAS IDs by asset link
- `GET /lookup/shells/{aasIdentifier}` - Get asset links by AAS ID
- `POST /lookup/shells/{aasIdentifier}` - Create asset links for AAS ID
- `DELETE /lookup/shells/{aasIdentifier}` - Delete asset links for AAS ID

### Example Requests

#### POST /lookup/shells/{aasIdentifier}

```bash
curl -X 'POST' \
  'http://localhost:8084/lookup/shells/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYWFzLzI1OTVfNjc5NF85NDc4Xzc2NTE' \
  -H 'accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '[
   {
      "name": "globalAssetId", "value": "https://example.com/ids/asset/1063_3578_5966_4799"
   }
]'
```

#### GET /lookup/shells

```bash
curl -X 'GET' \
  'http://localhost:8084/lookup/shells?assetIds=ew0KICAgIm5hbWUiOiAiZ2xvYmFsQXNzZXRJZCIsDQogICAidmFsdWUiOiAiaHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYXNzZXQvMTA2M18zNTc4XzU5NjZfNDc5OSINCn0' \
  -H 'accept: application/json'
```

#### GET /lookup/shells/{aasIdentifier}

```bash
curl -X 'GET' \
  'http://localhost:8084/lookup/shells/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYWFzLzI1OTVfNjc5NF85NDc4Xzc2NTE' \
  -H 'accept: application/json'
```

#### DELETE /lookup/shells/{aasIdentifier}

```bash
curl -X 'DELETE' \
  'http://localhost:8084/lookup/shells/aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYWFzLzI1OTVfNjc5NF85NDc4Xzc2NTE' \
  -H 'accept: application/json'
```

## Configuration

The service uses environment variables for configuration:

- `LOG_LEVEL` - Logging level (default: "info")
