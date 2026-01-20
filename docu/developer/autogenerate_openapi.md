# Project Structure Overview: BaSyx Go Components

```sh
podman run --rm -v "${PWD}:/local" openapitools/openapi-generator-cli generate `
  -i /local/Plattform_i40-AssetAdministrationShellRegistryServiceSpecification-V3.1.0_SSP-004-resolved.json `
  -g go-server `
  -o /local/server
```