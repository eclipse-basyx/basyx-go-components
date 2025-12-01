# BaSyx Go Components

Welcome to the repo of the BaSyx Go Components

## Developing BaSyx Go Components

We provide launch scripts for the different services. They are configured in the `.vscode/launch.json` file. You can run and debug the services directly from VSCode using the `Run and Debug` view.

Alternatively you can run the services from the command line. For example, to run the Submodel Repository Service, use the following command:

```bash
go run ./cmd/submodelrepositoryservice/main.go -config ./cmd/submodelrepositoryservice/config.yaml -databaseSchema ./basyxschema.sql
```
