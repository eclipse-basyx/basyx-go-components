# BaSyx Company Lookup

The BaSyx Company Lookup is a component that can be used in dataspaces to find endpoints for company-provided services such as AAS registries, repositories, and related interfaces.

It acts as a centralized directory where participants register Company Descriptors. Clients can then discover available endpoints by company domain, company name, and optional asset ID matching.

## Example Usage of BaSyx Company Lookup

### GET

Retrieve all registered companies:

```bash
curl --location 'http://localhost:5080/companies'
```

Filter by company name (base64url encoded):

```bash
curl --location -G 'http://localhost:5080/companies' --data-urlencode 'name=RnJhdW5ob2ZlciBJRVNF'
```

Retrieve one company descriptor by its encoded domain (for example `iese.fraunhofer.de` -> `aWVzZS5mcmF1bmhvZmVyLmRl`):

```bash
curl --location 'http://localhost:5080/companies/aWVzZS5mcmF1bmhvZmVyLmRl'
```

### POST

Create a new Company Descriptor:

```bash
curl --location --request POST 'http://localhost:5080/companies' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "Example company service provider"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "Example Company"
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
    "idShort": "exampleCompany",
    "name": "Example Company",
    "domain": "example.com",
    "nameOptions": [
        "Example Company Inc",
        "ExampleCo"
    ],
    "assetIdRegexPatterns": [
        "^https://i\\.example\\.com/assetId/"
    ]
}'
```

## API Documentation

The Company Lookup provides the following API endpoints:

- `GET /companies`: Returns all registered Company Descriptors.
- `GET /companies/{companyDomain}`: Returns one Company Descriptor by encoded domain.
- `POST /companies`: Registers a new Company Descriptor.
- `PUT /companies/{companyDomain}`: Updates an existing Company Descriptor.
- `DELETE /companies/{companyDomain}`: Deletes a Company Descriptor.

Optional query parameters on `GET /companies`:

- `limit`: Maximum number of results.
- `cursor`: Pagination cursor.
- `name`: Base64url-encoded company name filter.
- `assetId`: Base64url-encoded asset ID filter.

Hint: the path parameter `companyDomain` must be base64url encoded.
