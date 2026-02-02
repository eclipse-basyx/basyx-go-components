# BaSyx Registry of Infrastructures

The BaSyx Registry of Infrastructures (RoI) is a component that can be used in Datspaces (e.g., in MX Port Leo), to find Endpoints for AAS Infrastructure components such as AAS Servers or other Registries. It acts as a centralized directory where different registries and repositories can register themselves, allowing clients to discover available components and their endpoints.

The component supports filtering based on company names, enabling users to retrieve RoI descriptors specific to a particular company.

## Example Usage of BaSyx Registry of Infrastructures

### GET

The following request can be used to retrieve AAS Infrastructure components for a specific company which is given in the <company_name> parameter.

```bash
curl --location 'localhost:5080/infrastructure-descriptors?company=<company_name>' 
```

For instance, this request retrieves the components for "Fraunhofer IESE".

```bash
curl --location -G 'localhost:5080/infrastructure-descriptors' --data-urlencode 'company=Fraunhofer IESE'
```

### POST

The following request can be used to add a new descriptor for an AAS Infrastructure component for a specific company which is defined in the <company> parameter.

```bash
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "Example AAS Registry"
        }
    ],
    "displayname": [
        {
            "language": "en",
            "text": "AAS Registry 1"
        }
    ],
    "administration": {
        "version": "1",
        "revision": "0",
        "creator": {
            "type": "ExternalReference",
            "keys": [
                {
                    "type": "GlobalReference",
                    "value": "https://example.com"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://example.com/aas-registry",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-REGISTRY-3.0"
        }
    ],
    "idShort": "ExampleAASRegistry",
    "id": "https://example.com/roi/example-aas-registry",
    "company": "ExampleCompany"
}'
```

## API Documentation

The Registry of Infrastructures has the following API endpoints:

- `GET /infrastructure-descriptors`: Retrieves all registered AAS Infrastructure components. It supports an optional query parameter `company` to filter the results based on the company name.
- `GET /infrastructure-descriptors/{id}`: Retrieves a specific AAS Infrastructure component identified by its unique ID.
- `POST /infrastructure-descriptors`: Registers a new AAS Infrastructure component by accepting a RoI descriptor in the request body.
- `DELETE /infrastructure-descriptors/{id}`: Deletes a registered AAS Infrastructure component identified by its unique ID.
- `PUT /infrastructure-descriptors/{id}`: Updates an existing AAS Infrastructure component identified by its unique ID with the new RoI descriptor provided in the request body.

> hint: the ID has to be base64 URL encoded when used in the path parameter.
