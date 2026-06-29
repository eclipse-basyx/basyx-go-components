# Project Structure Overview: BaSyx Go Components

```sh
podman run --rm -v "${PWD}:/local" openapitools/openapi-generator-cli generate `
  -i /local/cmd/aasregistryservice/openapi.yaml `
  -g go-server `
  -o /local/server
```