#!/bin/bash

# Wait for the Registry of Infrastructures to be healthy
echo "Waiting for Registry of Infrastructures to be ready..."
max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if curl -s -f http://localhost:5080/health > /dev/null 2>&1; then
        echo "Registry of Infrastructures is healthy!"
        break
    fi
    attempt=$((attempt + 1))
    echo "Attempt $attempt/$max_attempts - Service not ready yet, waiting..."
    sleep 2
done

if [ $attempt -eq $max_attempts ]; then
    echo "Error: Registry of Infrastructures did not become healthy in time"
    exit 1
fi

# Give it a moment to fully initialize
sleep 2

echo "Pre-filling Registry of Infrastructures with sample data..."

# Add AAS Infrastructure descriptor
echo "Adding AAS Registry..."
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "AAS Registry of the AAS Dataspace for Everybody Tenant 0"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "D4E Tenant 0 AAS Registry"
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
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-registry",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-REGISTRY-3.0"
        }
    ],
    "idShort": "D4ETenant0AASRegistry",
    "id": "https://iese.fraunhofer.de/roi/d4e-tenant-0-aas-registry",
    "company": "Fraunhofer IESE"
}'

echo ""
echo "Adding AAS Discovery Service..."
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "AAS Discovery Service for D4E Tenant 0"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "D4E Tenant 0 AAS Discovery"
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
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-discovery",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-DISCOVERY-3.0"
        }
    ],
    "idShort": "D4ETenant0AASDiscovery",
    "id": "https://iese.fraunhofer.de/roi/d4e-tenant-0-aas-discovery",
    "company": "Fraunhofer IESE"
}'

echo ""
echo "Adding Submodel Registry..."
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "Submodel Registry for D4E Tenant 0"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "D4E Tenant 0 Submodel Registry"
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
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/sm-registry",
                "endpointProtocol": "HTTPS"
            },
            "interface": "SUBMODEL-REGISTRY-3.0"
        }
    ],
    "idShort": "D4ETenant0SubmodelRegistry",
    "id": "https://iese.fraunhofer.de/roi/d4e-tenant-0-submodel-registry",
    "company": "Fraunhofer IESE"
}'

echo ""
echo "Adding AAS Environment - Shells..."
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "AAS Environment Shells Repository for D4E Tenant 0"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "D4E Tenant 0 AAS Shells"
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
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-environment",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-REPOSITORY-3.0"
        }
    ],
    "idShort": "D4ETenant0AASShells",
    "id": "https://iese.fraunhofer.de/roi/d4e-tenant-0-aas-env-shells",
    "company": "Fraunhofer IESE"
}'

echo ""
echo "Adding AAS Environment - Submodels..."
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "AAS Environment Submodels Repository for D4E Tenant 0"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "D4E Tenant 0 Submodels"
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
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-environment",
                "endpointProtocol": "HTTPS"
            },
            "interface": "SUBMODEL-REPOSITORY-3.0"
        }
    ],
    "idShort": "D4ETenant0Submodels",
    "id": "https://iese.fraunhofer.de/roi/d4e-tenant-0-aas-env-submodels",
    "company": "Fraunhofer IESE"
}'

echo ""
echo "Adding AAS Environment - Concept Descriptions..."
curl --location --request POST 'http://localhost:5080/infrastructure-descriptors' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "AAS Environment Concept Descriptions Repository for D4E Tenant 0"
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "D4E Tenant 0 Concept Descriptions"
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
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-environment",
                "endpointProtocol": "HTTPS"
            },
            "interface": "CONCEPT-DESCRIPTION-REPOSITORY-3.0"
        }
    ],
    "idShort": "D4ETenant0ConceptDescriptions",
    "id": "https://iese.fraunhofer.de/roi/d4e-tenant-0-aas-env-concept-descriptions",
    "company": "Fraunhofer IESE"
}'

echo ""
echo "All sample data has been successfully added to Registry of Infrastructures!"
echo "You can access the service at: http://localhost:5080"